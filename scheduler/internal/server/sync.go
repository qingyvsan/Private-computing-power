package server

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	pb "computing-power/proto/v1"
	"computing-power/scheduler/internal/store"
)

// SyncService 实现 WAL 热备同步服务（Primary 端）
type SyncService struct {
	store    *store.Store
	walDir   string
	role     string // "primary"
	leaderID string
}

// NewSyncService 创建同步服务
func NewSyncService(st *store.Store, walDir, leaderID string) *SyncService {
	return &SyncService{
		store:    st,
		walDir:   walDir,
		role:     "primary",
		leaderID: leaderID,
	}
}

// SyncWAL 流式发送 WAL 条目（从指定序列号开始）
func (s *SyncService) SyncWAL(req *pb.SyncWALRequest, stream pb.SyncService_SyncWALServer) error {
	log.Printf("[sync] SyncWAL request: last_sequence=%d", req.LastSequence)

	lastSeq := req.LastSequence
	const batchSize = 100

	for {
		entries, err := s.store.ReadWALFrom(lastSeq + 1)
		if err != nil {
			log.Printf("[sync] ReadWALFrom(%d): %v", lastSeq+1, err)
			return err
		}

		if len(entries) == 0 {
			// 没有新条目，发送空响应并结束
			return stream.Send(&pb.SyncWALResponse{
				Entries: nil,
				More:    false,
			})
		}

		// 分批发送
		for i := 0; i < len(entries); i += batchSize {
			end := i + batchSize
			if end > len(entries) {
				end = len(entries)
			}
			batch := entries[i:end]

			var syncEntries []*pb.SyncWALEntry
			for _, e := range batch {
				syncEntries = append(syncEntries, &pb.SyncWALEntry{
					Sequence: e.Sequence,
					Type:     uint32(e.Type),
					Key:      e.Key,
					Data:     e.Data,
				})
				if e.Sequence > lastSeq {
					lastSeq = e.Sequence
				}
			}

			more := end < len(entries)
			if err := stream.Send(&pb.SyncWALResponse{
				Entries: syncEntries,
				More:    more,
			}); err != nil {
				return err
			}
		}

		// 如果已经读取了所有可用条目，等待并重试
		// 实际使用中，Standby 会持续轮询
		if len(entries) < batchSize {
			return nil
		}
	}
}

// HealthCheck 返回当前服务状态
func (s *SyncService) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Timestamp: time.Now().UnixMilli(),
		Role:      s.role,
		Sequence:  s.store.GetLastSequence(),
		LeaderID:  s.leaderID,
	}, nil
}

// ========== 检查点管理 ==========

// Checkpointer 管理 BoltDB 检查点和 WAL 清理
type Checkpointer struct {
	store    *store.Store
	walDir   string
	interval time.Duration
	stopCh   chan struct{}
}

// NewCheckpointer 创建检查点管理器
func NewCheckpointer(st *store.Store, walDir string, interval time.Duration) *Checkpointer {
	return &Checkpointer{
		store:    st,
		walDir:   walDir,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动定期检查点循环
func (cp *Checkpointer) Start(ctx context.Context) {
	if cp.interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(cp.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := cp.CreateCheckpoint(); err != nil {
					log.Printf("[checkpoint] create checkpoint: %v", err)
				}
			}
		}
	}()
	log.Printf("[checkpoint] started (interval=%s)", cp.interval)
}

// CreateCheckpoint 创建 BoltDB 检查点并清理 WAL
func (cp *Checkpointer) CreateCheckpoint() error {
	checkpointDir := filepath.Join(cp.walDir, "..", "checkpoints")
	if err := os.MkdirAll(checkpointDir, 0755); err != nil {
		return err
	}

	checkpointPath := filepath.Join(checkpointDir, "snapshot-"+time.Now().Format("20060102-150405"))
	log.Printf("[checkpoint] creating checkpoint: %s", checkpointPath)

	// 使用 BoltDB 的 View + WriteTo 创建一致性快照
	// 注意：store 需要导出 BoltDB 实例或提供快照方法
	// 这里通过创建文件副本方式实现简化版检查点
	if err := cp.store.Backup(checkpointPath); err != nil {
		return err
	}

	// 清理上次检查点之前的 WAL 文件
	keepAfter := time.Now().Add(-cp.interval * 2)
	entries, err := os.ReadDir(cp.walDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".log" {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(keepAfter) {
				os.Remove(filepath.Join(cp.walDir, e.Name()))
			}
		}
	}

	log.Printf("[checkpoint] checkpoint created, old WAL files cleaned")
	return nil
}

// Stop 停止检查点循环
func (cp *Checkpointer) Stop() {
	close(cp.stopCh)
}