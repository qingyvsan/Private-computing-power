package scheduler

import (
	"sort"

	pb "computing-power/proto/v1"
	"computing-power/pkg/resource"
)

// ScoreAndRank 对候选节点评分并降序排列，排除零分节点
func (e *Engine) ScoreAndRank(candidates []*pb.Node, spec *pb.ResourceSpec) []Candidate {
	if len(candidates) == 0 {
		return nil
	}
	result := make([]Candidate, 0, len(candidates))
	for _, node := range candidates {
		s := e.scoreNode(node, spec)
		if s > 0 {
			result = append(result, Candidate{Node: node, Score: s})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})
	return result
}

// scoreNode 计算单个节点的 4 维加权评分
func (e *Engine) scoreNode(node *pb.Node, spec *pb.ResourceSpec) float64 {
	// 1. 资源匹配度（0-1）
	resourceScore := 0.5
	if spec != nil && node.Resources != nil {
		resourceScore = resource.Score(node.Resources, spec)
	}

	// 2. 网络质量（0-1）：RTT 越低越好，0ms→1.0, 500ms+→0.0
	networkScore := 0.5
	if node.Resources != nil && node.Resources.Network != nil {
		rtt := node.Resources.Network.RTTMs
		networkScore = 1.0 - float64(rtt)/500.0
		if networkScore < 0 {
			networkScore = 0
		}
	}

	// 3. 信誉度（0-1）
	repScore := node.Reputation
	if repScore < 0 {
		repScore = 0
	} else if repScore > 1 {
		repScore = 1
	}

	// 4. 负载（0-1，越低越好）
	loadScore := 0.5
	if node.Resources != nil {
		loadScore = 1.0 - node.Resources.CPUUsage
		if loadScore < 0 {
			loadScore = 0
		}
	}

	return resourceScore*e.weights.ResourceMatch +
		networkScore*e.weights.NetworkQuality +
		repScore*e.weights.Reputation +
		loadScore*e.weights.Load
}