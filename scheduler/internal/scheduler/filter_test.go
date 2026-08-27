package scheduler

import (
	"testing"

	pb "computing-power/proto/v1"
	"computing-power/pkg/trustgraph"
)

func newTestEngine(t *testing.T) (*Engine, *trustgraph.Graph) {
	t.Helper()
	trust := trustgraph.NewGraph()
	eng := New(nil, nil, trust, 3, 0, 10, ScoringWeights{
		ResourceMatch:  0.4,
		NetworkQuality: 0.3,
		Reputation:     0.2,
		Load:           0.1,
	})
	return eng, trust
}

func makeNode(id string, status pb.NodeStatus, cpuUsage float64) *pb.Node {
	return &pb.Node{
		ID:     id,
		Status: status,
		Resources: &pb.NodeResources{
			CPUCores:  8,
			CPUUsage:  cpuUsage,
			MemoryBytes: 16384,
			MemoryUsed:  4096,
			DiskBytes:   512000,
			DiskUsed:    128000,
			Network: &pb.NetworkMetrics{
				RTTMs: 10,
			},
		},
		Reputation:   1.0,
		CurrentTasks: 0,
		MaxTasks:     10,
		BlockList:    nil,
	}
}

// ========== filterOnline ==========

func TestFilterOnline_KeepsOnlineAndBusy(t *testing.T) {
	eng, _ := newTestEngine(t)
	nodes := []*pb.Node{
		makeNode("n1", pb.NodeStatusOnline, 0.3),
		makeNode("n2", pb.NodeStatusBusy, 0.9),
		makeNode("n3", pb.NodeStatusOffline, 0),
		makeNode("n4", pb.NodeStatusUnhealthy, 0),
		makeNode("n5", pb.NodeStatusUnspecified, 0),
	}
	result := eng.filterOnline(nodes)
	if len(result) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result))
	}
	if result[0].ID != "n1" || result[1].ID != "n2" {
		t.Errorf("expected n1,n2, got %s,%s", result[0].ID, result[1].ID)
	}
}

func TestFilterOnline_EmptyInput(t *testing.T) {
	eng, _ := newTestEngine(t)
	result := eng.filterOnline(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestFilterOnline_AllOffline(t *testing.T) {
	eng, _ := newTestEngine(t)
	nodes := []*pb.Node{
		makeNode("n1", pb.NodeStatusOffline, 0),
		makeNode("n2", pb.NodeStatusUnhealthy, 0),
	}
	result := eng.filterOnline(nodes)
	if len(result) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result))
	}
}

// ========== filterResources ==========

func TestFilterResources_AllPass(t *testing.T) {
	eng, _ := newTestEngine(t)
	nodes := []*pb.Node{makeNode("n1", pb.NodeStatusOnline, 0.3)}
	spec := &pb.ResourceSpec{
		CPUCores:    2,
		MemoryBytes: 1024,
	}
	result := eng.filterResources(nodes, spec)
	if len(result) != 1 {
		t.Fatalf("expected 1 node, got %d", len(result))
	}
}

func TestFilterResources_InsufficientCPU(t *testing.T) {
	eng, _ := newTestEngine(t)
	nodes := []*pb.Node{makeNode("n1", pb.NodeStatusOnline, 0.3)}
	// 请求 16 核，但节点只有 8 核，且已用 30%，可用 ~5.6 核
	spec := &pb.ResourceSpec{
		CPUCores:    16,
		MemoryBytes: 1024,
	}
	result := eng.filterResources(nodes, spec)
	if len(result) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result))
	}
}

func TestFilterResources_NilSpec(t *testing.T) {
	eng, _ := newTestEngine(t)
	nodes := []*pb.Node{makeNode("n1", pb.NodeStatusOnline, 0.3)}
	result := eng.filterResources(nodes, nil)
	if len(result) != 1 {
		t.Errorf("expected 1 node, got %d", len(result))
	}
}

func TestFilterResources_NilNodeResources(t *testing.T) {
	eng, _ := newTestEngine(t)
	node := makeNode("n1", pb.NodeStatusOnline, 0.3)
	node.Resources = nil
	nodes := []*pb.Node{node}
	spec := &pb.ResourceSpec{CPUCores: 1}
	result := eng.filterResources(nodes, spec)
	if len(result) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result))
	}
}

// ========== filterTrust ==========

func TestFilterTrust_SelfOwner(t *testing.T) {
	eng, _ := newTestEngine(t)
	nodes := []*pb.Node{makeNode("n1", pb.NodeStatusOnline, 0.3)}
	result := eng.filterTrust(nodes, "n1")
	if len(result) != 1 {
		t.Errorf("expected 1 node (self), got %d", len(result))
	}
}

func TestFilterTrust_NotReachable(t *testing.T) {
	eng, _ := newTestEngine(t)
	nodes := []*pb.Node{
		makeNode("n1", pb.NodeStatusOnline, 0.3),
		makeNode("n2", pb.NodeStatusOnline, 0.3),
	}
	// owner=n1, n2 不可达（信任图为空）
	result := eng.filterTrust(nodes, "n1")
	if len(result) != 1 {
		t.Errorf("expected 1 node (self only), got %d", len(result))
	}
}

func TestFilterTrust_Reachable(t *testing.T) {
	eng, trust := newTestEngine(t)
	trust.AddEdge("owner", "n1", nil, nil)
	nodes := []*pb.Node{
		makeNode("owner", pb.NodeStatusOnline, 0.3),
		makeNode("n1", pb.NodeStatusOnline, 0.3),
		makeNode("n2", pb.NodeStatusOnline, 0.3),
	}
	result := eng.filterTrust(nodes, "owner")
	if len(result) != 2 {
		t.Errorf("expected 2 nodes (self + n1), got %d", len(result))
	}
}

func TestFilterTrust_EmptyOwner(t *testing.T) {
	eng, _ := newTestEngine(t)
	nodes := []*pb.Node{makeNode("n1", pb.NodeStatusOnline, 0.3)}
	result := eng.filterTrust(nodes, "")
	if len(result) != 1 {
		t.Errorf("expected 1 node, got %d", len(result))
	}
}

// ========== filterBlocklist ==========

func TestFilterBlocklist_NoBlocklist(t *testing.T) {
	eng, _ := newTestEngine(t)
	// 需要 registry 中有 owner 节点
	// 但这里测试时 eng.registry 为 nil，得跳过
	// 只测试空输入场景
	result := eng.filterBlocklist(nil, "owner")
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

// ========== contains ==========

func TestContains(t *testing.T) {
	list := []string{"a", "b", "c"}
	if !contains(list, "a") {
		t.Error("expected contains a")
	}
	if contains(list, "d") {
		t.Error("expected not contains d")
	}
	if contains(nil, "a") {
		t.Error("expected not contains in nil")
	}
}

// ========== Filter pipeline ==========

func TestFilterPipeline_Integration(t *testing.T) {
	eng, trust := newTestEngine(t)
	trust.AddEdge("owner", "n2", nil, nil)

	nodes := []*pb.Node{
		makeNode("owner", pb.NodeStatusOnline, 0.3),
		makeNode("n2", pb.NodeStatusOnline, 0.3),
		makeNode("n3", pb.NodeStatusOffline, 0),
		makeNode("n4", pb.NodeStatusOnline, 0.3),
	}
	spec := &pb.ResourceSpec{CPUCores: 1, MemoryBytes: 512}
	result := eng.Filter(nodes, spec, "owner")
	// owner 自调度 + n2 信任可达
	if len(result) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result))
	}
}