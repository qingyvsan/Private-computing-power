package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	// 打开浏览器（如果配置了自动打开）
	url := fmt.Sprintf("http://127.0.0.1:%d", cfg.Console.Port)
	if cfg.Console.AutoOpen {
		if err := openBrowser(url); err != nil {
			log.Printf("cpstart: open browser: %v", err)
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
	var cmd string
	var args []string
	switch goos := os.Getenv("GOOS"); goos {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		// 根据 runtime.GOOS 判断
		if os.PathSeparator == '\\' {
			cmd = "cmd"
			args = []string{"/c", "start", url}
		} else {
			cmd = "xdg-open"
			args = []string{url}
		}
	}
	return runCommand(cmd, args...)
}

func runCommand(cmd string, args ...string) error {
	// 简单的命令执行包装
	// 使用 os.StartProcess 或 exec.Command
	proc, err := os.StartProcess(cmd, append([]string{cmd}, args...), &os.ProcAttr{
		Files: []*os.File{nil, nil, nil},
	})
	if err != nil {
		return err
	}
	return proc.Release()
}