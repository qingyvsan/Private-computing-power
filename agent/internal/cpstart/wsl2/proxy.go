package wsl2

import (
	"context"
	"fmt"
	"log"
	"net"
	"os/exec"
	"sync"
	"time"
)

// DefaultProxyPort 默认 socat 代理 TCP 端口
const DefaultProxyPort = 19090

// ProxyConfig WSL2 containerd 代理配置
type ProxyConfig struct {
	// Distro 是 WSL2 发行版名称，如 "Ubuntu-24.04"
	Distro string

	// Port 是 socat 监听的 TCP 端口。0 表示使用 DefaultProxyPort。
	Port int

	// Socket 是 WSL2 内 containerd 的 Unix socket 路径。空值使用默认路径。
	Socket string
}

// Proxy 管理 WSL2 内的 socat TCP→Unix socket 转发进程。
// socat 在 WSL2 内监听 TCP 端口，将请求转发到 containerd 的 Unix socket，
// WSL2 自动将 TCP 端口暴露到 Windows 的 127.0.0.1。
//
// 实现方式：wsl.exe 直接运行 socat（不经过 bash -c），wsl 进程作为 socat 的父进程存活。
// 停止时 kill wsl 进程即可连带关闭 socat 和 WSL2 内的端口转发。
type Proxy struct {
	cfg  ProxyConfig
	port int
	host string // 实际探测到的可达地址（127.0.0.1 或 ::1）

	mu      sync.Mutex
	running bool
	cmd     *exec.Cmd // wsl.exe 进程，socat 作为其子进程运行
	done    chan struct{}
	distro  string
}

// NewProxy 创建 WSL2 代理管理器
func NewProxy(cfg ProxyConfig) *Proxy {
	if cfg.Port == 0 {
		cfg.Port = DefaultProxyPort
	}
	if cfg.Socket == "" {
		cfg.Socket = "/run/containerd/containerd.sock"
	}
	return &Proxy{
		cfg:  cfg,
		port: cfg.Port,
		done: make(chan struct{}),
	}
}

// Start 启动 socat 代理。阻塞直到代理就绪或 ctx 取消。
// 步骤：安装依赖 → 启动 socat → 等待端口就绪 → 启动健康检查。
func (p *Proxy) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return fmt.Errorf("proxy already running")
	}
	p.mu.Unlock()

	distro := p.cfg.Distro
	if distro == "" {
		var err error
		distro, err = DetectDistro(ctx)
		if err != nil {
			return fmt.Errorf("detect wsl2 distro: %w", err)
		}
	}
	p.distro = distro

	port := p.port
	log.Printf("wsl2 proxy: starting socat on %s port %d -> %s", distro, port, p.cfg.Socket)

	// 1. 确保 containerd 在 WSL2 内运行
	if out, err := runWSL(ctx, distro, fmt.Sprintf(
		`if [ ! -S %[1]s ]; then
			if [ -f /opt/cp-agent/start-containerd.sh ]; then bash /opt/cp-agent/start-containerd.sh 2>&1;
			else (command -v containerd >/dev/null 2>&1 && containerd &); fi
		fi
		sleep 1
		[ -S %[1]s ] && echo containerd-ready || echo containerd-missing`, p.cfg.Socket)); err != nil {
		return fmt.Errorf("ensure containerd running: %w\n%s", err, decodeWSLOutput(out))
	}

	// 2. 确保 socat 已安装
	if out, err := runWSL(ctx, distro, "command -v socat >/dev/null 2>&1 || (apt-get update -qq && apt-get install -y -qq socat 2>&1)"); err != nil {
		return fmt.Errorf("install socat: %w\n%s", err, decodeWSLOutput(out))
	}

	// 3. 清理残留 socat，然后启动 socat（前台运行，wsl.exe 保持存活）
	runWSL(ctx, distro, fmt.Sprintf("pkill -f 'socat.*TCP-LISTEN:%d' 2>/dev/null", port))

	// 使用 bash -c "exec socat ..." 前台运行 socat，bash 进程被 socat 替换，
	// wsl.exe 作为 socat 的父进程存活。当 socat 退出时 wsl.exe 自动退出。
	// 停止时 kill wsl.exe 进程即可连带关闭 socat。
	socatCmd := fmt.Sprintf("exec socat TCP-LISTEN:%d,fork,reuseaddr,backlog=128 UNIX-CONNECT:%s",
		port, p.cfg.Socket)
	cmd := exec.Command("wsl.exe", "-d", distro, "-u", "root", "-e", "bash", "-c", socatCmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start socat: %w", err)
	}
	p.cmd = cmd

	// 4. 等待端口就绪（轮询 Windows 侧 loopback）。WSL2 localhostForwarding
	// 可能绑定在 IPv4 (127.0.0.1) 或 IPv6 (::1)，两者都探测。
	deadline := time.Now().Add(10 * time.Second)
	host := ""
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			p.killAndWait()
			return ctx.Err()
		default:
		}
		for _, h := range []string{"127.0.0.1", "::1"} {
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(h, fmt.Sprintf("%d", port)), time.Second)
			if err == nil {
				conn.Close()
				host = h
				break
			}
		}
		if host != "" {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if host == "" {
		p.killAndWait()
		return fmt.Errorf("wsl2 proxy: timed out waiting for socat on port %d", port)
	}

	p.host = host
	p.mu.Lock()
	p.running = true
	p.mu.Unlock()

	// 5. 启动后台健康检查（监控 socat 进程）
	go p.healthLoop()

	log.Printf("wsl2 proxy: socat ready on %s:%d -> %s", host, port, p.cfg.Socket)
	return nil
}

