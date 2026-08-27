package scheduler

import (
	"testing"

	pb "computing-power/proto/v1"
)

func TestScoreNode_AllDimensions(t *testing.T) {
	eng, _ := newTestEngine(t)
	node := makeNode("n1", pb.NodeStatusOnline, 0.3)
	spec := &pb.ResourceSpec{CPUCores: 2, MemoryBytes: 1024}

	score := eng.scoreNode(node, spec)
	if score <= 0 {
		t.Errorf("expected positive score, got %f", score)
	}
	// 各维度权重和应为 1.0
	if score > 1.0 {
		t.Errorf("expected score <= 1.0, got %f", score)
	}
}

func TestScoreNode_ZeroWeights(t *testing.T) {
	eng := New(nil, nil, nil, 3, 0, 10, ScoringWeights{})
	node := makeNode("n1", pb.NodeStatusOnline, 0.3)
	score := eng.scoreNode(node, nil)
	if score != 0 {
		t.Errorf("expected 0 with zero weights, got %f", score)
	}
}

func TestScoreNode_MissingNetwork(t *testing.T) {
	eng, _ := newTestEngine(t)
	node := makeNode("n1", pb.NodeStatusOnline, 0.3)
	node.Resources.Network = nil
	score := eng.scoreNode(node, nil)
	if score <= 0 {
		t.Errorf("expected positive score with default network, got %f", score)
	}
}

func TestScoreNode_NegativeReputation(t *testing.T) {
	eng, _ := newTestEngine(t)
	node := makeNode("n1", pb.NodeStatusOnline, 0.3)
	node.Reputation = -0.5
	score := eng.scoreNode(node, nil)
	if score < 0 {
		t.Errorf("expected non-negative score, got %f", score)
	}
}

func TestScoreNode_FullCPU(t *testing.T) {
	eng, _ := newTestEngine(t)
	node := makeNode("n1", pb.NodeStatusOnline, 1.0) // 100% CPU
	spec := &pb.ResourceSpec{CPUCores: 1, MemoryBytes: 1024}
	score := eng.scoreNode(node, spec)
	if score < 0 {
		t.Errorf("expected non-negative even with full cpu, got %f", score)
	}
}

func TestScoreNode_HighRTT(t *testing.T) {
	eng, _ := newTestEngine(t)
	node := makeNode("n1", pb.NodeStatusOnline, 0.3)
	node.Resources.Network.RTTMs = 1000 // 1s RTT
	score := eng.scoreNode(node, nil)
	if score < 0 {
		t.Errorf("expected non-negative with high RTT, got %f", score)
	}
}

func TestScoreAndRank_Ordering(t *testing.T) {
	eng, _ := newTestEngine(t)
	n1 := makeNode("fast", pb.NodeStatusOnline, 0.1)
	n2 := makeNode("slow", pb.NodeStatusOnline, 0.9)
	n1.Resources.Network.RTTMs = 5
	n2.Resources.Network.RTTMs = 200

	spec := &pb.ResourceSpec{CPUCores: 1, MemoryBytes: 512}
	ranked := eng.ScoreAndRank([]*pb.Node{n1, n2}, spec)
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked, got %d", len(ranked))
	}
	if ranked[0].Node.ID != "fast" {
		t.Errorf("expected fast node first, got %s", ranked[0].Node.ID)
	}
	if ranked[0].Score < ranked[1].Score {
		t.Errorf("expected first score >= second score: %f < %f", ranked[0].Score, ranked[1].Score)
	}
}

func TestScoreAndRank_Empty(t *testing.T) {
	eng, _ := newTestEngine(t)
	ranked := eng.ScoreAndRank(nil, nil)
	if ranked != nil {
		t.Errorf("expected nil, got %v", ranked)
	}
}

func TestScoreAndRank_ZeroScore(t *testing.T) {
	eng := New(nil, nil, nil, 3, 0, 10, ScoringWeights{})
	n1 := makeNode("n1", pb.NodeStatusOnline, 0.3)
	ranked := eng.ScoreAndRank([]*pb.Node{n1}, nil)
	if len(ranked) != 0 {
		t.Errorf("expected 0 nodes with zero weights, got %d", len(ranked))
	}
}