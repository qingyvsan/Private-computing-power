package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "computing-power/proto/v1"

	"computing-power/agent/internal/config"
	"computing-power/agent/internal/container"
	"computing-power/agent/internal/executor"
	"computing-power/agent/internal/heartbeat"
	"computing-power/agent/internal/nebula"
	"computing-power/pkg/updater"
	"computing-power/pkg/version"
)

// Agent 节点 Agent 生命周期管理器
type Agent struct {
	cfg       *config.Config
	nodeID    string
	conn      *grpc.ClientConn
	client    pb.SchedulerServiceClient
	collector *heartbeat.Collector
	runtime   container.Runtime

	manager   *executor.Manager
	reporter  *executor.Reporter
	exec      *executor.Executor
	nebulaMgr *nebula.Manager
	updater   *updater.Updater

	onRegistered func(nodeID string) // 注册完成回调
}

// New 创建节点 Agent
func New(cfg *config.Config) (*Agent, error) {
	// 创建 Nebula 管理器
	nebulaCfg := &nebula.Config{
		Enabled:    cfg.Nebula.Enabled,
		BinaryPath: cfg.Nebula.BinaryPath,
		DataDir:    cfg.Nebula.DataDir,
		ConfigPath: cfg.Nebula.ConfigPath,
		CertDir:    cfg.Nebula.CertDir,
	}
	nebulaMgr := nebula.NewManager(nebulaCfg)

	a := &Agent{
		cfg:       cfg,
		nebulaMgr: nebulaMgr,
		collector: heartbeat.NewCollector(cfg.Resources.ReportGPU, cfg.Resources.ReportNetwork, nebulaMgr, cfg.Scheduler.Address),
	}

	// 初始化自动更新器
	if cfg.Updater.Enabled {
		uCfg := updater.Config{
			Enabled:        cfg.Updater.Enabled,
			CheckInterval:  parseDuration(cfg.Updater.CheckInterval, 6*time.Hour),
			ManifestURL:    cfg.Updater.ManifestURL,
			DownloadDir:    cfg.Updater.DownloadDir,
			BackupCount:    cfg.Updater.BackupCount,
			CurrentVersion: version.Short(),
			BinaryPath:     os.Args[0],
			Platform:       fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH),
		}
		a.updater = updater.New(uCfg)
		log.Printf("updater enabled: interval=%s, manifest=%s", cfg.Updater.CheckInterval, cfg.Updater.ManifestURL)
	}

	// 初始化容器运行时
	if rt, err := container.NewRuntime(cfg.Containerd.Socket, cfg.Containerd.Namespace); err == nil {
		a.runtime = rt
		log.Printf("container runtime available: %s", cfg.Containerd.Socket)
	} else {
		log.Printf("container runtime not available: %v", err)
	}

	return a, nil
}

// NodeID 返回注册后的节点 ID
func (a *Agent) NodeID() string {
	return a.nodeID
}

// SetOnRegistered 设置注册完成回调
func (a *Agent) SetOnRegistered(fn func(nodeID string)) {
	a.onRegistered = fn
}

// Start 启动 Agent
func (a *Agent) Start(ctx context.Context) error {
	// 建立到调度器的连接
	conn, err := grpc.NewClient(
		a.cfg.Scheduler.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()), // TODO(P6): 使用 mTLS
		grpc.WithDefaultCallOptions(grpc.ForceCodecV2(pb.JSONCodec{})),
	)
	if err != nil {
		return fmt.Errorf("connect to scheduler: %w", err)
	}
	a.conn = conn
	a.client = pb.NewSchedulerServiceClient(conn)

	// 注册节点
	resp, err := a.register(ctx)
	if err != nil {
		return fmt.Errorf("register node: %w", err)
	}

	// 配置并启动 Nebula（如果启用且调度器返回了证书）
	if a.cfg.Nebula.Enabled && resp != nil && len(resp.NebulaCertificate) > 0 {
		lighthouseAddr := a.cfg.Scheduler.Address
		if err := a.nebulaMgr.Configure(
			resp.NebulaCertificate,
			resp.NebulaPrivateKey,
			resp.CACertificate,
			resp.OverlayIP,
			lighthouseAddr,
			resp.NebulaConfig,
		); err != nil {
			log.Printf("nebula configure: %v (overlay disabled)", err)
		} else {
			if err := a.nebulaMgr.Start(); err != nil {
				log.Printf("nebula start: %v (overlay disabled)", err)
			}
		}
	}

	// 创建 executor 组件
	a.manager = executor.NewManager()
	a.reporter = executor.NewReporter(a.nodeID, a.client)

	// 创建 HAMi 管理器（未启用时返回 nil）
	hamiMgr := container.NewHAMiManager(
		a.cfg.HAMI.Enabled,
		a.cfg.HAMI.LibPath,
		a.cfg.HAMI.ConfigDir,
		a.cfg.HAMI.DefaultMemoryMB,
		a.cfg.HAMI.DefaultCores,
	)

	a.exec = executor.NewExecutor(a.runtime, a.manager, a.reporter, hamiMgr)

	// 在后台启动状态上报流
	go func() {
		initial := parseDuration(a.cfg.Scheduler.ReconnectInitial, time.Second)
		maxDelay := parseDuration(a.cfg.Scheduler.ReconnectMax, 60*time.Second)
		if err := a.reporter.StartWithRetry(ctx, initial, maxDelay); err != nil {
			log.Printf("unit status reporter stopped: %v", err)
		}
	}()

	// 启动自动更新器
	if a.updater != nil {
		a.updater.Start(ctx)
	}

	// 启动心跳上报
	interval := parseDuration(a.cfg.Heartbeat.Interval, 3*time.Second)
	jitter := parseDuration(a.cfg.Heartbeat.Jitter, 500*time.Millisecond)
	reporter := heartbeat.NewReporter(
		a.nodeID,
		a.collector,
		a.client,
		interval,
		jitter,
		func() []string {
			return a.manager.List()
		},
		a.exec.HandleCommand,
	)

	initial := parseDuration(a.cfg.Scheduler.ReconnectInitial, time.Second)
	maxDelay := parseDuration(a.cfg.Scheduler.ReconnectMax, 60*time.Second)
	return reporter.StartWithRetry(ctx, conn, initial, maxDelay)
}

// register 注册节点到调度器
func (a *Agent) register(ctx context.Context) (*pb.RegisterNodeResponse, error) {
	name := a.cfg.Agent.Name
	if name == "" {
		name = "agent-" + fmt.Sprintf("%d", time.Now().UnixMilli()%100000)
	}

	// 收集硬件指纹
	hostname, _ := os.Hostname()

	req := &pb.RegisterNodeRequest{
		Name:                name,
		InviteCode:          a.cfg.Scheduler.InviteCode,
		HardwareFingerprint: hostname,
		Version:             "0.1.0-dev",
	}

	resp, err := a.client.RegisterNode(ctx, req)
	if err != nil {
		return nil, err
	}

	a.nodeID = resp.NodeID
	log.Printf("registered as node %s, overlay IP %s", resp.NodeID, resp.OverlayIP)

	if a.onRegistered != nil {
		a.onRegistered(resp.NodeID)
	}

	return resp, nil
}

// Stop 停止 Agent
func (a *Agent) Stop() {
	if a.nebulaMgr != nil {
		a.nebulaMgr.Stop()
	}
	if a.reporter != nil {
		a.reporter.Stop()
	}
	if a.conn != nil {
		a.conn.Close()
	}
	log.Printf("agent stopped")
}

// parseDuration 解析时长配置
func parseDuration(s string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}