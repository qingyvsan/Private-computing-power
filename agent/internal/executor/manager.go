package executor

import (
	"sync"
)

// Manager 是线程安全的单元注册表
// 跟踪本节点上所有运行中的单元，取代 core.Agent.tasks
type Manager struct {
	mu    sync.RWMutex
	units map[string]*UnitState
}

// NewManager 创建单元管理器
func NewManager() *Manager {
	return &Manager{
		units: make(map[string]*UnitState),
	}
}

// Add 添加单元到注册表
func (m *Manager) Add(unitID, containerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.units[unitID] = &UnitState{
		UnitID:      unitID,
		ContainerID: containerID,
	}
}

// Remove 从注册表移除单元
func (m *Manager) Remove(unitID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.units, unitID)
}

// Get 获取单元状态；返回 nil 表示不存在
func (m *Manager) Get(unitID string) *UnitState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.units[unitID]
}

// List 返回所有单元 ID（用于心跳 runningUnits 回调）
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.units))
	for id := range m.units {
		ids = append(ids, id)
	}
	return ids
}

// Len 返回单元数量
func (m *Manager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.units)
}