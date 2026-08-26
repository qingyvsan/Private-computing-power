package registry

import (
	"sync"
	"time"

	"computing-power/pkg/phidetector"
	pb "computing-power/proto/v1"
)

// Registry 节点注册中心
// 负责：节点在线状态管理、φ-accrual 故障检测、资源状态跟踪
type Registry struct {
	mu       sync.RWMutex
	nodes    map[string]*pb.Node // nodeID -> Node
	detector *phidetector.Manager

	// 调度限制
	maxTasksPerNode int
}

// NewRegistry 创建节点注册中心
func NewRegistry(windowSize, minSamples int, phiThreshold float64) *Registry {
	return &Registry{
		nodes:           make(map[string]*pb.Node),
		detector:        phidetector.NewManager(windowSize, minSamples, phiThreshold),
		maxTasksPerNode: 10,
	}
}

// Register 注册新节点
func (r *Registry) Register(n *pb.Node) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.nodes[n.ID]; !exists {
		r.detector.GetOrCreate(n.ID)
	}
	n.LastHeartbeat = time.Now().UnixMilli()
	n.Status = pb.NodeStatusOnline
	r.nodes[n.ID] = n
}

// Unregister 注销节点
func (r *Registry) Unregister(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n, ok := r.nodes[nodeID]; ok {
		n.Status = pb.NodeStatusOffline
	}
	r.detector.Remove(nodeID)
}

// ReportHeartbeat 记录心跳并更新节点状态
func (r *Registry) ReportHeartbeat(nodeID string, res *pb.NodeResources, runningUnits []string) {
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.detector.ReportHeartbeat(nodeID, now)

	if n, ok := r.nodes[nodeID]; ok {
		n.LastHeartbeat = now.UnixMilli()
		if res != nil {
			n.Resources = res
		}
		n.CurrentTasks = int32(len(runningUnits))
		// 根据负载更新状态
		if res != nil && res.CPUUsage > 0.95 && n.CurrentTasks >= int32(r.maxTasksPerNode) {
			n.Status = pb.NodeStatusBusy
		} else {
			n.Status = pb.NodeStatusOnline
		}
	}
}

// Phi 获取节点 φ 值
func (r *Registry) Phi(nodeID string, now time.Time) float64 {
	return r.detector.Phi(nodeID, now)
}

// IsAvailable 检查节点是否可用
func (r *Registry) IsAvailable(nodeID string, now time.Time) bool {
	return r.detector.IsAvailable(nodeID, now)
}

// GetNode 获取节点信息
func (r *Registry) GetNode(nodeID string) *pb.Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nodes[nodeID]
}

// ListOnline 列出所有在线节点
func (r *Registry) ListOnline() []*pb.Node {
	now := time.Now()
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*pb.Node
	for id, n := range r.nodes {
		if r.detector.IsAvailable(id, now) && n.Status == pb.NodeStatusOnline {
			result = append(result, n)
		}
	}
	return result
}

// ListAll 列出所有节点
func (r *Registry) ListAll() []*pb.Node {
	now := time.Now()
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*pb.Node
	for id, n := range r.nodes {
		// 更新内存中的在线状态
		n.Status = pb.NodeStatusOnline
		if !r.detector.IsAvailable(id, now) {
			n.Status = pb.NodeStatusOffline
		}
		result = append(result, n)
	}
	return result
}

// DetectFailures 检测故障节点，返回 φ > 阈值的节点 ID 列表
func (r *Registry) DetectFailures(now time.Time) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var failed []string
	for id := range r.nodes {
		if !r.detector.IsAvailable(id, now) {
			failed = append(failed, id)
		}
	}
	return failed
}