package phidetector

import (
	"math"
	"sync"
	"time"
)

// Detector 单个节点的 φ-accrual 故障检测器
// φ = -log10(P(当前时间 - 上次心跳时间 | 历史心跳间隔分布))
//
// φ 阈值含义：
//   φ = 1  → 约 10%   概率误判（适合非关键任务）
//   φ = 4  → 约 0.1%  概率误判（默认阈值，通用任务）
//   φ = 8  → 约 0.01% 概率误判（适合高可靠长驻服务）
type Detector struct {
	mu            sync.RWMutex
	intervals     []float64   // 滑动窗口：心跳间隔 (毫秒)
	windowSize    int         // 滑动窗口大小，默认 1000
	minSamples    int         // 最少样本数，默认 100
	threshold     float64     // φ 阈值，默认 4.0
	lastHeartbeat time.Time   // 上次心跳时间
	// 缓存的高斯分布参数
	mean     float64
	variance float64
	dirty    bool // 需要重新计算 mean/variance
}

// NewDetector 创建 φ-accrual 故障检测器
func NewDetector(windowSize, minSamples int, threshold float64) *Detector {
	if windowSize <= 0 {
		windowSize = 1000
	}
	if minSamples <= 0 {
		minSamples = 100
	}
	if threshold <= 0 {
		threshold = 4.0
	}
	return &Detector{
		intervals:  make([]float64, 0, windowSize),
		windowSize: windowSize,
		minSamples: minSamples,
		threshold:  threshold,
		dirty:      true,
	}
}

// ReportHeartbeat 记录一次心跳
func (d *Detector) ReportHeartbeat(now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.lastHeartbeat.IsZero() {
		interval := float64(now.Sub(d.lastHeartbeat).Milliseconds())
		if len(d.intervals) >= d.windowSize {
			// 移除最旧的样本
			d.intervals = d.intervals[1:]
		}
		d.intervals = append(d.intervals, interval)
		d.dirty = true
	}
	d.lastHeartbeat = now
}

// Phi 计算当前时间点的 φ 值
func (d *Detector) Phi(now time.Time) float64 {
	d.mu.RLock()
	if len(d.intervals) < d.minSamples || d.lastHeartbeat.IsZero() {
		d.mu.RUnlock()
		return 0.0 // 样本不足，返回 0 表示信任
	}

	sinceLast := float64(now.Sub(d.lastHeartbeat).Milliseconds())
	d.mu.RUnlock()

	if d.dirty {
		d.updateStats()
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.variance <= 0 {
		// 如果方差为 0（所有间隔相同），使用固定阈值
		if sinceLast > d.mean*3 {
			return d.threshold + 1
		}
		return 0
	}

	// 计算正态分布 CDF 的补数
	stddev := math.Sqrt(d.variance)
	x := (sinceLast - d.mean) / stddev
	// 使用标准正态分布的互补误差函数计算 P(x)
	p := 1.0 - 0.5*math.Erfc(-x/math.Sqrt2)

	if p <= 0 {
		return d.threshold + 1 // 概率极低，确定故障
	}
	return -math.Log10(p)
}

// IsAvailable 判断节点是否可用（φ < 阈值）
func (d *Detector) IsAvailable(now time.Time) bool {
	return d.Phi(now) < d.threshold
}

// Threshold 返回当前阈值
func (d *Detector) Threshold() float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.threshold
}

// LastHeartbeat 返回上次心跳时间
func (d *Detector) LastHeartbeat() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastHeartbeat
}

// updateStats 更新高斯分布参数（均值、方差）
func (d *Detector) updateStats() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.dirty {
		return
	}

	n := len(d.intervals)
	if n == 0 {
		return
	}

	// 计算均值
	var sum float64
	for _, v := range d.intervals {
		sum += v
	}
	mean := sum / float64(n)

	// 计算方差
	var varianceSum float64
	for _, v := range d.intervals {
		diff := v - mean
		varianceSum += diff * diff
	}
	variance := varianceSum / float64(n-1)

	d.mean = mean
	d.variance = variance
	d.dirty = false
}

// Manager 管理所有节点的 φ-accrual 检测器
type Manager struct {
	mu         sync.RWMutex
	detectors  map[string]*Detector // nodeID -> Detector
	windowSize int
	minSamples int
	threshold  float64
}

// NewManager 创建检测器管理器
func NewManager(windowSize, minSamples int, threshold float64) *Manager {
	if windowSize <= 0 {
		windowSize = 1000
	}
	if minSamples <= 0 {
		minSamples = 100
	}
	if threshold <= 0 {
		threshold = 4.0
	}
	return &Manager{
		detectors:  make(map[string]*Detector),
		windowSize: windowSize,
		minSamples: minSamples,
		threshold:  threshold,
	}
}

// GetOrCreate 获取或创建指定节点的检测器
func (m *Manager) GetOrCreate(nodeID string) *Detector {
	m.mu.RLock()
	d, ok := m.detectors[nodeID]
	m.mu.RUnlock()
	if ok {
		return d
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// 双重检查
	if d, ok := m.detectors[nodeID]; ok {
		return d
	}
	d = NewDetector(m.windowSize, m.minSamples, m.threshold)
	m.detectors[nodeID] = d
	return d
}

// ReportHeartbeat 记录节点心跳
func (m *Manager) ReportHeartbeat(nodeID string, now time.Time) {
	d := m.GetOrCreate(nodeID)
	d.ReportHeartbeat(now)
}

// Phi 获取节点当前 φ 值
func (m *Manager) Phi(nodeID string, now time.Time) float64 {
	d := m.GetOrCreate(nodeID)
	return d.Phi(now)
}

// IsAvailable 检查节点是否可用
func (m *Manager) IsAvailable(nodeID string, now time.Time) bool {
	d := m.GetOrCreate(nodeID)
	return d.IsAvailable(now)
}

// Remove 移除指定节点的检测器
func (m *Manager) Remove(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.detectors, nodeID)
}

// GetAll 返回所有节点的可用状态
func (m *Manager) GetAll(now time.Time) map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]bool, len(m.detectors))
	for id, d := range m.detectors {
		result[id] = d.IsAvailable(now)
	}
	return result
}

// Stats 返回检测器统计信息
func (m *Manager) Stats(now time.Time) map[string]struct {
	Phi           float64
	LastHeartbeat time.Time
	SampleCount   int
	Available     bool
} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]struct {
		Phi           float64
		LastHeartbeat time.Time
		SampleCount   int
		Available     bool
	}, len(m.detectors))
	for id, d := range m.detectors {
		d.mu.RLock()
		phi := d.Phi(now)
		result[id] = struct {
			Phi           float64
			LastHeartbeat time.Time
			SampleCount   int
			Available     bool
		}{
			Phi:           phi,
			LastHeartbeat: d.lastHeartbeat,
			SampleCount:   len(d.intervals),
			Available:     phi < d.threshold,
		}
		d.mu.RUnlock()
	}
	return result
}