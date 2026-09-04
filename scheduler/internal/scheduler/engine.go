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

	// 调度互斥锁：保证 scheduleOnce() 同时只有一个实例运行
	scheduleMu sync.Mutex

	// 唤醒通道：ScheduleNow() 通过非阻塞发送触发立即调度
	wakeCh chan struct{}

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
		wakeCh:          make(chan struct{}, 1),
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
			case <-e.wakeCh:
				// 被 ScheduleNow() 唤醒，执行一次立即调度
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
// 通过 wakeCh 通知调度循环，而非启动新的 goroutine，
// 从而避免 scheduleOnce() 被并发执行导致 unit 重复分配。
func (e *Engine) ScheduleNow() {
	select {
	case <-e.ctx.Done():
		return
	default:
	}
	select {
	case e.wakeCh <- struct{}{}:
	default:
		// 通道已满说明已有一次待处理的唤醒，无需重复发送
	}
}

// MaxRetries 返回最大重试次数
func (e *Engine) MaxRetries() int {
	return e.maxRetries
}

// scheduleOnce 执行一次完整的调度周期
func (e *Engine) scheduleOnce() {
	// 互斥锁：保证同一时间只有一个调度周期在执行，
	// 防止并发分配同一 unit 到多个节点。
	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()

	// 0. 回收孤立 Unit（节点离线后遗留的 Assigned/Running Unit）
	e.reclaimStaleUnits()

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

// ========== 孤立 Unit 回收 ==========

// reclaimStaleUnits 回收孤立 Unit（节点离线后遗留的 Assigned/Running Unit）
// 被回收的 Unit 标记为 Failed（"node offline"），随后走正常 retry 机制重新分配。
// 调用方需持有 scheduleMu 互斥锁。
func (e *Engine) reclaimStaleUnits() {
	// 获取所有 Assigned / Running 状态的 Unit
	staleUnits, err := e.listActiveUnits()
	if err != nil {
		log.Printf("engine: list active units for reclaim: %v", err)
		return
	}
	if len(staleUnits) == 0 {
		return
	}

	now := time.Now().UnixMilli()
	for _, u := range staleUnits {
		node := e.registry.GetNode(u.AssignedNode)
		// 节点不存在（已注销）或已离线 → 回收
		if node == nil || node.Status == pb.NodeStatusOffline {
			log.Printf("engine: reclaiming stale unit %s (node %s offline)", u.ID, u.AssignedNode)
			if _, err := e.store.UpdateUnitStatus(u.ID, pb.UnitStatusFailed, 0,
				"node offline, unit reclaimed at "+time.UnixMilli(now).Format("15:04:05")); err != nil {
				log.Printf("engine: reclaim unit %s: %v", u.ID, err)
			}
		}
	}
}

// listActiveUnits 列出所有 Assigned 和 Running 状态的 Unit
func (e *Engine) listActiveUnits() ([]*pb.Unit, error) {
	var result []*pb.Unit
	for _, status := range []pb.UnitStatus{pb.UnitStatusAssigned, pb.UnitStatusRunning} {
		units, err := e.store.ListUnitsByStatus(status)
		if err != nil {
			return nil, err
		}
		result = append(result, units...)
	}
	return result, nil
}