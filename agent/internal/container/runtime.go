package container

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	Mounts    []string
	Runtime   string // "" 表示默认，或 "kata" 表示 Kata Containers
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
}

// NewRuntime 创建容器运行时
// 返回 containerd 实现；如果不可用则返回错误
func NewRuntime(socket, namespace string) (Runtime, error) {
	r := &containerdRuntime{
		socket:    socket,
		namespace: namespace,
	}
	if !r.IsAvailable() {
		return nil, fmt.Errorf("%w: socket %s not found", ErrRuntimeNotAvailable, socket)
	}
	return r, nil
}

// containerdRuntime 基于 containerd 的容器运行时实现
type containerdRuntime struct {
	socket    string
	namespace string
}

func (r *containerdRuntime) IsAvailable() bool {
	// P4 阶段接入 containerd 客户端
	// 当前仅做 socket 存在性检查
	if r.socket == "" {
		return false
	}
	return fileExists(r.socket)
}

func (r *containerdRuntime) PullImage(ctx context.Context, image string) error {
	// TODO(P4): 通过 containerd + Dragonfly dfdaemon 拉取镜像
	return fmt.Errorf("containerd integration not implemented yet")
}

func (r *containerdRuntime) CreateContainer(ctx context.Context, spec *ContainerSpec) (string, error) {
	return "", fmt.Errorf("containerd integration not implemented yet")
}

func (r *containerdRuntime) StartContainer(ctx context.Context, id string) error {
	return fmt.Errorf("containerd integration not implemented yet")
}

func (r *containerdRuntime) StopContainer(ctx context.Context, id string) error {
	return fmt.Errorf("containerd integration not implemented yet")
}

func (r *containerdRuntime) KillContainer(ctx context.Context, id string) error {
	return fmt.Errorf("containerd integration not implemented yet")
}

func (r *containerdRuntime) RemoveContainer(ctx context.Context, id string) error {
	return fmt.Errorf("containerd integration not implemented yet")
}

func (r *containerdRuntime) GetStatus(ctx context.Context, id string) (*ContainerStatus, error) {
	return nil, fmt.Errorf("containerd integration not implemented yet")
}

// fileExists 检查文件是否存在
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}