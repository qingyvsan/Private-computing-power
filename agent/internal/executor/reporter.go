package executor

import (
	"context"
	"log"
	"sync"
	"time"

	pb "computing-power/proto/v1"
)

// Reporter 管理 ReportUnitStatus 双向流
// 向调度器上报单元状态变更并接收确认
type Reporter struct {
	nodeID string
	client pb.SchedulerServiceClient

	mu     sync.RWMutex
	stream pb.Scheduler_ReportUnitStatusClient
	stopCh chan struct{}
}

// NewReporter 创建状态上报器
func NewReporter(nodeID string, client pb.SchedulerServiceClient) *Reporter {
	return &Reporter{
		nodeID: nodeID,
		client: client,
		stopCh: make(chan struct{}),
	}
}

// Start 打开状态上报流并在后台接收确认
// 阻塞直到 ctx 取消或流不可恢复
func (r *Reporter) Start(ctx context.Context) error {
	if r.client == nil {
		return nil
	}

	// 建立双向流
	stream, err := r.client.ReportUnitStatus(ctx)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.stream = stream
	r.mu.Unlock()

	log.Printf("unit status report stream established for node %s", r.nodeID)

	// 后台接收确认，防止流阻塞
	ackCh := make(chan struct{})
	go func() {
		defer close(ackCh)
		for {
			_, err := stream.Recv()
			if err != nil {
				return
			}
			// ack 仅用于防止流背压，不处理具体内容
		}
	}()

	// 等待 ctx 取消或流结束
	select {
	case <-ctx.Done():
		return nil
	case <-ackCh:
		return nil
	}
}

// StartWithRetry 带自动重连的状态上报
func (r *Reporter) StartWithRetry(ctx context.Context, initial, max time.Duration) error {
	delay := initial
	for {
		err := r.Start(ctx)
		if ctx.Err() != nil {
			return nil
		}
		log.Printf("unit status report stream lost, retrying in %s: %v", delay, err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
		delay *= 2
		if delay > max {
			delay = max
		}
	}
}

// Report 发送单元状态报告
// 如果流不可用则返回错误
func (r *Reporter) Report(unitID string, status pb.UnitStatus, exitCode int32, errMsg string, output []byte) error {
	r.mu.RLock()
	stream := r.stream
	r.mu.RUnlock()

	if stream == nil {
		return nil
	}

	report := &pb.UnitStatusReport{
		UnitID:       unitID,
		NodeID:       r.nodeID,
		Status:       status,
		ExitCode:     exitCode,
		ErrorMessage: errMsg,
		Output:       output,
	}

	return stream.Send(report)
}

// Stop 关闭流
func (r *Reporter) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stream != nil {
		r.stream.CloseSend()
		r.stream = nil
	}
}