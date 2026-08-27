package registry

import (
	"log"
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

	maxTasksPerNode  int
	heartbeatTimeout time.Duration
	phiThreshold     float64

	// 状态变更回调（用于日志/事件）
	onStatusChanged func(nodeID string, old, new pb.NodeStatus)
}

// NewRegistry 创建节点注册中心
func NewRegistry(windowSize, minSamples int, phiThreshold float64) *Registry {
	return &Registry{
		nodes:            make(map[string]*pb.Node),
		detector:         phidetector.NewManager(windowSize, minSamples, phiThreshold),
		maxTasksPerNode:  10,
		heartbeatTimeout: 30 * time.Second,
		phiThreshold:     phiThreshold,
	}
}

// SetHeartbeatTimeout 设置心跳超时（故障检测循环用）
func (r *Registry) SetHeartbeatTimeout(timeout time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.heartbeatTimeout = timeout
}

// SetOnStatusChanged 设置状态变更回调
func (r *Registry) SetOnStatusChanged(fn func(nodeID string, old, new pb.NodeStatus)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onStatusChanged = fn
}

// Start 启动定期故障检测循环
// checkInterval: 检测间隔（建议 heartbeat_interval 的 2 倍）
func (r *Registry) Start(ctx <-chan struct{}, checkInterval time.Duration) {
	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx:
				return
			case now := <-ticker.C:
				r.detectAndTransition(now)
			}
		}
	}()
	log.Printf("failure detection loop started (interval=%s, phi_threshold=%.1f, heartbeat_timeout=%s)",
		checkInterval, r.phiThreshold, r.heartbeatTimeout)
}

// detectAndTransition 执行故障检测并状态流转
func (r *Registry) detectAndTransition(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, n := range r.nodes {
		// 已离线的节点不再检测，直到重新注册
		if n.Status == pb.NodeStatusOffline {
			continue
		}

		phi := r.detector.Phi(id, now)
		sinceLastHeartbeat := now.Sub(time.UnixMilli(n.LastHeartbeat))
		timeoutExceeded := sinceLastHeartbeat > r.heartbeatTimeout

		var newStatus pb.NodeStatus

		if timeoutExceeded || phi >= r.phiThreshold*2 {
			// φ 远超阈值 或 心跳超时 → Offline
			newStatus = pb.NodeStatusOffline
		} else if phi >= r.phiThreshold {
			// φ 超过阈值但未超时*2 → Unhealthy
			newStatus = pb.NodeStatusUnhealthy
		} else {
			// φ 正常，Online 或 Busy（由 ReportHeartbeat 控制）
			newStatus = n.Status
			if newStatus != pb.NodeStatusBusy && newStatus != pb.NodeStatusOnline {
				newStatus = pb.NodeStatusOnline
			}
		}

		if newStatus != n.Status {
			oldStatus := n.Status
			n.Status = newStatus
			if r.onStatusChanged != nil {
				r.onStatusChanged(id, oldStatus, newStatus)
			}
			log.Printf("node %s status changed: %s -> %s (phi=%.2f, since_last_hb=%v)",
				id, oldStatus, newStatus, phi, sinceLastHeartbeat)
		}
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
	n.PhiValue = 0
	n.HeartbeatSampleCount = 0
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

// ReportHeartbeat 记录心跳并根据 φ 值更新节点状态
func (r *Registry) ReportHeartbeat(nodeID string, res *pb.NodeResources, runningUnits []string) {
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.detector.ReportHeartbeat(nodeID, now)

	n, ok := r.nodes[nodeID]
	if !ok {
		return
	}

	n.LastHeartbeat = now.UnixMilli()
	if res != nil {
		n.Resources = res
	}
	n.CurrentTasks = int32(len(runningUnits))

	// 计算 φ 值，决定状态
	phi := r.detector.Phi(nodeID, now)
	n.PhiValue = phi

	// 获取样本数
	stats := r.detector.Stats(now)
	if s, exists := stats[nodeID]; exists {
		n.HeartbeatSampleCount = int32(s.SampleCount)
	}

	// 状态机：根据 φ 值决定节点状态
	oldStatus := n.Status

	switch oldStatus {
	case pb.NodeStatusOffline:
		// 离线节点重新心跳 → Online
		n.Status = pb.NodeStatusOnline
		log.Printf("node %s reconnected: offline -> online (phi=%.2f)", nodeID, phi)
	case pb.NodeStatusUnhealthy:
		if phi < r.phiThreshold {
			// φ 恢复正常 → Online
			n.Status = pb.NodeStatusOnline
			log.Printf("node %s recovered: unhealthy -> online (phi=%.2f)", nodeID, phi)
		}
		// 否则保持 Unhealthy
	default:
		// Online / Busy / Unhealthy 状态统一用 φ 判断
		if phi >= r.phiThreshold {
			n.Status = pb.NodeStatusUnhealthy
			if oldStatus != pb.NodeStatusUnhealthy {
				log.Printf("node %s became unhealthy: phi=%.2f >= %.2f", nodeID, phi, r.phiThreshold)
			}
		} else {
			// 根据负载判断 Online ↔ Busy
			if res != nil && res.CPUUsage > 0.95 && n.CurrentTasks >= int32(r.maxTasksPerNode) {
				n.Status = pb.NodeStatusBusy
			} else {
				n.Status = pb.NodeStatusOnline
			}
		}
	}

	if oldStatus != n.Status && r.onStatusChanged != nil {
		r.onStatusChanged(nodeID, oldStatus, n.Status)
	}
}

// Phi 获取节点 φ 值
func (r *Registry) Phi(nodeID string, now time.Time) float64 {
	return r.detector.Phi(nodeID, now)
}

// IsAvailable 检查节点是否可用（Online 或 Busy）
func (r *Registry) IsAvailable(nodeID string, now time.Time) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n, ok := r.nodes[nodeID]
	if !ok {
		return false
	}
	return n.Status == pb.NodeStatusOnline || n.Status == pb.NodeStatusBusy
}

// GetNode 获取节点信息
func (r *Registry) GetNode(nodeID string) *pb.Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := r.nodes[nodeID]
	if n == nil {
		return nil
	}
	// 填充运行时 φ 值
	now := time.Now()
	phi := r.detector.Phi(nodeID, now)
	n.PhiValue = phi
	stats := r.detector.Stats(now)
	if s, exists := stats[nodeID]; exists {
		n.HeartbeatSampleCount = int32(s.SampleCount)
	}
	return n
}

