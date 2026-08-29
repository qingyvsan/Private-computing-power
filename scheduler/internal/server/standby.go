package server

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "computing-power/proto/v1"
	"computing-power/scheduler/internal/store"
)

// StandbyRole 备节点角色状态
type StandbyRole string

const (
	RoleStandby StandbyRole = "standby"
	RoleActive  StandbyRole = "active"
)

// Standby 备节点同步客户端
type Standby struct {
	store      *store.Store
	primaryAddr string
	role       StandbyRole
	mu         sync.RWMutex

	healthCheckInterval time.Duration
	failoverTimeout     time.Duration
	lastSyncSeq         uint64

	conn   *grpc.ClientConn
	client pb.SyncServiceClient

	stopCh chan struct{}
}

// NewStandby 创建备节点
func NewStandby(st *store.Store, primaryAddr string, healthCheckInterval, failoverTimeout time.Duration) *Standby {
	return &Standby{
		store:               st,
		primaryAddr:          primaryAddr,
		role:                RoleStandby,
		healthCheckInterval: healthCheckInterval,
		failoverTimeout:     failoverTimeout,
		stopCh:              make(chan struct{}),
	}
}

// Start 启动同步循环
func (s *Standby) Start(ctx context.Context) {
	log.Printf("[standby] starting sync client, primary=%s", s.primaryAddr)

	// 连接 Primary
	if err := s.connect(); err != nil {
		log.Printf("[standby] connect to primary: %v", err)
		return
	}

	// 启动同步循环
	go s.syncLoop(ctx)

	// 启动健康检查
	go s.healthCheckLoop(ctx)
}

// connect 连接到 Primary 的 Sync 服务
func (s *Standby) connect() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, s.primaryAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.ForceCodecV2(pb.JSONCodec{})),
	)
	if err != nil {
		return err
	}
	s.conn = conn
	s.client = pb.NewSyncServiceClient(conn)
	log.Printf("[standby] connected to primary at %s", s.primaryAddr)
	return nil
}

// syncLoop 定期同步 WAL 条目
func (s *Standby) syncLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
			s.syncOnce(ctx)
		}
	}
}

// syncOnce 执行一次同步
func (s *Standby) syncOnce(ctx context.Context) {
	s.mu.RLock()
	if s.role != RoleStandby || s.client == nil {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()

	// 获取当前序列号
	lastSeq := s.store.GetLastSequence()

	// 如果本地已有序列号，从下一个开始同步
	reqSeq := lastSeq
	if lastSeq > 0 {
		// 从 Primary 获取最新序列号
		healthResp, err := s.client.HealthCheck(ctx, &pb.HealthCheckRequest{
			Timestamp: time.Now().UnixMilli(),
		})
		if err != nil {
			log.Printf("[standby] health check: %v", err)
			return
		}

		// 如果已经同步到最新，跳过
		if lastSeq >= healthResp.Sequence {
			return
		}
	}

	log.Printf("[standby] syncing from sequence %d", reqSeq)

	// 从 Primary 拉取 WAL 条目
	stream, err := s.client.SyncWAL(ctx, &pb.SyncWALRequest{
		LastSequence: reqSeq,
		StandbyID:    "standby-1",
	})
	if err != nil {
		log.Printf("[standby] SyncWAL: %v", err)
		return
	}

	// 回放 WAL 条目到本地 Store
	s.store.SetReplaying(true)
	defer s.store.SetReplaying(false)

	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}

		for _, entry := range resp.Entries {
			if err := replayEntry(s.store, entry); err != nil {
				log.Printf("[standby] replay entry seq=%d key=%s: %v", entry.Sequence, entry.Key, err)
				continue
			}
			if entry.Sequence > s.lastSyncSeq {
				s.lastSyncSeq = entry.Sequence
			}
		}

		if !resp.More {
			break
		}
	}

	log.Printf("[standby] sync complete, last_seq=%d", s.lastSyncSeq)
}

