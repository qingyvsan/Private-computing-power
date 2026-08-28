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
		maxReqMem := int64(0)
		for _, reqGPU := range req.GPUs {
			totalGPUs += int(reqGPU.Count)
			if reqGPU.MemoryMB > maxReqMem {
				maxReqMem = reqGPU.MemoryMB
			}
		}
		availGPUs := 0
		gpusWithMaxMem := 0
		for _, nodeGPU := range node.GPUs {
			if nodeGPU.MemoryAvailMB >= req.GPUs[0].MemoryMB {
				availGPUs++
			}
			if nodeGPU.MemoryAvailMB >= maxReqMem {
				gpusWithMaxMem++
			}
		}
		if availGPUs < totalGPUs || gpusWithMaxMem < 1 {
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
		score += cpuScore * 0.30
		weights += 0.30
	}

	// 内存匹配度
	if req.MemoryBytes > 0 && node.MemoryBytes > 0 {
		availMem := float64(node.MemoryBytes - node.MemoryUsed)
		reqMem := float64(req.MemoryBytes)
		memScore := availMem / (availMem + reqMem)
		score += memScore * 0.25
		weights += 0.25
	}

	// GPU 匹配度（剩余 GPU 内存越多越好）
	if len(req.GPUs) > 0 && len(node.GPUs) > 0 {
		totalAvailMem := int64(0)
		totalTotalMem := int64(0)
		for _, gpu := range node.GPUs {
			totalAvailMem += gpu.MemoryAvailMB
			totalTotalMem += gpu.MemoryTotalMB
		}
		if totalTotalMem > 0 {
			gpuRatio := float64(totalAvailMem) / float64(totalTotalMem)
			score += gpuRatio * 0.15
			weights += 0.15
		}
	}

	// 网络匹配度（延迟越低越好）
	if req.Network != nil && node.Network != nil && req.Network.MaxLatencyMs > 0 {
		latencyRatio := 1.0 - (node.Network.RTTMs / float64(req.Network.MaxLatencyMs))
		if latencyRatio < 0 {
			latencyRatio = 0
		}
		score += latencyRatio * 0.20
		weights += 0.20
	}

	// 负载匹配度（当前负载越低越好）
	cpuUtil := node.CPUUsage
	loadScore := 1.0 - cpuUtil
	score += loadScore * 0.10
	weights += 0.10

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