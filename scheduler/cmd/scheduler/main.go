package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	pb "computing-power/proto/v1"

	"computing-power/pkg/trustgraph"
	"computing-power/pkg/version"
	"computing-power/pkg/wal"
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

	// 监听信号（提前创建 ctx，WAL 热备需要）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleSignals(cancel)

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

	// ========== WAL 热备 ==========
	if cfg.Sync.Enabled {
		walDir := cfg.Sync.WalDir
		if err := os.MkdirAll(walDir, 0755); err != nil {
			return fmt.Errorf("create wal dir: %w", err)
		}
		w, err := wal.NewWriter(walDir, cfg.Sync.MaxWalSize)
		if err != nil {
			return fmt.Errorf("create wal writer: %w", err)
		}
		st.EnableWAL(w)
		log.Printf("wal enabled: dir=%s role=%s", walDir, cfg.Sync.Role)

		if cfg.Sync.Role == "primary" {
			// Sync 服务（独立端口），先创建以便检查点管理器共享备机 ack
			syncServer := grpc.NewServer(grpc.ForceServerCodecV2(pb.JSONCodec{}))
			syncSvc := server.NewSyncService(st, walDir, cfg.Scheduler.ID)
			pb.RegisterSyncServiceServer(syncServer, syncSvc)

			// 检查点管理器（共享 SyncService 的备机 ack 信息，安全清理 WAL）
			checkpointInterval := parseDuration(cfg.Sync.CheckpointInterval, 5*time.Minute)
			cp := server.NewCheckpointer(st, walDir, checkpointInterval, syncSvc.MinStandbyAck, w.CurrentFileSeq)
			cp.Start(ctx)
			go func() {
				syncLis, err := net.Listen("tcp", cfg.Sync.ListenAddr)
				if err != nil {
					log.Printf("[main] sync listen on %s: %v", cfg.Sync.ListenAddr, err)
					return
				}
				log.Printf("[main] sync service listening on %s", cfg.Sync.ListenAddr)
				if err := syncServer.Serve(syncLis); err != nil {
					log.Printf("[main] sync serve: %v", err)
				}
			}()
		}
	}

	// 创建节点注册中心
	reg := registry.NewRegistry(
		cfg.FailureDetection.WindowSize,
		cfg.FailureDetection.MinSamples,
		cfg.FailureDetection.PhiThreshold,
	)

		// 创建 gRPC mTLS CA（从 cfg.TLS.CACert/CAKey 加载或生成）
	var grpcCA *ca.GRPCCA
	if cfg.TLS.Enabled {
		grpcCA, err = ca.NewGRPCCA(
			cfg.TLS.CACert,
			cfg.TLS.CAKey,
			"ComputingPower",
			365*24*time.Hour,
		)
		if err != nil {
			log.Printf("gRPC CA init: %v (mTLS disabled)", err)
			grpcCA = nil
		} else {
			log.Printf("gRPC mTLS CA initialized from %s, %s", cfg.TLS.CACert, cfg.TLS.CAKey)
		}
	}

	// 构建 gRPC 服务器选项
	var serverOpts []grpc.ServerOption
	serverOpts = append(serverOpts,
		grpc.ForceServerCodecV2(pb.JSONCodec{}),
		grpc.MaxRecvMsgSize(cfg.Server.GRPC.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(cfg.Server.GRPC.MaxSendMsgSize),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     0,
			MaxConnectionAge:      0,
			MaxConnectionAgeGrace: 0,
			Time:                  10 * time.Second,
			Timeout:               5 * time.Second,
		}),
	)

	// 如果 gRPC CA 可用，启用 mTLS
	if grpcCA != nil {
		serverCertPEM, serverKeyPEM, err := grpcCA.GenerateServerCert()
		if err != nil {
			return fmt.Errorf("generate gRPC server cert: %w", err)
		}
		tlsConfig, err := grpcCA.ServerTLSConfig(serverCertPEM, serverKeyPEM)
		if err != nil {
			return fmt.Errorf("build gRPC server TLS config: %w", err)
		}
		serverOpts = append(serverOpts,
			grpc.Creds(credentials.NewTLS(tlsConfig)),
			grpc.UnaryInterceptor(server.UnaryMTLSInterceptor()),
			grpc.StreamInterceptor(server.StreamMTLSInterceptor()),
		)
		log.Printf("gRPC mTLS enabled")
	}

	// 创建 gRPC 服务器
	grpcServer := grpc.NewServer(serverOpts...)

// 解析心跳间隔和超时
	heartbeatInterval := parseDuration(cfg.FailureDetection.HeartbeatInterval, 3*time.Second)
	heartbeatTimeout := parseDuration(cfg.FailureDetection.HeartbeatTimeout, 30*time.Second)

	// 创建信任图（P7 前为内存空图）
	trust := trustgraph.NewGraph()

	// 从已有信任边恢复
	edges, err := st.ListTrustEdges()
	if err == nil {
		for _, e := range edges {
			var expiresAt *time.Time
			if e.ExpiresAt > 0 {
				t := time.UnixMilli(e.ExpiresAt)
				expiresAt = &t
			}
			trust.AddEdge(e.FromNode, e.ToNode, e.Signature, expiresAt)
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
	srv := server.New(st, reg, trust, heartbeatInterval, heartbeatTimeout, cfg, caMgr, ipamMgr, grpcCA)

	// Standby 模式：延迟启动 gRPC server，等待主节点宕机后提升
	if cfg.Sync.Enabled && cfg.Sync.Role == "standby" {
		healthCheckInterval := parseDuration(cfg.Sync.HealthCheckInterval, 5*time.Second)
		failoverTimeout := parseDuration(cfg.Sync.FailoverTimeout, 15*time.Second)

		standby := server.NewStandby(st, cfg.Sync.PrimaryAddr, healthCheckInterval, failoverTimeout)
		standby.Start(ctx)
		log.Printf("[main] standby mode: syncing from primary %s", cfg.Sync.PrimaryAddr)

		// 等待提升或退出
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if standby.IsActive() {
						log.Printf("[main] promoted to active, starting gRPC server on %s", cfg.Server.GRPC.Listen)
						srv.Register(grpcServer)
						if err := srv.Start(ctx, cfg.Server.GRPC.Listen, grpcServer); err != nil {
							log.Printf("[main] server error after promotion: %v", err)
						}
						return
					}
				}
			}
		}()

		<-ctx.Done()
		standby.Stop()
		return nil
	}

	// Primary / 非同步模式：立即启动 gRPC server
	srv.Register(grpcServer)
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