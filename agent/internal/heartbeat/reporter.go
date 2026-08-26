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
}

// NewReporter 创建心跳上报器
func NewReporter(
	nodeID string,
	collector *Collector,
	client pb.SchedulerServiceClient,
	interval, jitter time.Duration,
	runningUnits func() []string,
) *Reporter {
	return &Reporter{
		nodeID:       nodeID,
		collector:    collector,
		client:       client,
		interval:     interval,
		jitter:       jitter,
		runningUnits: runningUnits,
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
				// TODO(P3): 处理调度器下发的命令
				for _, cmd := range result.resp.Commands {
					log.Printf("received command: %s", cmd.Type)
				}
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