package resource

import (
	"sort"

	pb "computing-power/proto/v1"
)

// Fits 检查节点资源是否满足任务需求
func Fits(node *pb.NodeResources, req *pb.ResourceSpec) bool {
	if node == nil || req == nil {
		return false
	}

	// CPU
	if node.CPUCores > 0 && req.CPUCores > 0 {
		availCPU := node.CPUCores * (1 - node.CPUUsage)
		if availCPU < req.CPUCores {
			return false
		}
	}

	// 内存
	if node.MemoryBytes > 0 && req.MemoryBytes > 0 {
		availMem := node.MemoryBytes - node.MemoryUsed
		if availMem < req.MemoryBytes {
			return false
		}
	}

	// 磁盘
	if node.DiskBytes > 0 && req.DiskBytes > 0 {
		availDisk := node.DiskBytes - node.DiskUsed
		if availDisk < req.DiskBytes {
			return false
		}
	}

	// GPU
	if len(req.GPUs) > 0 {
		totalGPUs := 0
		for _, reqGPU := range req.GPUs {
			totalGPUs += int(reqGPU.Count)
		}
		availGPUs := 0
		for _, nodeGPU := range node.GPUs {
			if nodeGPU.MemoryAvailMB >= req.GPUs[0].MemoryMB {
				availGPUs++
			}
		}
		if availGPUs < totalGPUs {
			return false
		}
	}

	// 网络
	if req.Network != nil && node.Network != nil {
		if req.Network.MaxLatencyMs > 0 && node.Network.RTTMs > float64(req.Network.MaxLatencyMs) {
			return false
		}
	}

	return true
}

// Score 计算节点与任务的匹配度（0-1），值越大越匹配
func Score(node *pb.NodeResources, req *pb.ResourceSpec) float64 {
	if !Fits(node, req) {
		return 0
	}

	score := 0.0
	weights := 0.0

	// CPU 匹配度（剩余 CPU 越多越好）
	if req.CPUCores > 0 && node.CPUCores > 0 {
		availCPU := node.CPUCores * (1 - node.CPUUsage)
		cpuScore := availCPU / (availCPU + req.CPUCores)
		score += cpuScore * 0.4
		weights += 0.4
	}

	// 内存匹配度
	if req.MemoryBytes > 0 && node.MemoryBytes > 0 {
		availMem := float64(node.MemoryBytes - node.MemoryUsed)
		reqMem := float64(req.MemoryBytes)
		memScore := availMem / (availMem + reqMem)
		score += memScore * 0.3
		weights += 0.3
	}

	// 网络匹配度（延迟越低越好）
	if req.Network != nil && node.Network != nil && req.Network.MaxLatencyMs > 0 {
		latencyRatio := 1.0 - (node.Network.RTTMs / float64(req.Network.MaxLatencyMs))
		if latencyRatio < 0 {
			latencyRatio = 0
		}
		score += latencyRatio * 0.2
		weights += 0.2
	}

	// 负载匹配度（当前负载越低越好）
	cpuUtil := node.CPUUsage
	loadScore := 1.0 - cpuUtil
	score += loadScore * 0.1
	weights += 0.1

	if weights == 0 {
		return 0.5
	}
	return score / weights
}

// RankedNodes 返回按评分排序的节点列表
type RankedNode struct {
	NodeID string
	Score  float64
}

func RankNodes(nodes map[string]*pb.NodeResources, req *pb.ResourceSpec) []RankedNode {
	var result []RankedNode
	for id, res := range nodes {
		s := Score(res, req)
		if s > 0 {
			result = append(result, RankedNode{NodeID: id, Score: s})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})
	return result
}