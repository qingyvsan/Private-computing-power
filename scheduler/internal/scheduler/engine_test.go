package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "computing-power/proto/v1"
	"computing-power/pkg/trustgraph"
	"computing-power/scheduler/internal/registry"
	"computing-power/scheduler/internal/store"
)

func newTestEngineWithStore(t *testing.T) (*Engine, *store.Store, *registry.Registry, *trustgraph.Graph) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		st.Close()
		os.Remove(path)
	})
	reg := registry.NewRegistry(1000, 100, 4.0)
	trust := trustgraph.NewGraph()
	eng := New(st, reg, trust, 3, 5*time.Second, 10, ScoringWeights{
		ResourceMatch:  0.4,
		NetworkQuality: 0.3,
		Reputation:     0.2,
		Load:           0.1,
	})
	return eng, st, reg, trust
}

func TestEngine_StartStop(t *testing.T) {
	eng, _, _, _ := newTestEngineWithStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	eng.Start(ctx)
	// 等待一次调度
	time.Sleep(100 * time.Millisecond)
	eng.Stop()
	cancel()
}

func TestEngine_ScheduleNow(t *testing.T) {
	eng, _, _, _ := newTestEngineWithStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	eng.Start(ctx)
	eng.ScheduleNow()
	time.Sleep(100 * time.Millisecond)
	eng.Stop()
	cancel()
}

func TestEngine_PopCommands_Empty(t *testing.T) {
	eng, _, _, _ := newTestEngineWithStore(t)
	cmds := eng.PopCommands("nonexistent")
	if cmds != nil {
		t.Errorf("expected nil, got %v", cmds)
	}
}

func TestEngine_PushAndPopCommands(t *testing.T) {
	eng, _, _, _ := newTestEngineWithStore(t)
	eng.PushCommand("n1", &pb.Command{Type: "test", Payload: []byte("data")})
	cmds := eng.PopCommands("n1")
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Type != "test" {
		t.Errorf("expected type test, got %s", cmds[0].Type)
	}
	// 第二次 Pop 应为空
	cmds2 := eng.PopCommands("n1")
	if cmds2 != nil {
		t.Errorf("expected nil after pop, got %v", cmds2)
	}
}

func TestEngine_PushMultipleCommands(t *testing.T) {
	eng, _, _, _ := newTestEngineWithStore(t)
	eng.PushCommand("n1", &pb.Command{Type: "cmd1"})
	eng.PushCommand("n1", &pb.Command{Type: "cmd2"})
	cmds := eng.PopCommands("n1")
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
}

func TestEngine_MaxRetries(t *testing.T) {
	eng, _, _, _ := newTestEngineWithStore(t)
	if eng.MaxRetries() != 3 {
		t.Errorf("expected 3, got %d", eng.MaxRetries())
	}
}

func TestEngine_DefaultConstructor(t *testing.T) {
	eng := New(nil, nil, nil, 0, 0, 0, ScoringWeights{})
	if eng.maxConcurrent != 100 {
		t.Errorf("expected default 100, got %d", eng.maxConcurrent)
	}
	if eng.maxRetries != 3 {
		t.Errorf("expected default 3, got %d", eng.maxRetries)
	}
	if eng.reassignDelay != 5*time.Second {
		t.Errorf("expected default 5s, got %v", eng.reassignDelay)
	}
}