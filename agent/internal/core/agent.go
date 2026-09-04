package core

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

// SetRuntime 在 agent 运行中替换容器运行时（例如 WSL2 代理就绪后）。
// 心跳 capability 闭包引用 a.runtime，替换后下次心跳自动上报 container 能力。
func (a *Agent) SetRuntime(rt container.Runtime) {
	a.runtime = rt
	if a.exec != nil {
		a.exec.SetRuntime(rt)
	}
	log.Printf("agent: container runtime updated")
}

// Start 启动 Agent
func (a *Agent) Start(ctx context.Context) error {
	// 构建 TLS 凭证
	tlsCreds, err := a.buildTLSCredentials()
	if err != nil {
		return fmt.Errorf("build TLS credentials: %w", err)
	}

	// 建立到调度器的连接
	var dialOpts []grpc.DialOption
	if tlsCreds != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(tlsCreds))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	dialOpts = append(dialOpts, grpc.WithDefaultCallOptions(grpc.ForceCodecV2(pb.JSONCodec{})))

	conn, err := grpc.NewClient(a.cfg.Scheduler.Address, dialOpts...)
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

	// 如果注册返回了 gRPC 客户端证书，保存到磁盘并重新连接
	if resp != nil && len(resp.GrpcCertificate) > 0 && len(resp.GrpcPrivateKey) > 0 {
		if err := a.saveGRPCCerts(resp); err != nil {
			log.Printf("save gRPC certs: %v (continuing without mTLS)", err)
		} else {
			log.Printf("gRPC mTLS client certificate saved, reconnecting...")
			a.conn.Close()
			if err := a.reconnectWithMTLS(ctx); err != nil {
				return fmt.Errorf("reconnect with mTLS: %w", err)
			}
		}
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

	a.exec = executor.NewExecutor(a.runtime, a.manager, a.reporter, hamiMgr, a.cfg.Resources.MaxCPUCores, a.cfg.Resources.MaxMemoryMB)

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
	// 心跳上报容器运行时能力（节点启动后运行时可能变为可用）
	reporter.SetCapabilities(func() []string {
		if a.runtime != nil && a.runtime.IsAvailable() {
			return []string{"container"}
		}
		return nil
	})

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

	// 上报容器运行时能力 + 初始资源
	if a.runtime != nil && a.runtime.IsAvailable() {
		req.Capabilities = []string{"container"}
	}
	req.InitialResources = a.collector.Collect()

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

// buildTLSCredentials 从配置构建 TLS 凭证
// 如果 CA 证书路径为空，返回 nil（使用不安全连接）
// 如果客户端证书路径为空，仅验证服务器证书（引导模式）
func (a *Agent) buildTLSCredentials() (credentials.TransportCredentials, error) {
	caCertPath := a.cfg.Scheduler.CACert
	if caCertPath == "" {
		return nil, nil
	}

	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCertPEM) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	tlsConfig := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
	}

	// 如果存在客户端证书，加载它
	clientCertPath := a.cfg.Scheduler.TLSCert
	clientKeyPath := a.cfg.Scheduler.TLSKey
	if clientCertPath != "" && clientKeyPath != "" {
		if _, err := os.Stat(clientCertPath); err == nil {
			if _, err := os.Stat(clientKeyPath); err == nil {
				clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
				if err != nil {
					log.Printf("load client cert: %v (will bootstrap)", err)
				} else {
					tlsConfig.Certificates = []tls.Certificate{clientCert}
				}
			}
		}
	}

	// 从地址解析 ServerName
	host, _, err := net.SplitHostPort(a.cfg.Scheduler.Address)
	if err == nil {
		tlsConfig.ServerName = host
	}

	return credentials.NewTLS(tlsConfig), nil
}

// saveGRPCCerts 保存 gRPC 客户端证书到磁盘
func (a *Agent) saveGRPCCerts(resp *pb.RegisterNodeResponse) error {
	certDir := filepath.Join(a.cfg.Agent.DataDir, "grpc")
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return fmt.Errorf("create cert dir: %w", err)
	}

	caPath := filepath.Join(certDir, "ca.crt")
	certPath := filepath.Join(certDir, "client.crt")
	keyPath := filepath.Join(certDir, "client.key")

	if err := os.WriteFile(caPath, resp.CACertificate, 0644); err != nil {
		return fmt.Errorf("write CA cert: %w", err)
	}
	if err := os.WriteFile(certPath, resp.GrpcCertificate, 0644); err != nil {
		return fmt.Errorf("write client cert: %w", err)
	}
	if err := os.WriteFile(keyPath, resp.GrpcPrivateKey, 0600); err != nil {
		return fmt.Errorf("write client key: %w", err)
	}

	// 更新配置路径，以便下次启动时自动加载
	a.cfg.Scheduler.CACert = caPath
	a.cfg.Scheduler.TLSCert = certPath
	a.cfg.Scheduler.TLSKey = keyPath

	log.Printf("gRPC mTLS certs saved to %s", certDir)
	return nil
}

// reconnectWithMTLS 使用已保存的客户端证书重新连接
func (a *Agent) reconnectWithMTLS(ctx context.Context) error {
	tlsCreds, err := a.buildTLSCredentials()
	if err != nil {
		return fmt.Errorf("build mTLS credentials: %w", err)
	}
	if tlsCreds == nil {
		return fmt.Errorf("no TLS credentials available for reconnection")
	}

	conn, err := grpc.NewClient(
		a.cfg.Scheduler.Address,
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithDefaultCallOptions(grpc.ForceCodecV2(pb.JSONCodec{})),
	)
	if err != nil {
		return fmt.Errorf("connect with mTLS: %w", err)
	}

	a.conn = conn
	a.client = pb.NewSchedulerServiceClient(conn)
	log.Printf("reconnected to scheduler with mTLS")
	return nil
}

// parseDuration 解析时长配置
func parseDuration(s string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}