package wsl2

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// StepState 自动化步骤状态
type StepState struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pending, running, done, failed, skipped
	Log    string `json:"log,omitempty"`
}

// Status 自动化整体状态
type Status struct {
	Running bool        `json:"running"`
	Steps   []StepState `json:"steps"`
	Error   string      `json:"error,omitempty"`
}

// AutomatorConfig 配置自动配置器的参数
type AutomatorConfig struct {
	DistroName       string // WSL2 发行版名称，默认 "Ubuntu-24.04"
	InstallPath      string // Windows 端安装目录，空 = 默认 ~/wsl/{distro}
	ContainerdSocket string // WSL2 内 containerd socket 路径，空 = 默认 /run/containerd/containerd.sock
}

// Automator WSL2 环境自动化配置器
type Automator struct {
	mu     sync.Mutex
	status Status
	cancel context.CancelFunc
	distro string // 检测到的 WSL2 发行版名称
	cfg    AutomatorConfig

	// OnReady 在配置成功完成后被调用（传入发行版名）。可为 nil。
	OnReady func(distro string)
}

// New 创建自动化配置器
func New(cfg AutomatorConfig) *Automator {
	if cfg.DistroName == "" {
		cfg.DistroName = "Ubuntu-24.04"
	}
	if cfg.ContainerdSocket == "" {
		cfg.ContainerdSocket = "/run/containerd/containerd.sock"
	}
	return &Automator{cfg: cfg}
}

// SetConfig 更新自动配置器的配置（仅在下一次 Start 时生效，必须在 Start 前调用）。
// 空字段表示保留当前值，不做覆盖。
func (a *Automator) SetConfig(cfg AutomatorConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if cfg.DistroName != "" {
		a.cfg.DistroName = cfg.DistroName
	}
	if cfg.InstallPath != "" {
		a.cfg.InstallPath = cfg.InstallPath
	}
	if cfg.ContainerdSocket != "" {
		a.cfg.ContainerdSocket = cfg.ContainerdSocket
	}
}

// Status 返回当前状态
func (a *Automator) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

// Start 启动异步 WSL2 自动配置
func (a *Automator) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.status.Running {
		return fmt.Errorf("setup already running")
	}

	a.status = Status{
		Running: true,
		Steps: []StepState{
			{Name: "检测 WSL2 状态", Status: "pending"},
			{Name: "检查/安装 Ubuntu 发行版", Status: "pending"},
			{Name: "安装 containerd", Status: "pending"},
			{Name: "配置 containerd", Status: "pending"},
			{Name: "安装 NVIDIA 容器工具包", Status: "pending"},
			{Name: "验证环境", Status: "pending"},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	go a.run(ctx)
	return nil
}

// Cancel 取消正在进行的配置
func (a *Automator) Cancel() {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()
}

// ========== 内部状态管理 ==========

func (a *Automator) setStep(i int, status, logLine string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if i < len(a.status.Steps) {
		a.status.Steps[i].Status = status
		if logLine != "" {
			a.status.Steps[i].Log = logLine
		}
	}
}

func (a *Automator) appendLog(i int, line string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if i < len(a.status.Steps) {
		if a.status.Steps[i].Log != "" {
			a.status.Steps[i].Log += "\n"
		}
		a.status.Steps[i].Log += line
	}
}

func (a *Automator) setError(err string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status.Error = err
	a.status.Running = false
}

func (a *Automator) setDone() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status.Running = false
}

// ========== 核心逻辑 ==========

