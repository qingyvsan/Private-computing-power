package macos

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
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

// Automator macOS 容器运行时自动配置器
// 通过 Homebrew 安装 Colima 并启动 containerd
type Automator struct {
	mu     sync.Mutex
	status Status
	cancel context.CancelFunc
}

// New 创建自动化配置器
func New() *Automator {
	return &Automator{}
}

// Status 返回当前状态
func (a *Automator) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

// Start 启动异步 macOS 容器运行时自动配置
func (a *Automator) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.status.Running {
		return fmt.Errorf("setup already running")
	}

	a.status = Status{
		Running: true,
		Steps: []StepState{
			{Name: "检测 Homebrew", Status: "pending"},
			{Name: "安装/更新 Colima", Status: "pending"},
			{Name: "启动 Colima 容器运行时", Status: "pending"},
			{Name: "验证 containerd", Status: "pending"},
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
	// Step 1: 检测 Homebrew
	a.setStep(0, "running", "正在检测 Homebrew...")

	brewPath, err := exec.LookPath("brew")
	if err != nil {
		a.appendLog(0, "未找到 Homebrew，正在引导安装...")
		a.appendLog(0, "Homebrew 是 macOS 上的包管理器，用于安装 Colima。")
		a.appendLog(0, "将自动通过 Homebrew 安装脚本进行安装。")

		// 安装 Homebrew（非交互式）
		cmd := exec.CommandContext(ctx, "/bin/bash", "-c",
			`NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)" 2>&1`)
		out, err := cmd.CombinedOutput()
		if err != nil {
			a.setStep(0, "failed", fmt.Sprintf("Homebrew 安装失败: %v\n%s\n\n请手动安装 Homebrew:\n  /bin/bash -c \"$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\"", err, string(out)))
			a.setError("Homebrew 安装失败，请手动安装后重试")
			return
		}
		a.appendLog(0, "Homebrew 安装完成")
	} else {
		a.appendLog(0, fmt.Sprintf("Homebrew 已安装: %s", brewPath))
	}
	a.setStep(0, "done", "Homebrew 已就绪")

	if ctxCancelled(ctx) {
		return
	}

	// Step 2: 安装/更新 Colima
	a.setStep(1, "running", "正在通过 Homebrew 安装 Colima...")

	// 检查是否已安装
	if _, err := exec.LookPath("colima"); err == nil {
		a.appendLog(1, "Colima 已安装，检查更新...")
		out, err := runCmd(ctx, "brew", "upgrade", "colima")
		if err != nil {
			a.appendLog(1, fmt.Sprintf("更新 Colima 时出现警告: %v", err))
		} else {
			a.appendLog(1, strings.TrimSpace(out))
		}
	} else {
		a.appendLog(1, "正在安装 Colima（可能需要几分钟）...")
		out, err := runCmdTimeout(ctx, 5*time.Minute, "brew", "install", "colima")
		if err != nil {
			a.setStep(1, "failed", fmt.Sprintf("Colima 安装失败: %v\n%s", err, out))
			a.setError("Colima 安装失败，请检查网络连接后重试")
			return
		}
		a.appendLog(1, strings.TrimSpace(out))
	}
	a.setStep(1, "done", "Colima 已安装")

	if ctxCancelled(ctx) {
		return
	}

	// Step 3: 启动 Colima
	a.setStep(2, "running", "正在启动 Colima 容器运行时...")

	// 检查 Colima 是否已在运行
	statusOut, _ := runCmd(ctx, "colima", "status")
	if strings.Contains(statusOut, "Running") {
		a.appendLog(2, "Colima 已在运行中")
	} else {
		a.appendLog(2, "正在启动 Colima（使用 containerd 运行时）...")
		a.appendLog(2, "首次启动可能需要下载虚拟机镜像（约 1-2GB），请耐心等待...")

		out, err := runCmdTimeout(ctx, 10*time.Minute, "colima", "start", "--runtime", "containerd")
		if err != nil {
			a.setStep(2, "failed", fmt.Sprintf("Colima 启动失败: %v\n%s", err, out))
			a.setError("Colima 启动失败，请检查日志后重试")
			return
		}
		a.appendLog(2, strings.TrimSpace(out))
	}

	// 等待 containerd socket 就绪
	a.appendLog(2, "等待 containerd socket 就绪...")
	socketPath := expandHome("~/.colima/default/containerd.sock")
	socketReady := false
	for i := 0; i < 60; i++ { // 最多等待 60 秒
		if ctxCancelled(ctx) {
			return
		}
		cmd := exec.CommandContext(ctx, "test", "-S", socketPath)
		if err := cmd.Run(); err == nil {
			a.appendLog(2, fmt.Sprintf("containerd socket 已就绪: %s", socketPath))
			socketReady = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !socketReady {
		a.setStep(2, "failed", fmt.Sprintf("containerd socket 未就绪: %s", socketPath))
		a.setError("containerd socket 未就绪，请检查 Colima 状态")
		return
	}

	a.setStep(2, "done", "Colima 容器运行时就绪")

	if ctxCancelled(ctx) {
		return
	}

	// Step 4: 验证 containerd
	a.setStep(3, "running", "正在验证容器运行时...")

	// 通过 colima ssh 执行 ctr version 验证
	out, err := runCmdTimeout(ctx, 30*time.Second, "colima", "ssh", "--", "ctr", "version")
	if err != nil {
		a.appendLog(3, fmt.Sprintf("通过 colima ssh 验证失败: %v", err))
		// 尝试直接验证 socket
		cmd := exec.CommandContext(ctx, "test", "-S", socketPath)
		if cmd.Run() == nil {
			a.appendLog(3, "containerd socket 存在，运行时可用")
		} else {
			a.setStep(3, "failed", fmt.Sprintf("containerd 验证失败: %v", err))
			a.setError("容器运行时验证失败，请检查 Colima 状态")
			return
		}
	} else {
		a.appendLog(3, strings.TrimSpace(out))
		a.appendLog(3, "容器运行时正常工作")
	}

	a.setStep(3, "done", "环境验证通过，容器运行时就绪")
	a.setDone()

	log.Printf("macos: macOS 容器运行时自动配置完成")
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

// runCmd 执行命令并返回输出
func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runCmdTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return runCmd(ctx, name, args...)
}

// expandHome 展开 ~ 为用户 home 目录
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return home + path[1:]
		}
	}
	return path
}