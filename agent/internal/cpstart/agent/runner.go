package agent

import (
	"context"
	"log"
	"sync"
	"sync/atomic"

	pb "computing-power/proto/v1"

	"computing-power/agent/internal/config"
	agentcore "computing-power/agent/internal/core"
	"computing-power/agent/internal/heartbeat"
	"computing-power/agent/internal/container"
	cpstartcfg "computing-power/agent/internal/cpstart/config"
)

// AgentStatus 运行状态
type AgentStatus int32

const (
	AgentStopped  AgentStatus = 0
	AgentStarting AgentStatus = 1
	AgentRunning  AgentStatus = 2
	AgentError    AgentStatus = 3
)

func (s AgentStatus) String() string {
	switch s {
	case AgentStopped:
		return "stopped"
	case AgentStarting:
		return "starting"
	case AgentRunning:
		return "running"
	case AgentError:
		return "error"
	default:
		return "unknown"
	}
}

// Runner Agent 运行期管理器
type Runner struct {
	cfg    *cpstartcfg.Config
	agent  *agentcore.Agent
	cancel context.CancelFunc
	done   chan struct{}

	nodeID atomic.Value  // string
	status atomic.Int32  // AgentStatus
	col    *heartbeat.Collector

	mu    sync.RWMutex
	lastErr error
}

// NewRunner 创建 Agent 运行器
func NewRunner(cfg *cpstartcfg.Config) *Runner {
	return &Runner{
		cfg:  cfg,
		done: make(chan struct{}),
		col:  heartbeat.NewCollector(cfg.Resources.ReportGPU, true, nil, cfg.Scheduler.Address),
	}
}

// Start 启动 Agent（后台运行）
func (r *Runner) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status.Load() == int32(AgentRunning) || r.status.Load() == int32(AgentStarting) {
		return nil
	}

	r.status.Store(int32(AgentStarting))
	r.done = make(chan struct{}) // 重置 done channel，防止重复 close
	ac := r.cfg.ToAgentConfig()

	// 处理资源限制
	r.applyResourceLimits(ac)

	agent, err := agentcore.New(ac)
	if err != nil {
		r.status.Store(int32(AgentError))
		r.lastErr = err
		return err
	}

	// 捕获注册后的节点 ID
	agent.SetOnRegistered(func(nodeID string) {
		r.nodeID.Store(nodeID)
	})

	r.agent = agent

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel

	go r.run(ctx)
	return nil
}

// run 在后台 goroutine 中运行 Agent
func (r *Runner) run(ctx context.Context) {
	done := r.done // 捕获启动时的 channel，防止 Start() 重置后误关闭新 channel
	defer func() {
		r.status.Store(int32(AgentStopped))
		close(done)
	}()

	log.Printf("cpstart: starting agent (scheduler=%s, name=%s)", r.cfg.Scheduler.Address, r.cfg.Agent.Name)
	r.status.Store(int32(AgentRunning))

	if err := r.agent.Start(ctx); err != nil {
		log.Printf("cpstart: agent stopped with error: %v", err)
		r.mu.Lock()
		r.lastErr = err
		r.mu.Unlock()
		r.status.Store(int32(AgentError))
	} else {
		log.Printf("cpstart: agent stopped gracefully")
	}
}

// Stop 停止 Agent
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cancel != nil {
		r.cancel()
	}
	if r.agent != nil {
		r.agent.Stop()
	}
}

// Wait 等待 Agent 退出
func (r *Runner) Wait() {
	<-r.done
}

// Status 返回当前状态
func (r *Runner) Status() AgentStatus {
	return AgentStatus(r.status.Load())
}

// NodeID 返回注册的节点 ID
func (r *Runner) NodeID() string {
	if id, ok := r.nodeID.Load().(string); ok {
		return id
	}
	return ""
}

// LastError 返回最后一次错误
func (r *Runner) LastError() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastErr
}

// SetRuntime 替换运行中 agent 的容器运行时（例如 WSL2 代理就绪后）
func (r *Runner) SetRuntime(rt container.Runtime) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.agent != nil {
		r.agent.SetRuntime(rt)
	}
}

// LocalResources 返回本地资源信息
func (r *Runner) LocalResources() *pb.NodeResources {
	return r.col.Collect()
}

// applyResourceLimits 将 cpstart 资源限制应用到 agent 配置
func (r *Runner) applyResourceLimits(ac *config.Config) {
	if r.cfg.Resources.MaxCPUCores > 0 {
		ac.Resources.MaxCPUCores = r.cfg.Resources.MaxCPUCores
	}
	if r.cfg.Resources.MaxMemoryMB > 0 {
		ac.Resources.MaxMemoryMB = r.cfg.Resources.MaxMemoryMB
	}
}