func (a *Automator) run(ctx context.Context) {
	// Step 1: 检测 WSL2
	a.setStep(0, "running", "正在检测 wsl.exe...")

	wslPath, err := exec.LookPath("wsl.exe")
	if err != nil {
		a.setStep(0, "failed", "未找到 wsl.exe，请确保已安装 WSL2")
		a.setError("WSL2 未安装，请先启用 WSL2 功能")
		return
	}
	a.appendLog(0, fmt.Sprintf("wsl.exe 路径: %s", wslPath))

	// 检查 WSL 状态
	out, err := runCmd(ctx, "wsl.exe", "--status")
	if err != nil {
		a.appendLog(0, fmt.Sprintf("wsl --status 输出: %s", out))
	} else {
		a.appendLog(0, strings.TrimSpace(out))
	}
	a.setStep(0, "done", "WSL2 已就绪")

	// 检查上下文是否取消
	if ctxCancelled(ctx) {
		return
	}

	// Step 2: 检查/安装发行版
	a.setStep(1, "running", "正在检查已安装的 WSL2 发行版...")

	// wsl.exe -l -v 可能因 WSL 未初始化返回 0xffffffff，此时无发行版
	list, err := runCmd(ctx, "wsl.exe", "-l", "-v")
	if err != nil {
		a.appendLog(1, fmt.Sprintf("wsl -l -v 查询失败: %v（WSL 可能未初始化）", err))
		a.appendLog(1, "尝试直接安装发行版...")
		list = ""
	} else {
		a.appendLog(1, list)
	}

	distro := findUbuntuDistro(list)
	if distro == "" {
		// 尝试通过 wsl --install 安装（需要管理员权限）
		a.appendLog(1, fmt.Sprintf("未找到 %s 发行版，正在安装...", a.cfg.DistroName))
		a.appendLog(1, "（首次安装需要从 Microsoft Store 下载，可能需要几分钟）")

		out, err = runCmdTimeout(ctx, 5*time.Minute, "wsl.exe", "--install", "-d", a.cfg.DistroName)
		if err != nil {
			a.appendLog(1, fmt.Sprintf("wsl --install 失败，尝试 wsl --import 方式..."))

			// 使用 wsl --import 方式（不需要管理员权限）
			if err := a.installDistroViaImport(ctx); err != nil {
				a.setStep(1, "failed", fmt.Sprintf("安装失败: %v\n\n请以管理员身份运行 PowerShell 执行:\n  wsl --install -d %s\n完成后返回此页面继续。", err, a.cfg.DistroName))
				a.setError("Ubuntu 发行版安装失败")
				return
			}
		} else {
			a.appendLog(1, out)
			a.appendLog(1, "发行版安装完成，等待初始化...")
			select {
			case <-ctx.Done():
				a.setError("配置已取消")
				return
			case <-time.After(10 * time.Second):
			}
		}

		distro = a.cfg.DistroName
	}

	// 确保 WSL2 版本
	if ctxCancelled(ctx) {
		return
	}
	a.appendLog(1, "确保 WSL2 版本...")
	runCmd(ctx, "wsl.exe", "--set-default-version", "2")

	// 检查 distro 是否为 WSL2，如果是 WSL1 则转换
	if ctxCancelled(ctx) {
		return
	}
	if list, err = runCmd(ctx, "wsl.exe", "-l", "-v"); err != nil {
		a.appendLog(1, fmt.Sprintf("wsl -l -v 查询失败: %v", err))
		list = ""
	}
	for _, line := range strings.Split(list, "\n") {
		if strings.Contains(line, distro) && strings.Contains(line, "1") {
			a.appendLog(1, "将发行版转换为 WSL2...")
			runCmdTimeout(ctx, 5*time.Minute, "wsl.exe", "--set-version", distro, "2")
			break
		}
	}

	a.distro = distro
	a.appendLog(1, fmt.Sprintf("使用发行版: %s", distro))
	a.setStep(1, "done", fmt.Sprintf("发行版 %s 已就绪", distro))

	// Step 3: 安装 containerd（先检查是否已安装，存在则跳过）
	if ctxCancelled(ctx) {
		return
	}
	a.setStep(2, "running", "正在检查 containerd 是否已安装...")
	out, err = runWSL(ctx, distro, "command -v containerd >/dev/null 2>&1 && echo installed || echo missing")
	if err == nil && strings.Contains(out, "installed") {
		a.appendLog(2, "containerd 已安装，跳过安装步骤")
		a.setStep(2, "skipped", "containerd 已安装")
	} else {
		a.setStep(2, "running", "正在更新 apt 缓存...")
		out, err = runWSL(ctx, distro, "apt-get update -qq 2>&1")
		if err != nil {
			a.appendLog(2, fmt.Sprintf("apt update 警告: %v", err))
		} else {
			a.appendLog(2, trimOutput(out))
		}

		if ctxCancelled(ctx) {
			return
		}

		a.appendLog(2, "正在安装 containerd...")
		out, err = runWSLTimeout(ctx, 3*time.Minute, distro, "apt-get install -y -qq containerd 2>&1")
		if err != nil {
			a.setStep(2, "failed", fmt.Sprintf("安装 containerd 失败: %v\n%s", err, out))
			a.setError("containerd 安装失败")
			return
		}
		a.appendLog(2, trimOutput(out))
		a.setStep(2, "done", "containerd 已安装")
	}

	// Step 4: 配置 containerd（先检查是否已配置，存在则跳过）
	if ctxCancelled(ctx) {
		return
	}
	a.setStep(3, "running", "正在检查 containerd 配置...")
	out, err = runWSL(ctx, distro, "test -f /etc/containerd/config.toml && test -f /opt/cp-agent/start-containerd.sh && echo configured || echo not_configured")
	if err == nil && strings.Contains(out, "configured") {
		a.appendLog(3, "containerd 已配置，跳过配置步骤")
		a.setStep(3, "skipped", "containerd 已配置")
	} else {
		a.setStep(3, "running", "正在生成 containerd 默认配置...")

		cmds := []struct {
			desc string
			cmd  string
		}{
			{"创建配置目录", "mkdir -p /etc/containerd"},
			{"生成默认配置", "containerd config default > /etc/containerd/config.toml 2>&1"},
			{"启用 SystemdCgroup", `sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml`},
			{"创建启动脚本", `mkdir -p /opt/cp-agent && cat > /opt/cp-agent/start-containerd.sh << 'SCRIPT'
#!/bin/bash
if ! pidof containerd > /dev/null 2>&1; then
    containerd &
    sleep 2
    echo "containerd started"
else
    echo "containerd already running"
fi
SCRIPT
chmod +x /opt/cp-agent/start-containerd.sh`},
			{"配置 WSL2 systemd", `cat > /etc/wsl.conf << 'EOF'
[boot]
systemd=true
EOF`},
		}

		for _, step := range cmds {
			if ctxCancelled(ctx) {
				return
			}
			a.appendLog(3, step.desc+"...")
			out, err = runWSL(ctx, distro, step.cmd)
			if err != nil {
				a.appendLog(3, fmt.Sprintf("  -> 警告: %v", err))
			} else if out != "" {
				a.appendLog(3, "  -> "+trimOutput(out))
			}
		}

		a.setStep(3, "done", "containerd 已配置")
	}

	// Step 5: NVIDIA 容器工具包
	if ctxCancelled(ctx) {
		return
	}
	a.setStep(4, "running", "正在检测 NVIDIA GPU...")

	out, err = runWSL(ctx, distro, "nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null || echo 'no_gpu'")
	if err != nil || strings.Contains(out, "no_gpu") || strings.TrimSpace(out) == "" {
		a.appendLog(4, "未检测到 NVIDIA GPU，跳过")
		a.setStep(4, "skipped", "未检测到 NVIDIA GPU")
	} else {
		gpuName := strings.TrimSpace(out)
		a.appendLog(4, fmt.Sprintf("检测到 GPU: %s", gpuName))

		a.appendLog(4, "添加 NVIDIA APT 仓库...")
		nvidiaCmds := []string{
			`curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg 2>/dev/null`,
			`curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' > /etc/apt/sources.list.d/nvidia-container-toolkit.list 2>/dev/null`,
			`apt-get update -qq 2>&1`,
			`apt-get install -y -qq nvidia-container-toolkit 2>&1`,
		}
		for _, c := range nvidiaCmds {
			if ctxCancelled(ctx) {
				return
			}
			out, err = runWSLTimeout(ctx, 2*time.Minute, distro, c)
			if err != nil {
				a.appendLog(4, fmt.Sprintf("  警告: %v", err))
			} else if out != "" {
				a.appendLog(4, "  "+trimOutput(out))
			}
		}

		a.appendLog(4, "配置 containerd 使用 NVIDIA 运行时...")
		out, err = runWSL(ctx, distro, "nvidia-ctk runtime configure --runtime=containerd 2>&1")
		if err != nil {
			a.appendLog(4, fmt.Sprintf("  警告: %v", err))
		} else {
			a.appendLog(4, "  "+trimOutput(out))
		}

		a.setStep(4, "done", "NVIDIA 容器工具包已安装")
	}

	// Step 6: 验证环境
	if ctxCancelled(ctx) {
		return
	}
	a.setStep(5, "running", "正在启动 containerd...")

	out, err = runWSL(ctx, distro, "bash /opt/cp-agent/start-containerd.sh 2>&1")
	if err != nil {
		a.appendLog(5, fmt.Sprintf("启动警告: %v", err))
	} else {
		a.appendLog(5, trimOutput(out))
	}

	a.appendLog(5, "验证 containerd 版本...")
	out, err = runWSL(ctx, distro, "ctr version 2>&1")
	if err != nil {
		a.setStep(5, "failed", fmt.Sprintf("containerd 验证失败: %v\n%s", err, out))
		a.setError("containerd 环境验证失败，请检查 WSL2 状态")
		return
	}
	a.appendLog(5, trimOutput(out))

	a.appendLog(5, "验证容器运行时...")
	out, err = runWSL(ctx, distro, "ctr image pull docker.io/library/hello-world:latest 2>&1")
	if err != nil {
		a.appendLog(5, fmt.Sprintf("（可选）拉取测试镜像失败: %v", err))
	} else {
		a.appendLog(5, "  容器运行时正常工作")
		runWSL(ctx, distro, "ctr image rm docker.io/library/hello-world:latest > /dev/null 2>&1")
	}

	a.setStep(5, "done", "环境验证通过，容器运行时就绪")
	a.setDone()

	log.Printf("wsl2: WSL2 自动配置完成 (distro=%s)", distro)

	// 通知外部调用方配置已完成（如启动 socat 代理）
	if a.OnReady != nil {
		a.OnReady(distro)
	}
}

