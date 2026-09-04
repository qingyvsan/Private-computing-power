package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	pb "computing-power/proto/v1"
	"computing-power/pkg/wal"
	"computing-power/scheduler/internal/store"
)

// SyncService 实现 WAL 热备同步服务（Primary 端）
type SyncService struct {
	store    *store.Store
	walDir   string
	role     string // "primary"
	leaderID string

	mu       sync.RWMutex
	// 各备机已确认回放的最后一个 WAL 条目序号
	// Checkpointer 依据该值安全清理旧 WAL 文件，避免删除备机未回放的条目。
	standbyAck map[string]uint64
}

// NewSyncService 创建同步服务
func NewSyncService(st *store.Store, walDir, leaderID string) *SyncService {
	return &SyncService{
		store:       st,
		walDir:      walDir,
		role:        "primary",
		leaderID:    leaderID,
		standbyAck:  make(map[string]uint64),
	}
}

// MinStandbyAck 返回所有备机已确认的最小回放序号
// 没有备机连接时返回 0（此时 Checkpointer 不做清理）。
func (s *SyncService) MinStandbyAck() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.standbyAck) == 0 {
		return 0
	}
	min := uint64(0)
	for _, seq := range s.standbyAck {
		if min == 0 || seq < min {
			min = seq
		}
	}
	return min
}

// SyncWAL 流式发送 WAL 条目（从指定序列号开始）
func (s *SyncService) SyncWAL(req *pb.SyncWALRequest, stream pb.SyncService_SyncWALServer) error {
	log.Printf("[sync] SyncWAL request: last_sequence=%d", req.LastSequence)

	// 记录备机已确认回放的序号，供 Checkpointer 安全清理参考
	if req.LastSequence > 0 {
		s.mu.Lock()
		s.standbyAck[req.StandbyID] = req.LastSequence
		s.mu.Unlock()
	}

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

	// minStandbyAck 返回所有备机已确认的最小回放序号
	// 没有备机连接时返回 0，此时不做 WAL 清理。
	minStandbyAck func() uint64
	// currentFileSeq 返回当前 WAL 写入器正在写入的文件序号
	// 清理时跳过该文件，避免删除活跃文件。
	currentFileSeq func() uint64
}

// NewCheckpointer 创建检查点管理器
func NewCheckpointer(st *store.Store, walDir string, interval time.Duration, minStandbyAck func() uint64, currentFileSeq func() uint64) *Checkpointer {
	return &Checkpointer{
		store:          st,
		walDir:         walDir,
		interval:       interval,
		stopCh:         make(chan struct{}),
		minStandbyAck:  minStandbyAck,
		currentFileSeq: currentFileSeq,
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

// CreateCheckpoint 创建 BoltDB 检查点并安全清理 WAL
// 安全清理规则：仅删除所有条目序号 ≤ minStandbyAck 的 WAL 文件，
// 确保备机不会因文件被清理而丢失未回放的条目。
func (cp *Checkpointer) CreateCheckpoint() error {
	checkpointDir := filepath.Join(cp.walDir, "..", "checkpoints")
	if err := os.MkdirAll(checkpointDir, 0755); err != nil {
		return err
	}

	checkpointPath := filepath.Join(checkpointDir, "snapshot-"+time.Now().Format("20060102-150405"))
	log.Printf("[checkpoint] creating checkpoint: %s", checkpointPath)

	// 使用 BoltDB 的 View + WriteTo 创建一致性快照
	if err := cp.store.Backup(checkpointPath); err != nil {
		return err
	}

	// 获取备机已确认的最小回放序号
	ackSeq := cp.minStandbyAck()
	if ackSeq == 0 {
		log.Printf("[checkpoint] no standby ack, skipping WAL cleanup")
		return nil
	}

	// 获取每个 WAL 文件的条目序号范围，仅删除最大值 ≤ ackSeq 的文件
	reader, err := wal.NewReader(cp.walDir)
	if err != nil {
		log.Printf("[checkpoint] create reader: %v", err)
		return nil
	}
	defer reader.Close()

	ranges, err := reader.FileRanges()
	if err != nil {
		log.Printf("[checkpoint] get file ranges: %v", err)
		return nil
	}

	activeFileSeq := uint64(0)
	if cp.currentFileSeq != nil {
		activeFileSeq = cp.currentFileSeq()
	}

	removed := 0
	for _, fr := range ranges {
		// 跳过当前活跃文件（正在写入，即使其条目全部 ≤ ack 也不应删除）
		if activeFileSeq > 0 && fileSeqOf(fr.Path) == activeFileSeq {
			continue
		}
		// 仅当文件的最后一条条目序号 ≤ 备机确认的序号时，该文件才可安全删除
		if fr.MaxSeq <= ackSeq {
			if err := os.Remove(fr.Path); err != nil {
				log.Printf("[checkpoint] remove %s: %v", fr.Path, err)
				continue
			}
			removed++
			log.Printf("[checkpoint] removed WAL file %s (max_seq=%d, ack=%d)",
				filepath.Base(fr.Path), fr.MaxSeq, ackSeq)
		}
	}

	log.Printf("[checkpoint] checkpoint created, removed %d WAL files (ack_seq=%d)", removed, ackSeq)
	return nil
}

// fileSeqOf 从 WAL 文件名中提取文件序号（如 "wal-000001.log" → 1）
func fileSeqOf(path string) uint64 {
	var seq uint64
	fmt.Sscanf(filepath.Base(path), "wal-%d.log", &seq)
	return seq
}

// Stop 停止检查点循环
func (cp *Checkpointer) Stop() {
	close(cp.stopCh)
}