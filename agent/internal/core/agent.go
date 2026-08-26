package core

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "computing-power/proto/v1"

	"computing-power/agent/internal/config"
	"computing-power/agent/internal/container"
	"computing-power/agent/internal/heartbeat"
)

// Agent 节点 Agent 生命周期管理器
type Agent struct {
	cfg       *config.Config
	nodeID    string
	conn      *grpc.ClientConn
	client    pb.SchedulerServiceClient
	collector *heartbeat.Collector
	runtime   container.Runtime
	tasks     map[string]string // unitID -> containerID
}

// New 创建节点 Agent
func New(cfg *config.Config) (*Agent, error) {
	a := &Agent{
		cfg:       cfg,
		tasks:     make(map[string]string),
		collector: heartbeat.NewCollector(cfg.Resources.ReportGPU, cfg.Resources.ReportNetwork),
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
	if err := a.register(ctx); err != nil {
		return fmt.Errorf("register node: %w", err)
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
			return a.runningUnits()
		},
	)

	initial := parseDuration(a.cfg.Scheduler.ReconnectInitial, time.Second)
	maxDelay := parseDuration(a.cfg.Scheduler.ReconnectMax, 60*time.Second)
	return reporter.StartWithRetry(ctx, conn, initial, maxDelay)
}

// register 注册节点到调度器
func (a *Agent) register(ctx context.Context) error {
	name := a.cfg.Agent.Name
	if name == "" {
		name = "agent-" + fmt.Sprintf("%d", time.Now().UnixMilli()%100000)
	}

	req := &pb.RegisterNodeRequest{
		Name:    name,
		Version: "0.1.0-dev",
	}

	resp, err := a.client.RegisterNode(ctx, req)
	if err != nil {
		return err
	}

	a.nodeID = resp.NodeID
	log.Printf("registered as node %s, overlay IP %s", resp.NodeID, resp.OverlayIP)
	return nil
}

// runningUnits 返回当前运行中的 Unit ID 列表
func (a *Agent) runningUnits() []string {
	var result []string
	for unitID := range a.tasks {
		result = append(result, unitID)
	}
	return result
}

// Stop 停止 Agent
func (a *Agent) Stop() {
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