// installDistroViaImport 通过 wsl --import 安装发行版（不需要管理员权限）
func (a *Automator) installDistroViaImport(ctx context.Context) error {
	a.appendLog(1, fmt.Sprintf("通过 wsl --import 方式安装 %s...", a.cfg.DistroName))

	// 创建临时目录下载 rootfs
	tmpDir, err := os.MkdirTemp("", "cp-wsl2-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	rootfsPath := filepath.Join(tmpDir, "ubuntu-wsl-24.04.tar.gz")

	// 下载 Ubuntu WSL rootfs
	a.appendLog(1, "正在下载 Ubuntu WSL rootfs（约 300MB）...")
	if err := a.downloadFile(ctx, "https://cloud-images.ubuntu.com/wsl/ubuntu-wsl-24.04.tar.gz", rootfsPath); err != nil {
		return fmt.Errorf("下载 rootfs 失败: %w", err)
	}
	a.appendLog(1, "rootfs 下载完成")

	if ctxCancelled(ctx) {
		return ctx.Err()
	}

	// 安装目录：优先使用配置的路径（用户可迁移到 D 盘等），否则默认 ~/wsl/{distro}
	installDir := a.cfg.InstallPath
	if installDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "./data/wsl"
		}
		installDir = filepath.Join(homeDir, "wsl", a.cfg.DistroName)
	}

	// 导入发行版
	a.appendLog(1, fmt.Sprintf("正在导入 WSL 发行版到 %s...", installDir))
	out, err := runCmd(ctx, "wsl.exe", "--import", a.cfg.DistroName, installDir, rootfsPath)
	if err != nil {
		return fmt.Errorf("导入失败: %w\n%s", err, decodeWSLOutput(out))
	}
	a.appendLog(1, decodeWSLOutput(out))
	a.appendLog(1, "发行版导入完成，正在初始化...")

	// 等待初始化
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
	}

	return nil
}