// healthCheckLoop 定期健康检查 Primary
func (s *Standby) healthCheckLoop(ctx context.Context) {
	failures := 0
	maxFailures := int(s.failoverTimeout / s.healthCheckInterval)
	if maxFailures < 1 {
		maxFailures = 3
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.healthCheckInterval):
			if s.client == nil {
				// 尝试重连
				if err := s.connect(); err != nil {
					log.Printf("[standby] reconnect: %v", err)
					failures++
				} else {
					failures = 0
				}
			} else {
				_, err := s.client.HealthCheck(ctx, &pb.HealthCheckRequest{
					Timestamp: time.Now().UnixMilli(),
				})
				if err != nil {
					failures++
					log.Printf("[standby] health check failed (%d/%d): %v", failures, maxFailures, err)
				} else {
					failures = 0
				}
			}

			if failures >= maxFailures {
				log.Printf("[standby] primary unreachable, promoting to active")
				s.promote()
				return
			}
		}
	}
}

// promote 提升为 Active 节点
func (s *Standby) promote() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.role = RoleActive
	log.Printf("[standby] promoted to active role")

	// 关闭连接
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
		s.client = nil
	}
}

// IsActive 返回是否已提升为 Active
func (s *Standby) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.role == RoleActive
}

// Role 返回当前角色
func (s *Standby) Role() StandbyRole {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.role
}

// Stop 停止 Standby
func (s *Standby) Stop() {
	close(s.stopCh)
	if s.conn != nil {
		s.conn.Close()
	}
}

// ========== WAL 回放 ==========

// replayEntry 回放一条 WAL 条目到 Store
func replayEntry(st *store.Store, entry *pb.SyncWALEntry) error {
	switch entry.Key {
	case "SaveNode":
		var node pb.Node
		if err := json.Unmarshal(entry.Data, &node); err != nil {
			return err
		}
		return st.SaveNode(&node)

	case "DeleteNode":
		return st.DeleteNode(string(entry.Data))

	case "SaveJob":
		var job pb.Job
		if err := json.Unmarshal(entry.Data, &job); err != nil {
			return err
		}
		return st.SaveJob(&job)

	case "DeleteJob":
		return st.DeleteJob(string(entry.Data))

	case "UpdateJobStatus":
		var args struct {
			JobID  string        `json:"job_id"`
			Status pb.JobStatus  `json:"status"`
		}
		if err := json.Unmarshal(entry.Data, &args); err != nil {
			return err
		}
		return st.UpdateJobStatus(args.JobID, args.Status)

	case "SaveUnit":
		var unit pb.Unit
		if err := json.Unmarshal(entry.Data, &unit); err != nil {
			return err
		}
		return st.SaveUnit(&unit)

	case "UpdateStageStatus":
		var args struct {
			StageID string          `json:"stage_id"`
			Status  pb.StageStatus  `json:"status"`
		}
		if err := json.Unmarshal(entry.Data, &args); err != nil {
			return err
		}
		return st.UpdateStageStatus(args.StageID, args.Status)

	case "SaveTrustEdge":
		var edge pb.TrustEdge
		if err := json.Unmarshal(entry.Data, &edge); err != nil {
			return err
		}
		return st.SaveTrustEdge(&edge)

	case "DeleteTrustEdge":
		var args struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := json.Unmarshal(entry.Data, &args); err != nil {
			return err
		}
		return st.DeleteTrustEdge(args.From, args.To)

	case "SaveIPAllocation":
		var args struct {
			NodeID string `json:"node_id"`
			IP     string `json:"ip"`
		}
		if err := json.Unmarshal(entry.Data, &args); err != nil {
			return err
		}
		return st.SaveIPAllocation(args.NodeID, args.IP)

	case "DeleteIPAllocation":
		return st.DeleteIPAllocation(string(entry.Data))

	default:
		log.Printf("[standby] unknown WAL entry key: %s", entry.Key)
		return nil
	}
}