package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	pb "computing-power/proto/v1"
	"computing-power/scheduler/internal/registry"
	"computing-power/scheduler/internal/store"

	"computing-power/pkg/trustgraph"
)

// ScoringWeights 节点评分各维度权重
type ScoringWeights struct {
	ResourceMatch  float64
	NetworkQuality float64
	Reputation     float64
	Load           float64
}

// Engine 调度引擎，负责将 Pending 状态的 Unit 自动分配给最合适的节点
type Engine struct {
	store    *store.Store
	registry *registry.Registry
	trust    *trustgraph.Graph

	maxRetries     int
	reassignDelay  time.Duration
	maxConcurrent  int
	weights        ScoringWeights

	// push 分发命令队列（nodeID → commands）
	pendingCommands map[string][]*pb.Command
	commandsMu      sync.Mutex

	// 并发分配信号量
	sem chan struct{}

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// New 创建调度引擎
func New(st *store.Store, reg *registry.Registry, trust *trustgraph.Graph,
	maxRetries int, reassignDelay time.Duration, maxConcurrent int,
	weights ScoringWeights) *Engine {

	if maxConcurrent <= 0 {
		maxConcurrent = 100
	}
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if reassignDelay <= 0 {
		reassignDelay = 5 * time.Second
	}

	return &Engine{
		store:           st,
		registry:        reg,
		trust:           trust,
		maxRetries:      maxRetries,
		reassignDelay:   reassignDelay,
		maxConcurrent:   maxConcurrent,
		weights:         weights,
		pendingCommands: make(map[string][]*pb.Command),
		sem:             make(chan struct{}, maxConcurrent),
		ctx:             context.Background(),
	}
}

// Start 启动调度循环（周期 5s）
func (e *Engine) Start(ctx context.Context) {
	e.ctx, e.cancel = context.WithCancel(ctx)
	e.done = make(chan struct{})
	go func() {
		defer close(e.done)
		// 先立即执行一次
		e.scheduleOnce()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-e.ctx.Done():
				return
			case <-ticker.C:
				e.scheduleOnce()
			}
		}
	}()
	log.Printf("scheduling engine started (tick=5s, concurrent=%d, max_retries=%d)",
		e.maxConcurrent, e.maxRetries)
}

// Stop 停止调度循环
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	<-e.done
}

// ScheduleNow 触发立即调度（异步非阻塞）
func (e *Engine) ScheduleNow() {
	select {
	case <-e.ctx.Done():
		return
	default:
	}
	go e.scheduleOnce()
}

// MaxRetries 返回最大重试次数
func (e *Engine) MaxRetries() int {
	return e.maxRetries
}

// scheduleOnce 执行一次完整的调度周期
func (e *Engine) scheduleOnce() {
	// 1. 获取所有 Pending Unit
	pendingUnits, err := e.store.ListUnitsByStatus(pb.UnitStatusPending)
	if err != nil {
		log.Printf("engine: list pending units: %v", err)
		return
	}

	// 2. 获取可重试的 Failed Unit
	retryUnits, err := e.loadRetryEligibleUnits()
	if err != nil {
		log.Printf("engine: list retry units: %v", err)
	}
	if len(retryUnits) > 0 {
		log.Printf("engine: %d units eligible for retry", len(retryUnits))
	}

	// 合并 Pending 和可重试的 Unit
	allUnits := append(pendingUnits, retryUnits...)
	if len(allUnits) == 0 {
		return
	}

	// 3. 按 JobID 分组
	unitsByJob := make(map[string][]*pb.Unit)
	for _, u := range allUnits {
		unitsByJob[u.JobID] = append(unitsByJob[u.JobID], u)
	}

	// 4. 逐 Job 调度
	for jobID, units := range unitsByJob {
		job, err := e.store.GetJob(jobID)
		if err != nil || job == nil {
			log.Printf("engine: get job %s: %v", jobID, err)
			continue
		}
		// 跳过已结束的 Job
		if job.Status != pb.JobStatusPending && job.Status != pb.JobStatusRunning {
			continue
		}
		e.scheduleJob(job, units)
	}
}

// ========== 命令队列（push 分发） ==========

// PushCommand 为节点添加待下发命令
func (e *Engine) PushCommand(nodeID string, cmd *pb.Command) {
	e.commandsMu.Lock()
	defer e.commandsMu.Unlock()
	e.pendingCommands[nodeID] = append(e.pendingCommands[nodeID], cmd)
}

// PopCommands 取出并清除节点的所有待下发命令（Heartbeat 处理程序调用）
func (e *Engine) PopCommands(nodeID string) []*pb.Command {
	e.commandsMu.Lock()
	defer e.commandsMu.Unlock()
	cmds := e.pendingCommands[nodeID]
	delete(e.pendingCommands, nodeID)
	return cmds
}