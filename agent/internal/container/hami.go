package container

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"

	pb "computing-power/proto/v1"
)

// vGPUDevice 表示 vgpu.json 中的单个虚拟 GPU 设备
type vGPUDevice struct {
	UUID   string `json:"uuid"`
	Memory int64  `json:"memory"`
	Type   string `json:"type"`
	Health bool   `json:"health"`
}

// vGPUConfig 表示 vgpu.json 的完整结构
type vGPUConfig struct {
	Devices []vGPUDevice `json:"devices"`
	Config  struct {
		DefaultMemory int64 `json:"default_memory"`
		DefaultCores  int32 `json:"default_cores"`
	} `json:"config"`
}

// HAMiManager 管理 GPU 分配和 vgpu.json 生成
// 线程安全，支持并发分配/释放
type HAMiManager struct {
	mu sync.Mutex

	enabled      bool
	libPath      string
	configDir    string
	defaultMemMB int64
	defaultCores int32

	// 当前分配状态：physicalGPU UUID -> 已分配 MB
	allocated map[string]int64
	// 容器 GPU 分配记录：containerID -> GPU UUID -> 已分配 MB
	containerAllocations map[string]map[string]int64
	// 物理 GPU 列表缓存（测试可注入）
	physicalGPUs []*pb.GPUDevice
}

// NewHAMiManager 创建 HAMi 管理器
// 如果未启用则返回 nil，调用者可以安全地使用 nil 检查
func NewHAMiManager(enabled bool, libPath, configDir string, defaultMemMB int64, defaultCores int32) *HAMiManager {
	if !enabled {
		return nil
	}

	mgr := &HAMiManager{
		enabled:      true,
		libPath:      libPath,
		configDir:    configDir,
		defaultMemMB: defaultMemMB,
		defaultCores: defaultCores,
		allocated:            make(map[string]int64),
			containerAllocations: make(map[string]map[string]int64),
	}

	// 创建配置目录
	if err := os.MkdirAll(mgr.configDir, 0755); err != nil {
		log.Printf("hami: create config dir %s: %v", mgr.configDir, err)
	}

	return mgr
}

// AllocateGPUs 为容器分配 GPU
// 根据 memoryMB/cores/count 找到满足条件的物理 GPU
// 返回分配列表，每个元素包含 UUID、分配的 MemoryMB、Cores
func (m *HAMiManager) AllocateGPUs(containerID string, memoryMB int64, cores int32, count int32) ([]GPUAllocation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.enabled {
		return nil, fmt.Errorf("hami: not enabled")
	}

	// 刷新物理 GPU 列表（优先使用测试注入的 GPU）
	if len(m.physicalGPUs) == 0 {
		gpus, err := DiscoverGPUs()
		if err != nil {
			return nil, fmt.Errorf("hami: discover gpus: %w", err)
		}
		m.physicalGPUs = gpus
	}

	if len(m.physicalGPUs) == 0 {
		return nil, fmt.Errorf("hami: no physical GPUs available")
	}

	// 构建可用 GPU 列表（物理 - 已分配）
	type gpuCandidate struct {
		device     *pb.GPUDevice
		availMemMB int64
	}
	var candidates []gpuCandidate
	for _, gpu := range m.physicalGPUs {
		allocated := m.allocated[gpu.UUID]
		avail := gpu.MemoryAvailMB - allocated
		if avail >= memoryMB {
			candidates = append(candidates, gpuCandidate{
				device:     gpu,
				availMemMB: avail,
			})
		}
	}

	// 按可用内存降序排序（best-fit）
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].availMemMB > candidates[j].availMemMB
	})

	if len(candidates) < int(count) {
		return nil, fmt.Errorf("hami: insufficient GPUs: need %d, available %d", count, len(candidates))
	}

	// 取前 count 个
	var allocs []GPUAllocation
	for i := 0; i < int(count); i++ {
		c := candidates[i]
		uuid := c.device.UUID
		m.allocated[uuid] += memoryMB
			// 记录容器分配
			if m.containerAllocations[containerID] == nil {
				m.containerAllocations[containerID] = make(map[string]int64)
			}
			m.containerAllocations[containerID][uuid] += memoryMB
		allocs = append(allocs, GPUAllocation{
			UUID:     uuid,
			MemoryMB: memoryMB,
			Cores:    cores,
		})
	}

	log.Printf("hami: allocated %d GPUs for container %s (memory=%dMB, cores=%d)",
		count, containerID, memoryMB, cores)
	return allocs, nil
}

