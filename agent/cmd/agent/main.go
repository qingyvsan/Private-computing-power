package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"computing-power/agent/internal/config"
	"computing-power/agent/internal/core"
	"computing-power/pkg/version"
)

func main() {
	var (
		configPath string
		showVer    bool
	)
	flag.StringVar(&configPath, "config", "configs/agent.yaml", "配置文件路径")
	flag.BoolVar(&showVer, "version", false, "显示版本信息")
	flag.Parse()

	if showVer {
		fmt.Println(version.Info())
		return
	}

	if err := run(configPath); err != nil {
		log.Fatalf("agent failed: %v", err)
	}
}

func run(configPath string) error {
	// 加载配置
	cfg := config.Default()
	if _, err := os.Stat(configPath); err == nil {
		loaded, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		cfg = loaded
		log.Printf("loaded config from %s", configPath)
	} else {
		log.Printf("config %s not found, using defaults", configPath)
	}

	log.Printf("computing-power agent starting: %s", version.Info())

	// 创建数据目录
	dirs := []string{cfg.Agent.DataDir, cfg.Agent.TempDir, cfg.HAMI.ConfigDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	// 创建 Agent
	agent, err := core.New(cfg)
	if err != nil {
		return err
	}
	defer agent.Stop()

	// 监听信号
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleSignals(cancel)

	// 启动 Agent（阻塞）
	return agent.Start(ctx)
}

func handleSignals(cancel context.CancelFunc) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Printf("received shutdown signal, stopping...")
	cancel()
}