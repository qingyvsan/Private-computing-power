package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"computing-power/pkg/version"

	cpstartagent "computing-power/agent/internal/cpstart/agent"
	cpstartcfg "computing-power/agent/internal/cpstart/config"
	"computing-power/agent/internal/cpstart/server"
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
	bridge := server.NewBridge(cfg.Scheduler.Address)
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

	runner.Stop()
	runner.Wait()

	if err := httpServer.Stop(shutdownCtx); err != nil {
		log.Printf("cpstart: http server shutdown error: %v", err)
	}

	log.Printf("cpstart: stopped")
	return nil
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