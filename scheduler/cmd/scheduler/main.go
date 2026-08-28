package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	pb "computing-power/proto/v1"

	"computing-power/pkg/trustgraph"
	"computing-power/pkg/version"
	"computing-power/scheduler/internal/ca"
	"computing-power/scheduler/internal/config"
	"computing-power/scheduler/internal/ipam"
	"computing-power/scheduler/internal/registry"
	"computing-power/scheduler/internal/server"
	"computing-power/scheduler/internal/store"
)

func main() {
	var (
		configPath string
		showVer    bool
	)
	flag.StringVar(&configPath, "config", "configs/scheduler.yaml", "配置文件路径")
	flag.BoolVar(&showVer, "version", false, "显示版本信息")
	flag.Parse()

	if showVer {
		fmt.Println(version.Info())
		return
	}

	if err := run(configPath); err != nil {
		log.Fatalf("scheduler failed: %v", err)
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

	log.Printf("computing-power scheduler starting: %s", version.Info())

	// 创建数据目录
	dataDir := filepath.Dir(cfg.Database.Path)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// 打开数据库
	st, err := store.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()
	log.Printf("store opened: %s", cfg.Database.Path)

	// 创建节点注册中心
	reg := registry.NewRegistry(
		cfg.FailureDetection.WindowSize,
		cfg.FailureDetection.MinSamples,
		cfg.FailureDetection.PhiThreshold,
	)

	// 创建 gRPC 服务器
	grpcServer := grpc.NewServer(
		grpc.ForceServerCodecV2(pb.JSONCodec{}),
		grpc.MaxRecvMsgSize(cfg.Server.GRPC.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(cfg.Server.GRPC.MaxSendMsgSize),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     0,
			MaxConnectionAge:      0,
			MaxConnectionAgeGrace: 0,
			Time:                  10,
			Timeout:               5,
		}),
	)

	// 解析心跳间隔和超时
	heartbeatInterval := parseDuration(cfg.FailureDetection.HeartbeatInterval, 3*time.Second)
	heartbeatTimeout := parseDuration(cfg.FailureDetection.HeartbeatTimeout, 30*time.Second)

	// 创建信任图（P7 前为内存空图）
	trust := trustgraph.NewGraph()

	// 从已有信任边恢复
	edges, err := st.ListTrustEdges()
	if err == nil {
		for _, e := range edges {
			trust.AddEdge(e.FromNode, e.ToNode, e.Signature, nil)
		}
	}

	// 创建 Nebula CA（自动生成或加载已有）
	caMgr, err := ca.NewManager(
		cfg.Nebula.CACert,
		cfg.Nebula.CAKey,
		"ComputingPower",
		365*24*time.Hour, // 节点证书有效期 1 年
	)
	if err != nil {
		log.Printf("nebula CA init: %v (overlay network disabled)", err)
		caMgr = nil
	}

	// 创建 IPAM
	ipamMgr, err := ipam.NewIPAM(st, cfg.Nebula.Network, cfg.Nebula.Lighthouse.Host)
	if err != nil {
		log.Printf("ipam init: %v (overlay IP disabled)", err)
		ipamMgr = nil
	}

	// 启动调度器服务
	srv := server.New(st, reg, trust, heartbeatInterval, heartbeatTimeout, cfg, caMgr, ipamMgr)
	srv.Register(grpcServer)

	// 监听信号
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleSignals(cancel)

	// 启动服务器（阻塞）
	return srv.Start(ctx, cfg.Server.GRPC.Listen, grpcServer)
}

func handleSignals(cancel context.CancelFunc) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Printf("received shutdown signal, stopping...")
	cancel()
}

// parseDuration 解析时长字符串，失败时返回默认值
func parseDuration(s string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}