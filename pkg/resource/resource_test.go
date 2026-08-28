package resource

import (
	"testing"

	pb "computing-power/proto/v1"
)

func TestFits_CPU(t *testing.T) {
	node := &pb.NodeResources{CPUCores: 8, CPUUsage: 0.25}
	req := &pb.ResourceSpec{CPUCores: 4}
	if !Fits(node, req) {
		t.Error("expected 4 CPU to fit on 8 cores with 25% usage (6 avail)")
	}
	reqLarge := &pb.ResourceSpec{CPUCores: 10}
	if Fits(node, reqLarge) {
		t.Error("expected 10 CPU not to fit on 8 cores with 25% usage (6 avail)")
	}
}

func TestFits_Memory(t *testing.T) {
	node := &pb.NodeResources{MemoryBytes: 16 * 1024 * 1024 * 1024, MemoryUsed: 4 * 1024 * 1024 * 1024}
	req := &pb.ResourceSpec{MemoryBytes: 8 * 1024 * 1024 * 1024}
	if !Fits(node, req) {
		t.Error("expected 8GB to fit on 16GB with 4GB used (12GB avail)")
	}
	reqLarge := &pb.ResourceSpec{MemoryBytes: 20 * 1024 * 1024 * 1024}
	if Fits(node, reqLarge) {
		t.Error("expected 20GB not to fit on 16GB with 4GB used (12GB avail)")
	}
}

func TestFits_GPU_Enough(t *testing.T) {
	node := &pb.NodeResources{
		GPUs: []*pb.GPUDevice{
			{UUID: "GPU-aaa", MemoryAvailMB: 24576},
			{UUID: "GPU-bbb", MemoryAvailMB: 10240},
		},
	}
	req := &pb.ResourceSpec{
		GPUs: []*pb.GPURequest{
			{MemoryMB: 8192, Cores: 100, Count: 1},
		},
	}
	if !Fits(node, req) {
		t.Error("expected 1x8GB GPU to fit")
	}
}

func TestFits_GPU_Insufficient(t *testing.T) {
	node := &pb.NodeResources{
		GPUs: []*pb.GPUDevice{
			{UUID: "GPU-aaa", MemoryAvailMB: 4096},
		},
	}
	req := &pb.ResourceSpec{
		GPUs: []*pb.GPURequest{
			{MemoryMB: 8192, Cores: 100, Count: 1},
		},
	}
	if Fits(node, req) {
		t.Error("expected 1x8GB GPU not to fit when only 4GB available")
	}
}

func TestFits_GPU_Multiple(t *testing.T) {
	node := &pb.NodeResources{
		GPUs: []*pb.GPUDevice{
			{UUID: "GPU-aaa", MemoryAvailMB: 24576},
			{UUID: "GPU-bbb", MemoryAvailMB: 24576},
		},
	}
	req := &pb.ResourceSpec{
		GPUs: []*pb.GPURequest{
			{MemoryMB: 8192, Cores: 100, Count: 2},
		},
	}
	if !Fits(node, req) {
		t.Error("expected 2x8GB GPUs to fit on 2 GPUs with 24GB each")
	}
}

func TestFits_GPU_MaxMemCheck(t *testing.T) {
	// 确保至少有一个 GPU 能满足最大请求内存
	node := &pb.NodeResources{
		GPUs: []*pb.GPUDevice{
			{UUID: "GPU-aaa", MemoryAvailMB: 4096},
			{UUID: "GPU-bbb", MemoryAvailMB: 4096},
		},
	}
	req := &pb.ResourceSpec{
		GPUs: []*pb.GPURequest{
			{MemoryMB: 8192, Cores: 100, Count: 2},
		},
	}
	if Fits(node, req) {
		t.Error("expected 2x8GB GPUs not to fit when max GPU mem is 4GB")
	}
}

func TestScore_CPU(t *testing.T) {
	node := &pb.NodeResources{CPUCores: 8, CPUUsage: 0}
	req := &pb.ResourceSpec{CPUCores: 4}
	s := Score(node, req)
	if s <= 0 {
		t.Error("expected positive score")
	}
}

func TestScore_GPU(t *testing.T) {
	node := &pb.NodeResources{
		CPUCores: 8,
		GPUs: []*pb.GPUDevice{
			{UUID: "GPU-aaa", MemoryAvailMB: 16384, MemoryTotalMB: 24576},
		},
	}
	req := &pb.ResourceSpec{
		CPUCores: 4,
		GPUs: []*pb.GPURequest{
			{MemoryMB: 8192, Cores: 100, Count: 1},
		},
	}
	s := Score(node, req)
	if s <= 0 {
		t.Error("expected positive score with GPU")
	}
	// GPU 评分应贡献一部分
	gpuScore := float64(16384) / float64(24576) * 0.15
	expected := gpuScore / (0.30 + 0.25 + 0.15 + 0.10) // CPU(0.30) + Mem(0.25) + GPU(0.15) + Load(0.10)
	if s < expected*0.9 || s > expected*1.1 {
		t.Logf("score=%f, expected approx %f", s, expected)
	}
}

func TestScore_NoGPU(t *testing.T) {
	node := &pb.NodeResources{CPUCores: 8, CPUUsage: 0}
	req := &pb.ResourceSpec{CPUCores: 4}
	s := Score(node, req)
	if s <= 0 {
		t.Error("expected positive score even without GPU")
	}
}

func TestRankNodes(t *testing.T) {
	nodes := map[string]*pb.NodeResources{
		"node-1": {CPUCores: 8, CPUUsage: 0.1},
		"node-2": {CPUCores: 4, CPUUsage: 0.5},
	}
	req := &pb.ResourceSpec{CPUCores: 2}
	ranked := RankNodes(nodes, req)
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked nodes, got %d", len(ranked))
	}
	if ranked[0].Score < ranked[1].Score {
		t.Error("expected ranked[0] to have higher score")
	}
}

func TestFits_NilInputs(t *testing.T) {
	if Fits(nil, nil) {
		t.Error("expected false for nil inputs")
	}
}

func TestScore_NilInputs(t *testing.T) {
	if Score(nil, nil) != 0 {
		t.Error("expected 0 score for nil inputs")
	}
}