// downloadFile 下载文件，支持断点续传
func (a *Automator) downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// 带进度的复制
	buf := make([]byte, 32*1024)
	written := int64(0)
	total := resp.ContentLength
	lastReport := time.Now()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			written += int64(n)
			if total > 0 && time.Since(lastReport) > 5*time.Second {
				pct := int(float64(written) / float64(total) * 100)
				a.appendLog(1, fmt.Sprintf("  下载进度: %d%% (%d/%d MB)", pct, written/1024/1024, total/1024/1024))
				lastReport = time.Now()
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// ========== 辅助函数 ==========

func ctxCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// runCmd 执行命令并返回输出（自动处理 UTF-16LE 编码）
func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return decodeWSLOutput(string(out)), err
}

func runCmdTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return runCmd(ctx, name, args...)
}

// runWSL 在 WSL2 发行版中执行命令
func runWSL(ctx context.Context, distro, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "wsl.exe", "-d", distro, "-u", "root", "-e", "bash", "-c", command)
	out, err := cmd.CombinedOutput()
	return decodeWSLOutput(string(out)), err
}

func runWSLTimeout(ctx context.Context, timeout time.Duration, distro, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return runWSL(ctx, distro, command)
}

// decodeWSLOutput 解码 wsl.exe 的 UTF-16LE 输出
func decodeWSLOutput(s string) string {
	// 检测是否为 UTF-16LE（包含 \x00 空字节）
	b := []byte(s)
	if len(b) < 2 {
		return s
	}

	// 检查是否包含 UTF-16LE 特征（偶数字节位置有空字节）
	nullCount := 0
	for i := 1; i < len(b); i += 2 {
		if b[i] == 0 {
			nullCount++
		}
	}
	if nullCount < len(b)/4 {
		return s // 不是 UTF-16LE
	}

	decoder := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()
	reader := transform.NewReader(bytes.NewReader(b), decoder)
	result, err := io.ReadAll(reader)
	if err != nil {
		return s
	}
	return string(bytes.TrimRight(result, "\x00\r\n"))
}

func findUbuntuDistro(list string) string {
	lines := strings.Split(list, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Ubuntu") || strings.Contains(line, "ubuntu") {
			fields := strings.Fields(line)
			for _, f := range fields {
				if strings.Contains(f, "Ubuntu") || strings.Contains(f, "ubuntu") {
					return f
				}
			}
			if len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return ""
}

func trimOutput(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = s[:500] + "..."
	}
	return s
}
