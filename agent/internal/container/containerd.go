package container

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	containerd "github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/errdefs"
)

// containerdRuntime 基于 containerd 的容器运行时实现
type containerdRuntime struct {
	client    *containerd.Client
	socket    string
	namespace string
}

// NewRuntime 创建容器运行时
// 返回 containerd 实现；如果不可用则返回错误
func NewRuntime(socket, namespace string) (Runtime, error) {
	if socket == "" {
		socket = "/run/containerd/containerd.sock"
	}
	if namespace == "" {
		namespace = "computing-power"
	}

	client, err := containerd.New(socket, containerd.WithDefaultNamespace(namespace))
	if err != nil {
		return nil, fmt.Errorf("%w: connect to containerd: %v", ErrRuntimeNotAvailable, err)
	}

	// 验证连通性
	verCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Version(verCtx); err != nil {
		client.Close()
		return nil, fmt.Errorf("%w: containerd not reachable: %v", ErrRuntimeNotAvailable, err)
	}

	return &containerdRuntime{
		client:    client,
		socket:    socket,
		namespace: namespace,
	}, nil
}

func (r *containerdRuntime) IsAvailable() bool {
	if r.client == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := r.client.Version(ctx)
	return err == nil
}

func (r *containerdRuntime) PullImage(ctx context.Context, image string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	log.Printf("pulling image: %s", image)
	_, err := r.client.Pull(ctx, image,
		containerd.WithPullUnpack,
		containerd.WithPullLabel("app", "computing-power"),
	)
	if err != nil {
		return fmt.Errorf("pull image %s: %w", image, err)
	}
	log.Printf("image pulled: %s", image)
	return nil
}

func (r *containerdRuntime) CreateContainer(ctx context.Context, spec *ContainerSpec) (string, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	image, err := r.client.GetImage(ctx, spec.Image)
	if err != nil {
		return "", fmt.Errorf("get image %s: %w", spec.Image, err)
	}

	containerID := spec.ID
	if containerID == "" {
		containerID = "cp-" + spec.Name
	}

	opts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithEnv(envMapToSlice(spec.Env)),
	}

	// 资源限制
	if spec.Resource != nil {
		if spec.Resource.CPUCores > 0 {
			opts = append(opts, oci.WithCPUCFS(
				int64(spec.Resource.CPUCores*100000), // quota
				100000,                                // period
			))
		}
		if spec.Resource.MemoryBytes > 0 {
			opts = append(opts, oci.WithMemoryLimit(uint64(spec.Resource.MemoryBytes)))
		}
	}

	// 启动命令
	if len(spec.Command) > 0 {
		opts = append(opts, oci.WithProcessArgs(spec.Command...))
	}

	// 设备
	if len(spec.Devices) > 0 {
		for _, devPath := range spec.Devices {
			opts = append(opts, oci.WithLinuxDevice(devPath, "rwm"))
		}
	}

	// Kata 运行时
	runtimeName := "io.containerd.runc.v2"
	if spec.Runtime == "kata" {
		runtimeName = "io.containerd.kata.v2"
	}

	_, err = r.client.NewContainer(ctx, containerID,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(containerID, image),
		containerd.WithNewSpec(opts...),
		containerd.WithRuntime(runtimeName, nil),
	)
	if err != nil {
		return "", fmt.Errorf("create container %s: %w", containerID, err)
	}

	log.Printf("container created: %s (image=%s)", containerID, spec.Image)
	return containerID, nil
}

func (r *containerdRuntime) StartContainer(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("load container %s: %w", id, err)
	}

	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return fmt.Errorf("create task for %s: %w", id, err)
	}

	if err := task.Start(ctx); err != nil {
		return fmt.Errorf("start task %s: %w", id, err)
	}

	log.Printf("container started: %s", id)
	return nil
}

func (r *containerdRuntime) StopContainer(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("load container %s: %w", id, err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get task for %s: %w", id, err)
	}

	// 先 SIGTERM，等待优雅退出
	waitCh, err := task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("wait task %s: %w", id, err)
	}

	if err := task.Kill(ctx, 15, containerd.WithKillAll); err != nil { // SIGTERM
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("kill task %s: %w", id, err)
		}
	}

	// 等待退出或超时后 SIGKILL
	select {
	case <-waitCh:
	case <-time.After(10 * time.Second):
		task.Kill(ctx, 9, containerd.WithKillAll) // SIGKILL
		<-waitCh
	}

	log.Printf("container stopped: %s", id)
	return nil
}

func (r *containerdRuntime) KillContainer(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("load container %s: %w", id, err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get task for %s: %w", id, err)
	}

	if err := task.Kill(ctx, 9, containerd.WithKillAll); err != nil { // SIGKILL
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("kill task %s: %w", id, err)
		}
	}

	log.Printf("container killed: %s", id)
	return nil
}

func (r *containerdRuntime) RemoveContainer(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("load container %s: %w", id, err)
	}

	// 尝试删除 task（如果存在）
	task, err := container.Task(ctx, nil)
	if err == nil {
		exitCh, waitErr := task.Wait(ctx)
		if waitErr == nil {
			task.Kill(ctx, 9, containerd.WithKillAll)
			<-exitCh
		}
		if _, delErr := task.Delete(ctx); delErr != nil {
			log.Printf("warning: delete task %s: %v", id, delErr)
		}
	}

	if err := container.Delete(ctx); err != nil {
		return fmt.Errorf("delete container %s: %w", id, err)
	}

	log.Printf("container removed: %s", id)
	return nil
}

func (r *containerdRuntime) GetStatus(ctx context.Context, id string) (*ContainerStatus, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return &ContainerStatus{ID: id, Status: "not_found", Running: false}, nil
		}
		return nil, fmt.Errorf("load container %s: %w", id, err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return &ContainerStatus{ID: id, Status: "created", Running: false}, nil
		}
		return nil, fmt.Errorf("get task for %s: %w", id, err)
	}

	status, err := task.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("get task status %s: %w", id, err)
	}

	cs := &ContainerStatus{
		ID:      id,
		Status:  string(status.Status),
		Running: status.Status == containerd.Running,
	}

	// 获取退出码
	if status.Status == containerd.Stopped {
		cs.ExitCode = int(status.ExitStatus)
	}

	return cs, nil
}

// envMapToSlice 将 map 转换为 ["KEY=VALUE", ...] 格式
func envMapToSlice(env map[string]string) []string {
	var result []string
	for k, v := range env {
		if k != "" {
			result = append(result, k+"="+v)
		}
	}
	return result
}

// fileExists 检查文件是否存在
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// GetNVIDIADevices 枚举宿主机上所有 NVIDIA 设备节点
// 返回 /dev/nvidiactl, /dev/nvidia-uvm, /dev/nvidia{N} 的存在路径
func GetNVIDIADevices() []string {
	var devices []string

	candidates := []string{
		"/dev/nvidiactl",
		"/dev/nvidia-uvm",
		"/dev/nvidia-uvm-tools",
	}
	for _, p := range candidates {
		if fileExists(p) {
			devices = append(devices, p)
		}
	}

	// 枚举 /dev/nvidia0 ~ /dev/nvidia15
	for i := 0; i <= 15; i++ {
		p := fmt.Sprintf("/dev/nvidia%d", i)
		if fileExists(p) {
			devices = append(devices, p)
		}
	}

	return devices
}