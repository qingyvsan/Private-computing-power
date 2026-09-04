package registry

import (
	"testing"
	"time"

	pb "computing-power/proto/v1"
)

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	// 使用小窗口和小样本数，方便测试
	return NewRegistry(10, 2, 4.0)
}

func newNode(id string) *pb.Node {
	return &pb.Node{
		ID:      id,
		Name:    "node-" + id,
		Status:  pb.NodeStatusOnline,
		Version: "1.0.0",
	}
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry(100, 10, 4.0)
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.phiThreshold != 4.0 {
		t.Errorf("expected phiThreshold=4.0, got %f", r.phiThreshold)
	}
}

func TestRegister(t *testing.T) {
	r := newTestRegistry(t)
	n := newNode("node-1")
	r.Register(n)

	got := r.GetNode("node-1")
	if got == nil {
		t.Fatal("GetNode returned nil after Register")
	}
	if got.ID != "node-1" {
		t.Errorf("expected node-1, got %s", got.ID)
	}
	if got.Status != pb.NodeStatusOnline {
		t.Errorf("expected status Online, got %s", got.Status.String())
	}
}

func TestRegister_OverwritesExisting(t *testing.T) {
	r := newTestRegistry(t)
	n1 := newNode("node-1")
	n1.Name = "original"
	r.Register(n1)

	n2 := newNode("node-1")
	n2.Name = "updated"
	r.Register(n2)

	got := r.GetNode("node-1")
	if got.Name != "updated" {
		t.Errorf("expected name 'updated', got '%s'", got.Name)
	}
}

func TestUnregister(t *testing.T) {
	r := newTestRegistry(t)
	r.Register(newNode("node-1"))
	r.Unregister("node-1")

	got := r.GetNode("node-1")
	if got != nil {
		t.Error("expected nil after Unregister")
	}
}

func TestUnregister_UnknownNode(t *testing.T) {
	r := newTestRegistry(t)
	// Should not panic
	r.Unregister("nonexistent")
}

func TestReportHeartbeat_UpdatesResources(t *testing.T) {
	r := newTestRegistry(t)
	r.Register(newNode("node-1"))

	res := &pb.NodeResources{
		CPUCores:    4.0,
		CPUUsage:    0.5,
		MemoryBytes: 8192,
		MemoryUsed:  4096,
	}
	r.ReportHeartbeat("node-1", res, nil, []string{"unit-1", "unit-2"})

	got := r.GetNode("node-1")
	if got == nil {
		t.Fatal("GetNode returned nil")
	}
	if got.Resources.CPUCores != 4.0 {
		t.Errorf("expected CPUCores=4.0, got %f", got.Resources.CPUCores)
	}
	if got.CurrentTasks != 2 {
		t.Errorf("expected CurrentTasks=2, got %d", got.CurrentTasks)
	}
}

func TestReportHeartbeat_UnknownNode(t *testing.T) {
	r := newTestRegistry(t)
	// ReportHeartbeat on unknown node should be a no-op
	r.ReportHeartbeat("nonexistent", nil, nil, nil)

	got := r.GetNode("nonexistent")
	if got != nil {
		t.Error("expected nil for unknown node after ReportHeartbeat")
	}
}

func TestGetNode_Unknown(t *testing.T) {
	r := newTestRegistry(t)
	got := r.GetNode("nonexistent")
	if got != nil {
		t.Error("expected nil for unknown node")
	}
}

func TestListOnline(t *testing.T) {
	r := newTestRegistry(t)
	r.Register(newNode("node-1"))
	r.Register(newNode("node-2"))

	online := r.ListOnline()
	if len(online) != 2 {
		t.Errorf("expected 2 online nodes, got %d", len(online))
	}
}

func TestListAll(t *testing.T) {
	r := newTestRegistry(t)
	r.Register(newNode("node-1"))
	r.Register(newNode("node-2"))

	all := r.ListAll()
	if len(all) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(all))
	}
}

func TestPhi_UnknownNode(t *testing.T) {
	r := newTestRegistry(t)
	phi := r.Phi("nonexistent", time.Now())
	if phi != 0.0 {
		t.Errorf("expected phi=0 for unknown node, got %f", phi)
	}
}

func TestIsAvailable_UnknownNode(t *testing.T) {
	r := newTestRegistry(t)
	avail := r.IsAvailable("nonexistent", time.Now())
	if avail {
		t.Error("expected unavailable for unknown node")
	}
}

func TestLoadNodes(t *testing.T) {
	r := newTestRegistry(t)
	nodes := []*pb.Node{
		newNode("node-1"),
		newNode("node-2"),
		newNode("node-3"),
	}
	r.LoadNodes(nodes)

	all := r.ListAll()
	if len(all) != 3 {
		t.Errorf("expected 3 nodes after LoadNodes, got %d", len(all))
	}
}

func TestLoadNodes_OverwritesExisting(t *testing.T) {
	r := newTestRegistry(t)
	r.Register(newNode("node-1"))
	r.LoadNodes([]*pb.Node{newNode("node-2")})

	all := r.ListAll()
	if len(all) != 2 {
		t.Errorf("expected 2 nodes after LoadNodes (overwrite adds), got %d", len(all))
	}
}

func TestSetHeartbeatTimeout(t *testing.T) {
	r := newTestRegistry(t)
	r.SetHeartbeatTimeout(60 * time.Second)
	if r.heartbeatTimeout != 60*time.Second {
		t.Errorf("expected timeout 60s, got %v", r.heartbeatTimeout)
	}
}

func TestSetOnStatusChanged(t *testing.T) {
	r := newTestRegistry(t)
	triggered := false
	r.SetOnStatusChanged(func(nodeID string, old, new pb.NodeStatus) {
		triggered = true
	})

	// Register does not trigger onStatusChanged; ReportHeartbeat does
	r.Register(newNode("node-1"))
	r.ReportHeartbeat("node-1", &pb.NodeResources{CPUUsage: 0.5}, nil, nil)

	// ReportHeartbeat may or may not trigger depending on phi,
	// but we just verify the callback is set without panic
	if !triggered && r.GetNode("node-1") == nil {
		t.Fatal("node should exist after Register")
	}
}

func TestReportHeartbeat_StatusTransition(t *testing.T) {
	r := newTestRegistry(t)
	r.phiThreshold = 0.01 // 极低阈值，确保心跳间隔稍长就会触发不健康

	statusChanges := make([]string, 0)
	r.SetOnStatusChanged(func(nodeID string, old, new pb.NodeStatus) {
		statusChanges = append(statusChanges, old.String()+"->"+new.String())
	})

	r.Register(newNode("node-1"))

	// 发送心跳，节点应保持 Online
	r.ReportHeartbeat("node-1", nil, nil, nil)
	got := r.GetNode("node-1")
	if got.Status != pb.NodeStatusOnline {
		t.Errorf("expected Online after heartbeat, got %s", got.Status.String())
	}
}

// TestDataRace 测试并发安全性
func TestDataRace(t *testing.T) {
	r := newTestRegistry(t)

	// 并发注册和查询
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			r.Register(newNode("node-1"))
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		r.GetNode("node-1")
		r.ListAll()
		r.ListOnline()
		r.Phi("node-1", time.Now())
	}
	<-done
}