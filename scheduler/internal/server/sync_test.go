package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "computing-power/proto/v1"
	"computing-power/pkg/wal"
	"computing-power/scheduler/internal/store"
)

// TestCheckpointerSafeCleanup 验证 Checkpointer 只删除备机已确认回放的 WAL 文件
func TestCheckpointerSafeCleanup(t *testing.T) {
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	os.MkdirAll(walDir, 0755)

	// 创建 Store 并启用 WAL
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	// 使用极小 maxSize 强制频繁轮转，产生多个 WAL 文件
	w, err := wal.NewWriter(walDir, 64) // 每个条目约 100+ 字节，必然触发轮转
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	st.EnableWAL(w)

	// 写入若干 WAL 条目（会分布在多个文件中）
	var lastSeq uint64
	for i := 0; i < 10; i++ {
		u := &pb.Unit{ID: string(rune('a' + i)), JobID: "job-1", Status: pb.UnitStatusPending}
		if err := st.SaveUnit(u); err != nil {
			t.Fatalf("save unit %d: %v", i, err)
		}
	}
	lastSeq = st.GetLastSequence()
	if lastSeq < 10 {
		t.Fatalf("expected lastSeq >= 10, got %d", lastSeq)
	}
	// 确认确实产生了多个文件
	files, _ := os.ReadDir(walDir)
	if len(files) < 2 {
		t.Fatalf("expected >= 2 WAL files after rotation, got %d", len(files))
	}

	activeFile := func() uint64 { return w.CurrentFileSeq() }

	// 场景 1：无备机连接 → 不清理
	noAck := func() uint64 { return 0 }
	cp := NewCheckpointer(st, walDir, time.Minute, noAck, activeFile)
	if err := cp.CreateCheckpoint(); err != nil {
		t.Fatalf("checkpoint (no ack): %v", err)
	}
	files, _ = os.ReadDir(walDir)
	if len(files) == 0 {
		t.Fatalf("no ack: WAL files should NOT be cleaned")
	}

	// 场景 2：备机已确认到 lastSeq → 所有旧文件可清理（仅保留当前活动文件）
	allAck := func() uint64 { return lastSeq }
	cp2 := NewCheckpointer(st, walDir, time.Minute, allAck, activeFile)
	if err := cp2.CreateCheckpoint(); err != nil {
		t.Fatalf("checkpoint (all ack): %v", err)
	}
	files, _ = os.ReadDir(walDir)
	if len(files) != 1 {
		t.Fatalf("all ack: expected exactly 1 (active) WAL file, got %d", len(files))
	}

	// 场景 3：备机仅确认到中间位置 → 应保留所有包含未确认条目的文件
	midAck := lastSeq / 2
	midFn := func() uint64 { return midAck }
	cp3 := NewCheckpointer(st, walDir, time.Minute, midFn, activeFile)
	if err := cp3.CreateCheckpoint(); err != nil {
		t.Fatalf("checkpoint (mid ack): %v", err)
	}
	// 确认剩余的所有文件（除活跃文件外）最大序号都 > midAck
	reader, err := wal.NewReader(walDir)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}
	ranges, err := reader.FileRanges()
	if err != nil {
		t.Fatalf("file ranges: %v", err)
	}
	if len(ranges) == 0 {
		t.Fatalf("expected remaining WAL files")
	}
	for _, fr := range ranges {
		if fr.MaxSeq <= midAck {
			t.Fatalf("file %s (max_seq=%d) should have been cleaned (ack=%d)",
				filepath.Base(fr.Path), fr.MaxSeq, midAck)
		}
	}
}
