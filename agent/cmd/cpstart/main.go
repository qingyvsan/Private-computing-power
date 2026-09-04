package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc/credentials"

	"computing-power/pkg/version"

	"computing-power/agent/internal/container"
	cpstartagent "computing-power/agent/internal/cpstart/agent"
	cpstartcfg "computing-power/agent/internal/cpstart/config"
	"computing-power/agent/internal/cpstart/macos"
	"computing-power/agent/internal/cpstart/server"
	"computing-power/agent/internal/cpstart/wsl2"
)

func main() {
	fmt.Printf("Computing Power cpstart %s\n", version.Info())
	fmt.Println()

	if err := run(); err != nil {
		log.Fatalf("cpstart failed: %v", err)
	}
}

func run() error {
	// 加载配置
	cfg := cpstartcfg.Default()
	configPath := cfg.ConfigPath()

	if _, err := os.Stat(configPath); err == nil {
		loaded, err := cpstartcfg.Load(configPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		cfg = loaded
		log.Printf("cpstart: loaded config from %s", configPath)
	} else {
		log.Printf("cpstart: no config found at %s, using defaults (first-time setup)", configPath)
	}

	// 创建 gRPC 桥接
	bridge := server.NewBridge(cfg.Scheduler.Address, buildTLSCreds(cfg))
	defer bridge.Close()

	// 创建 Agent 运行器
	runner := cpstartagent.NewRunner(cfg)

	// 创建 HTTP 服务器
	httpServer, err := server.NewHTTPServer(cfg, bridge, runner)
	if err != nil {
		return fmt.Errorf("create http server: %v", err)
	}

	// 启动 Agent
	if err := runner.Start(); err != nil {
		log.Printf("cpstart: agent start warning: %v", err)
	}

	// 启动 HTTP 服务器
	if err := httpServer.Start(); err != nil {
		return fmt.Errorf("start http server: %v", err)
	}

	// WSL2 代理生命周期管理
	var wsl2Proxy *wsl2.Proxy
	var wsl2Mu sync.Mutex

	// connectAgentRuntime 通过代理连接 containerd 并注入 agent
	connectAgentRuntime := func(proxy *wsl2.Proxy) {
		rt, err := container.NewTCPRuntime(proxy.Addr(), cfg.ContainerdNamespace())
		if err != nil {
			log.Printf("cpstart: connect containerd via proxy failed: %v", err)
			return
		}
		runner.SetRuntime(rt)
		log.Printf("cpstart: agent container runtime connected via WSL2 proxy %s", proxy.Addr())
	}

	// startWSL2Proxy 在给定发行版上启动 socat 代理并连接 agent
	startWSL2Proxy := func(distro string) bool {
		log.Printf("cpstart: starting WSL2 containerd proxy for %s", distro)
		proxy := wsl2.NewProxy(wsl2.ProxyConfig{
			Distro: distro,
			Port:   cfg.WSL2.ProxyPort,
			Socket: cfg.WSL2.ContainerdSocket,
		})
		proxyCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := proxy.Start(proxyCtx); err != nil {
			log.Printf("cpstart: WSL2 proxy start failed: %v", err)
			return false
		}
		wsl2Mu.Lock()
		wsl2Proxy = proxy
		wsl2Mu.Unlock()
		connectAgentRuntime(proxy)
		return true
	}

	// 检查容器运行时可用性，如果不可用则自动触发环境配置
	if backend := container.DetectBackend(); backend.Type == "none" {
		if runtime.GOOS == "windows" && cfg.WSL2.Enabled {
			// 获取 HTTP handler 持有的共享 automator 实例
			auto := httpServer.Handler().WSL2Automator()

			// OnReady：WSL2 配置完成后启动 socat 代理
			auto.OnReady = func(distro string) { startWSL2Proxy(distro) }

			// 快速路径：先尝试直接启动代理（WSL2 已配置好的场景）
			fastCtx, fastCancel := context.WithTimeout(context.Background(), 15*time.Second)
			if proxy := wsl2.NewProxy(wsl2.ProxyConfig{Port: cfg.WSL2.ProxyPort, Socket: cfg.WSL2.ContainerdSocket}); proxy.Start(fastCtx) == nil {
				fastCancel()
				wsl2Mu.Lock()
				wsl2Proxy = proxy
				wsl2Mu.Unlock()
				connectAgentRuntime(proxy)
			} else {
				fastCancel()
				// 发行版不存在或 containerd 未装 → 触发完整 automator 安装
				log.Printf("cpstart: WSL2 proxy not ready, auto-triggering WSL2 setup")
				if err := auto.Start(); err != nil {
					log.Printf("cpstart: WSL2 auto setup start failed: %v", err)
				}
			}
		} else if runtime.GOOS == "darwin" {
			log.Printf("cpstart: no container runtime detected, auto-triggering Colima setup")
			macosAuto := macos.New()
			if err := macosAuto.Start(); err != nil {
				log.Printf("cpstart: auto macOS setup start failed: %v", err)
			} else {
				time.AfterFunc(5*time.Second, func() {
					status := macosAuto.Status()
					if !status.Running && status.Error != "" {
						log.Printf("cpstart: macOS auto setup failed: %s", status.Error)
					} else if !status.Running {
						log.Printf("cpstart: macOS auto setup completed")
					} else {
						log.Printf("cpstart: macOS auto setup in progress (check UI for details)")
					}
				})
			}
		}
	} else {
		log.Printf("cpstart: container runtime detected: %s (%s)", backend.Type, backend.Socket)
	}

	// 打开浏览器（如果配置了自动打开且不在 WSL2 中）
	url := fmt.Sprintf("http://127.0.0.1:%d", cfg.Console.Port)
	if cfg.Console.AutoOpen {
		if isWSL2() {
			log.Printf("cpstart: detected WSL2 environment, skipping browser open")
			fmt.Printf("\n  Access the web UI from Windows at: %s\n", url)
			fmt.Printf("  (WSL2 auto-forwards ports to Windows)\n\n")
		} else {
			if err := openBrowser(url); err != nil {
				log.Printf("cpstart: open browser: %v", err)
			}
		}
	}
	log.Printf("cpstart: console available at %s", url)

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("cpstart: received signal %v, shutting down...", sig)

	// 优雅关闭
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 停止 WSL2 代理
	wsl2Mu.Lock()
	if wsl2Proxy != nil && wsl2Proxy.IsRunning() {
		log.Printf("cpstart: stopping WSL2 containerd proxy")
		wsl2Proxy.Stop()
	}
	proxyDistro := ""
	if wsl2Proxy != nil {
		proxyDistro = wsl2Proxy.Distro()
	}
	wsl2Mu.Unlock()

	// 关闭 WSL2 发行版（连带关闭 containerd 等所有进程）
	if proxyDistro != "" {
		log.Printf("cpstart: terminating WSL2 distro %s", proxyDistro)
		if err := exec.CommandContext(shutdownCtx, "wsl.exe", "--terminate", proxyDistro).Run(); err != nil {
			log.Printf("cpstart: wsl --terminate warning: %v", err)
		}
	}

	runner.Stop()
	runner.Wait()

	if err := httpServer.Stop(shutdownCtx); err != nil {
		log.Printf("cpstart: http server shutdown error: %v", err)
	}

	log.Printf("cpstart: stopped")
	return nil
}

// buildTLSCreds 从 cpstart 配置构建 gRPC TLS 凭证
func buildTLSCreds(cfg *cpstartcfg.Config) credentials.TransportCredentials {
	if cfg.TLS.CACert == "" {
		return nil // 使用不安全连接
	}

	caPEM, err := os.ReadFile(cfg.TLS.CACert)
	if err != nil {
		log.Printf("cpstart: read CA cert: %v (falling back to insecure)", err)
		return nil
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		log.Printf("cpstart: parse CA cert failed (falling back to insecure)")
		return nil
	}

	tlsConfig := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
	}

	// 如果存在客户端证书，加载它
	if cfg.TLS.Cert != "" && cfg.TLS.Key != "" {
		if _, err := os.Stat(cfg.TLS.Cert); err == nil {
			if _, err := os.Stat(cfg.TLS.Key); err == nil {
				clientCert, err := tls.LoadX509KeyPair(cfg.TLS.Cert, cfg.TLS.Key)
				if err != nil {
					log.Printf("cpstart: load client cert: %v (proceeding without)", err)
				} else {
					tlsConfig.Certificates = []tls.Certificate{clientCert}
				}
			}
		}
	}

	return credentials.NewTLS(tlsConfig)
}

// openBrowser 在默认浏览器中打开 URL
func openBrowser(url string) error {
	// 使用标准库的 http.Client 验证服务是否已启动
	client := &http.Client{Timeout: 3 * time.Second}
	for i := 0; i < 10; i++ {
		if resp, err := client.Get(url); err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// 跨平台浏览器打开
	switch runtime.GOOS {
	case "windows":
		return runCommand("cmd.exe", "/c", "start", url)
	case "darwin":
		return runCommand("open", url)
	default:
		return runCommand("xdg-open", url)
	}
}

func runCommand(cmd string, args ...string) error {
	// 使用 exec.Command 确保 PATH 查找正确
	return exec.Command(cmd, args...).Start()
}

// isWSL2 检测当前是否运行在 WSL2 环境中
// 通过检查 /proc/sys/kernel/osrelease 是否包含 "microsoft" 或 "WSL"
// 也支持通过 CP_WSL2 环境变量手动指定
func isWSL2() bool {
	if os.Getenv("CP_WSL2") == "1" {
		return true
	}
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return false
	}
	content := strings.ToLower(string(data))
	return strings.Contains(content, "microsoft") || strings.Contains(content, "wsl")
}
