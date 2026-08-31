package scheduler

import (
	pb "computing-power/proto/v1"
	"computing-power/pkg/resource"
)

// Candidate 已通过过滤、待评分的节点
type Candidate struct {
	Node  *pb.Node
	Score float64
}

// Filter 执行完整过滤管线：在线 → 资源 → 信任 → 黑名单
func (e *Engine) Filter(nodes []*pb.Node, spec *pb.ResourceSpec, ownerID string, allowSelfAssignment bool) []*pb.Node {
	nodes = e.filterOnline(nodes)
	nodes = e.filterResources(nodes, spec)
	nodes = e.filterTrust(nodes, ownerID, allowSelfAssignment)
	nodes = e.filterBlocklist(nodes, ownerID)
	return nodes
}

// filterOnline 保留状态为 Online 或 Busy 的节点
func (e *Engine) filterOnline(nodes []*pb.Node) []*pb.Node {
	if len(nodes) == 0 {
		return nil
	}
	result := make([]*pb.Node, 0, len(nodes))
	for _, n := range nodes {
		if n.Status == pb.NodeStatusOnline || n.Status == pb.NodeStatusBusy {
			result = append(result, n)
		}
	}
	return result
}

// filterResources 保留资源满足硬约束的节点
func (e *Engine) filterResources(nodes []*pb.Node, spec *pb.ResourceSpec) []*pb.Node {
	if spec == nil || len(nodes) == 0 {
		return nodes
	}
	result := make([]*pb.Node, 0, len(nodes))
	for _, n := range nodes {
		if n.Resources != nil && resource.Fits(n.Resources, spec) {
			result = append(result, n)
		}
	}
	return result
}

// filterTrust 保留 owner 信任可达的节点（owner 自身节点仅在 allowSelfAssignment=true 时通过）
func (e *Engine) filterTrust(nodes []*pb.Node, ownerID string, allowSelfAssignment bool) []*pb.Node {
	if ownerID == "" || len(nodes) == 0 {
		return nodes
	}
	result := make([]*pb.Node, 0, len(nodes))
	for _, n := range nodes {
		// 默认不把自己的节点分配给自己；allowSelfAssignment=true 时允许
		if n.ID == ownerID && !allowSelfAssignment {
			continue
		}
		if n.ID == ownerID || e.trust.IsReachable(ownerID, n.ID, 10) {
			result = append(result, n)
		}
	}
	return result
}

// filterBlocklist 排除双方 BlockList 中的节点
func (e *Engine) filterBlocklist(nodes []*pb.Node, ownerID string) []*pb.Node {
	if ownerID == "" || len(nodes) == 0 || e.registry == nil {
		return nodes
	}
	// 获取 owner 节点的 BlockList
	ownerNode := e.registry.GetNode(ownerID)
	if ownerNode == nil {
		return nodes
	}
	result := make([]*pb.Node, 0, len(nodes))
	for _, n := range nodes {
		if contains(n.BlockList, ownerID) {
			continue
		}
		if contains(ownerNode.BlockList, n.ID) {
			continue
		}
		result = append(result, n)
	}
	return result
}

// contains 检查字符串切片中是否包含目标
func contains(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}