// Stop 停止 socat 代理。杀死进程，等待 healthLoop 确认退出后返回。
func (p *Proxy) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	p.mu.Unlock()

	// 杀死 wsl.exe 进程（连带 socat 一起结束），由 healthLoop 的 watcher
	// goroutine 负责 Wait，避免重复 Wait 死锁。
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
	<-p.done
	log.Printf("wsl2 proxy: stopped")
}

// killAndWait 在 Start 失败路径（healthLoop 尚未启动）上杀死并回收进程句柄。
func (p *Proxy) killAndWait() {
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}
}

// Port 返回代理监听的 TCP 端口
func (p *Proxy) Port() int {
	return p.port
}

// Addr 返回 Windows 侧可访问的代理地址
func (p *Proxy) Addr() string {
	p.mu.Lock()
	host := p.host
	p.mu.Unlock()
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", p.port))
}

// IsRunning 返回代理是否正在运行
func (p *Proxy) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// Distro 返回 WSL2 发行版名称
func (p *Proxy) Distro() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.distro
}

// Done 返回一个 channel，代理停止时关闭
func (p *Proxy) Done() <-chan struct{} {
	return p.done
}

// healthLoop 监控 socat 进程。当 wsl.exe（socat 父进程）退出时，
// 标记代理停止并关闭 done 通道。同时定期检查 TCP 端口。
func (p *Proxy) healthLoop() {
	defer close(p.done)

	// 监听 wsl.exe 进程退出
	waitCh := make(chan error, 1)
	go func() {
		if p.cmd != nil {
			waitCh <- p.cmd.Wait()
		}
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case err := <-waitCh:
			log.Printf("wsl2 proxy: socat process exited: %v", err)
			p.mu.Lock()
			p.running = false
			p.mu.Unlock()
			return
		case <-ticker.C:
			// 定期确认 TCP 端口仍可达
			conn, err := net.DialTimeout("tcp", p.Addr(), time.Second)
			if err != nil {
				log.Printf("wsl2 proxy: port %d not reachable (process may be dying)", p.port)
				continue
			}
			conn.Close()
		}
	}
}

// DetectDistro 检测第一个可用的 WSL2 Ubuntu 发行版名称
func DetectDistro(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "wsl.exe", "-l", "-v")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("wsl -l -v failed: %w. is WSL2 installed?", err)
	}
	distro := findUbuntuDistro(decodeWSLOutput(string(out)))
	if distro == "" {
		return "", fmt.Errorf("no Ubuntu WSL2 distro found. please install one first")
	}
	return distro, nil
}