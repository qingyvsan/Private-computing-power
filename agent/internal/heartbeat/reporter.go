package heartbeat

import (
	"context"
	"log"
	"math/rand"
	"time"

	"google.golang.org/grpc"

	pb "computing-power/proto/v1"
)

// Reporter 心跳上报器
// 定期向调度器发送心跳，包含资源状态和运行中任务
type Reporter struct {
	nodeID       string
	collector    *Collector
	client       pb.SchedulerServiceClient
	stream       pb.Scheduler_HeartbeatClient
	interval     time.Duration
	jitter       time.Duration
	runningUnits func() []string
	commandHandler func(cmd *pb.Command)

	// 运行时状态
	lastStatus   pb.NodeStatus
	lastPhiValue float64
}

// NewReporter 创建心跳上报器
func NewReporter(
	nodeID string,
	collector *Collector,
	client pb.SchedulerServiceClient,
	interval, jitter time.Duration,
	runningUnits func() []string,
	commandHandler func(cmd *pb.Command),
) *Reporter {
	return &Reporter{
		nodeID:         nodeID,
		collector:      collector,
		client:         client,
		interval:       interval,
		jitter:         jitter,
		runningUnits:   runningUnits,
		commandHandler: commandHandler,
		lastStatus:     pb.NodeStatusOnline,
	}
}

// Start 开始心跳上报（阻塞）
func (r *Reporter) Start(ctx context.Context) error {
	// 建立双向流
	stream, err := r.client.Heartbeat(ctx)
	if err != nil {
		return err
	}
	r.stream = stream

	log.Printf("heartbeat stream established for node %s, interval %s", r.nodeID, r.interval)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		res := r.collector.Collect()
		req := &pb.HeartbeatRequest{
			NodeID:      r.nodeID,
			Resources:   res,
			RunningUnits: r.runningUnits(),
		}

		if err := stream.Send(req); err != nil {
			return err
		}

		// 接收调度器响应（非阻塞）
		type respResult struct {
			resp *pb.HeartbeatResponse
			err  error
		}
		respCh := make(chan respResult, 1)
		go func() {
			resp, err := stream.Recv()
			respCh <- respResult{resp, err}
		}()

		select {
		case result := <-respCh:
			if result.err != nil {
				return result.err
			}
			if result.resp != nil {
				r.processResponse(result.resp)
			}
		case <-time.After(500 * time.Millisecond):
			// 调度器暂时没有响应，继续
		}

		// 等待下一个心跳周期（带抖动，避免惊群）
		delay := r.interval
		if r.jitter > 0 {
			delay += time.Duration(rand.Int63n(int64(r.jitter)))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
}

// processResponse 处理调度器心跳响应
func (r *Reporter) processResponse(resp *pb.HeartbeatResponse) {
	// 记录状态变化
	if resp.NodeStatus != pb.NodeStatusUnspecified && resp.NodeStatus != r.lastStatus {
		log.Printf("node status changed by server: %s -> %s (phi=%.2f)",
			r.lastStatus, resp.NodeStatus, resp.PhiValue)
		r.lastStatus = resp.NodeStatus
	}

	// 记录 φ 值（仅变化超过 0.5 时打印，避免日志淹没）
	if resp.PhiValue > 0 && (resp.PhiValue-r.lastPhiValue > 0.5 || r.lastPhiValue-resp.PhiValue > 0.5) {
		log.Printf("heartbeat phi=%.2f status=%s samples=%s",
			resp.PhiValue, resp.NodeStatus, "N/A")
		r.lastPhiValue = resp.PhiValue
	}

	// 处理调度器下发的命令
	for _, cmd := range resp.Commands {
		if r.commandHandler != nil {
			r.commandHandler(cmd)
		}
	}

	// TODO(P3): 根据服务器建议调整心跳间隔
	// if resp.HeartbeatInterval != "" {
	// 	if newInterval, err := time.ParseDuration(resp.HeartbeatInterval); err == nil {
	// 		r.interval = newInterval
	// 	}
	// }
}

// StartWithRetry 带重连的心跳上报
func (r *Reporter) StartWithRetry(ctx context.Context, grpcConn *grpc.ClientConn, initial, max time.Duration) error {
	delay := initial
	for {
		err := r.Start(ctx)
		if ctx.Err() != nil {
			return nil
		}
		log.Printf("heartbeat stream lost, retrying in %s: %v", delay, err)
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