// WriteVGPUConfig 为指定容器生成 vgpu.json 配置文件
// 返回生成的文件路径
func (m *HAMiManager) WriteVGPUConfig(containerID string, allocs []GPUAllocation) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dir := filepath.Join(m.configDir, containerID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("hami: create vgpu dir: %w", err)
	}

	cfg := vGPUConfig{}
	cfg.Config.DefaultMemory = m.defaultMemMB
	cfg.Config.DefaultCores = m.defaultCores

	for _, alloc := range allocs {
		// 查找物理 GPU 型号
		model := "unknown"
		for _, pg := range m.physicalGPUs {
			if pg.UUID == alloc.UUID {
				model = pg.Model
				break
			}
		}
		cfg.Devices = append(cfg.Devices, vGPUDevice{
			UUID:   alloc.UUID,
			Memory: alloc.MemoryMB,
			Type:   model,
			Health: true,
		})
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("hami: marshal vgpu config: %w", err)
	}

	filePath := filepath.Join(dir, "vgpu.json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("hami: write vgpu config: %w", err)
	}

	log.Printf("hami: wrote vgpu config for container %s: %s", containerID, filePath)
	return filePath, nil
}

// ReleaseGPUs 释放指定容器的 GPU 分配
func (m *HAMiManager) ReleaseGPUs(containerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.enabled {
		return
	}

	// 查找该容器的 GPU 分配记录并释放
	if allocs, ok := m.containerAllocations[containerID]; ok {
		for uuid, mem := range allocs {
			m.allocated[uuid] -= mem
			if m.allocated[uuid] <= 0 {
				delete(m.allocated, uuid)
			}
		}
		delete(m.containerAllocations, containerID)
		log.Printf("hami: released GPU allocations for container %s (freed %d GPUs)", containerID, len(allocs))
	} else {
		log.Printf("hami: no GPU allocations found for container %s", containerID)
	}
}

// CleanupVGPUConfig 删除指定容器的 vgpu.json 配置目录
func (m *HAMiManager) CleanupVGPUConfig(containerID string) {
	dir := filepath.Join(m.configDir, containerID)
	if err := os.RemoveAll(dir); err != nil {
		log.Printf("hami: cleanup vgpu config for %s: %v", containerID, err)
	} else {
		log.Printf("hami: cleaned up vgpu config for %s", containerID)
	}
}

// ReleaseAllGPUs 释放所有 GPU 分配
// 在容器退出时调用
func (m *HAMiManager) ReleaseAllGPUs() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allocated = make(map[string]int64)
	m.physicalGPUs = nil
	log.Printf("hami: released all GPU allocations")
}

// AvailableGPUs 返回当前可用 GPU 列表（含已分配量）
// 用于心跳上报
func (m *HAMiManager) AvailableGPUs() []*pb.GPUDevice {
	m.mu.Lock()
	defer m.mu.Unlock()

	var gpus []*pb.GPUDevice
	if len(m.physicalGPUs) > 0 {
		// 使用测试注入或缓存的 GPU 列表
		for _, gpu := range m.physicalGPUs {
			gpus = append(gpus, &pb.GPUDevice{
				UUID:          gpu.UUID,
				Model:         gpu.Model,
				MemoryTotalMB: gpu.MemoryTotalMB,
				MemoryAvailMB: gpu.MemoryAvailMB,
				ComputeUtil:   gpu.ComputeUtil,
			})
		}
	} else {
		var err error
		gpus, err = DiscoverGPUs()
		if err != nil {
			return []*pb.GPUDevice{}
		}
	}

	// 更新可用内存为物理 - 已分配
	for _, gpu := range gpus {
		if allocated, ok := m.allocated[gpu.UUID]; ok {
			gpu.MemoryAvailMB -= allocated
			if gpu.MemoryAvailMB < 0 {
				gpu.MemoryAvailMB = 0
			}
		}
	}
	return gpus
}

// LibPath 返回 LD_PRELOAD 路径
func (m *HAMiManager) LibPath() string {
	return m.libPath
}

// Enabled 返回是否启用
func (m *HAMiManager) Enabled() bool {
	return m.enabled
}

// setTestGPUs 供测试使用，注入模拟 GPU 列表
func (m *HAMiManager) setTestGPUs(gpus []*pb.GPUDevice) {
	m.physicalGPUs = gpus
}