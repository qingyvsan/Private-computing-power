package container

import (
	"context"
	"errors"
)

// ErrRuntimeNotAvailable 运行时不可用（未安装 containerd）
var ErrRuntimeNotAvailable = errors.New("container runtime not available")

// ContainerSpec 容器规格
type ContainerSpec struct {
	ID        string
	Image     string
	Name      string
	Command   []string
	Env       map[string]string
	Mounts    []string // "host:container" 格式的挂载列表
	Devices   []string // 主机设备路径列表（如 /dev/nvidia0）
	Runtime   string   // "" 表示默认，或 "kata" 表示 Kata Containers
	GPUConfig []GPUAllocation
	Resource  *ResourceLimit
}

// GPUAllocation GPU 分配
type GPUAllocation struct {
	UUID     string
	MemoryMB int64
	Cores    int32
}

// ResourceLimit 资源限制
type ResourceLimit struct {
	CPUCores    float64
	MemoryBytes int64
}

// ContainerStatus 容器状态
type ContainerStatus struct {
	ID        string
	Status    string
	ExitCode  int
	Running   bool
	Error     string
}

// Runtime 容器运行时接口
type Runtime interface {
	// PullImage 拉取镜像
	PullImage(ctx context.Context, image string) error
	// CreateContainer 创建容器
	CreateContainer(ctx context.Context, spec *ContainerSpec) (string, error)
	// StartContainer 启动容器
	StartContainer(ctx context.Context, id string) error
	// StopContainer 停止容器（优雅终止）
	StopContainer(ctx context.Context, id string) error
	// KillContainer 强制终止容器
	KillContainer(ctx context.Context, id string) error
	// RemoveContainer 删除容器
	RemoveContainer(ctx context.Context, id string) error
	// GetStatus 获取容器状态
	GetStatus(ctx context.Context, id string) (*ContainerStatus, error)
	// IsAvailable 运行时是否可用
	IsAvailable() bool
	// GetContainerLogs 获取容器 stdout 日志
	GetContainerLogs(ctx context.Context, id string) ([]byte, error)
}