// ListOnline 列出所有可调度节点（Online + Busy）
func (r *Registry) ListOnline() []*pb.Node {
	now := time.Now()
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*pb.Node
	for id, n := range r.nodes {
		if n.Status == pb.NodeStatusOnline || n.Status == pb.NodeStatusBusy {
			// 填充运行时 φ 值
			phi := r.detector.Phi(id, now)
			n.PhiValue = phi
			stats := r.detector.Stats(now)
			if s, exists := stats[id]; exists {
				n.HeartbeatSampleCount = int32(s.SampleCount)
			}
			result = append(result, n)
		}
	}
	return result
}

// ListAll 列出所有节点（含 φ 统计）
func (r *Registry) ListAll() []*pb.Node {
	now := time.Now()
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*pb.Node
	for id, n := range r.nodes {
		// 填充运行时 φ 值
		phi := r.detector.Phi(id, now)
		n.PhiValue = phi
		stats := r.detector.Stats(now)
		if s, exists := stats[id]; exists {
			n.HeartbeatSampleCount = int32(s.SampleCount)
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

// GetNodeStats 获取节点检测统计
func (r *Registry) GetNodeStats(nodeID string, now time.Time) *NodeStats {
	r.mu.RLock()
	n, ok := r.nodes[nodeID]
	r.mu.RUnlock()
	if !ok {
		return nil
	}
	phi := r.detector.Phi(nodeID, now)
	stats := r.detector.Stats(now)
	s := NodeStats{
		NodeID:      nodeID,
		Status:      n.Status,
		PhiValue:    phi,
		LastHeartbeat: time.UnixMilli(n.LastHeartbeat),
	}
	if st, exists := stats[nodeID]; exists {
		s.SampleCount = st.SampleCount
	}
	return &s
}

// GetAllStats 返回所有节点检测统计
func (r *Registry) GetAllStats(now time.Time) []NodeStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []NodeStats
	stats := r.detector.Stats(now)
	for id, n := range r.nodes {
		phi := r.detector.Phi(id, now)
		ns := NodeStats{
			NodeID:      id,
			Status:      n.Status,
			PhiValue:    phi,
			LastHeartbeat: time.UnixMilli(n.LastHeartbeat),
		}
		if s, exists := stats[id]; exists {
			ns.SampleCount = s.SampleCount
		}
		result = append(result, ns)
	}
	return result
}

// NodeStats 节点检测统计
type NodeStats struct {
	NodeID        string
	Status        pb.NodeStatus
	PhiValue      float64
	SampleCount   int
	LastHeartbeat time.Time
}