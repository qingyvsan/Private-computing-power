package container

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	containerd "github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/errdefs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// containerLogs 保存容器的 stdout/stderr 输出
type containerLogs struct {
	stdout bytes.Buffer
	stderr bytes.Buffer
	mu     sync.Mutex
}

func (cl *containerLogs) Write(p []byte) (int, error) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.stdout.Write(p)
}

// containerdRuntime 基于 containerd 的容器运行时实现
type containerdRuntime struct {
	client    *containerd.Client
	socket    string
	namespace string
	logs      sync.Map // map[string]*containerLogs
}

// NewRuntime 创建容器运行时
// 返回 containerd 实现；如果不可用则返回错误
// 在非 Linux 系统（如 macOS）上，会尝试多个候选 socket 路径
func NewRuntime(socket, namespace string) (Runtime, error) {
	if namespace == "" {
		namespace = "computing-power"
	}

	// 构建候选 socket 路径列表
	candidates := buildSocketCandidates(socket)

	var lastErr error
	for _, s := range candidates {
		client, err := containerd.New(s, containerd.WithDefaultNamespace(namespace))
		if err != nil {
			lastErr = err
			continue
		}

		// 验证连通性
		verCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, verErr := client.Version(verCtx)
		cancel()
		if verErr != nil {
			client.Close()
			lastErr = verErr
			continue
		}

		log.Printf("container runtime connected via %s", s)
		return &containerdRuntime{
			client:    client,
			socket:    s,
			namespace: namespace,
		}, nil
	}

	return nil, fmt.Errorf("%w: connect to containerd (tried %d candidates): %v",
		ErrRuntimeNotAvailable, len(candidates), lastErr)
}

// NewTCPRuntime 通过 TCP 连接 containerd（用于 Windows 经 WSL2 socat 代理访问）
// addr 形如 "127.0.0.1:19090"。在 Windows 上，WSL2 自动将内部 TCP 端口暴露到 127.0.0.1。
func NewTCPRuntime(addr, namespace string) (Runtime, error) {
	if namespace == "" {
		namespace = "computing-power"
	}

	log.Printf("connecting to containerd via TCP at %s", addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.FailOnNonTempDialError(true),
		grpc.WithReturnConnectionError(),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: dial tcp %s: %v", ErrRuntimeNotAvailable, addr, err)
	}

	client, err := containerd.NewWithConn(conn,
		containerd.WithDefaultNamespace(namespace),
		// Windows 宿主上的客户端默认请求 windows/amd64 平台，但 WSL2 containerd 提供的是
		// Linux 容器镜像。显式固定 Linux 平台让镜像解析/拉取/解包一致。
		containerd.WithDefaultPlatform(platforms.Only(platforms.MustParse("linux/amd64"))),
	)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: new client via tcp %s: %v", ErrRuntimeNotAvailable, addr, err)
	}

	// 验证连通性
	verCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, verErr := client.Version(verCtx)
	cancel()
	if verErr != nil {
		client.Close()
		return nil, fmt.Errorf("%w: verify tcp %s: %v", ErrRuntimeNotAvailable, addr, verErr)
	}

	log.Printf("container runtime connected via TCP %s", addr)
	return &containerdRuntime{
		client:    client,
		socket:    addr,
		namespace: namespace,
	}, nil
}

// Backend 描述检测到的容器后端
type Backend struct {
	Type   string `json:"type"`   // containerd / colima / docker-desktop / orbstack / none
	Socket string `json:"socket"` // 检测到的 socket 路径
}

// DetectBackend 检测当前系统可用的容器后端
// Linux 上检查标准 containerd socket；macOS 上检查 Colima/Docker Desktop/OrbStack
func DetectBackend() Backend {
	linuxDefault := "/run/containerd/containerd.sock"

	if runtime.GOOS == "linux" {
		if fileExists(linuxDefault) {
			return Backend{Type: "containerd", Socket: linuxDefault}
		}
		return Backend{Type: "none"}
	}

	if home, err := os.UserHomeDir(); err == nil {
		candidates := []struct {
			name   string
			socket string
		}{
			{"colima", home + "/.colima/default/containerd.sock"},
			{"docker-desktop", home + "/.docker/run/containerd/containerd.sock"},
			{"orbstack", home + "/.orbstack/run/docker.sock"},
		}
		for _, c := range candidates {
			if fileExists(c.socket) {
				return Backend{Type: c.name, Socket: c.socket}
			}
		}
	}

	return Backend{Type: "none"}
}

// buildSocketCandidates 返回要尝试的 containerd socket 路径列表
// 在 macOS 上，Colima/Docker Desktop/OrbStack 的 socket 路径与 Linux 不同
func buildSocketCandidates(configured string) []string {
	var candidates []string

	// Linux 标准路径（兜底）
	linuxDefault := "/run/containerd/containerd.sock"

	// 1. 优先使用配置的路径
	if configured != "" {
		candidates = append(candidates, configured)
	} else {
		candidates = append(candidates, linuxDefault)
	}

	// 2. 非 Linux 系统添加 macOS 容器后端候选路径
	if runtime.GOOS != "linux" {
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates,
				home+"/.colima/default/containerd.sock",          // Colima
				home+"/.docker/run/containerd/containerd.sock",   // Docker Desktop
				home+"/.orbstack/run/docker.sock",                // OrbStack
			)
		}
	}

	// 去重，保持顺序
	seen := make(map[string]bool)
	var result []string
	for _, p := range candidates {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
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

	image = normalizeImageRef(image)
	log.Printf("pulling image: %s", image)
	_, err := r.client.Pull(ctx, image,
		containerd.WithPullUnpack,
		containerd.WithPullSnapshotter("overlayfs"),
		containerd.WithPullLabel("app", "computing-power"),
	)
	if err != nil {
		return fmt.Errorf("pull image %s: %w", image, err)
	}
	log.Printf("image pulled: %s", image)
	return nil
}

// normalizeImageRef 将简写镜像名规范化为完整引用，补全 docker.io/library/ 前缀。
// containerd 的 reference.Parse 需要带 registry host 的完整引用，
// 短名（如 "python:3.11-slim"）会导致解析失败。
// 同时将 docker.io 替换为国内可用镜像源（docker.m.daocloud.io）。
func normalizeImageRef(image string) string {
	if image == "" || strings.Contains(image, "://") {
		return image
	}
	// 去掉 tag 和 digest，判断是否已经包含 registry host（含 "." 或 ":" 或 单段官方镜像名）
	base := image
	if idx := strings.LastIndex(base, "@"); idx >= 0 {
		base = base[:idx]
	}
	repo := base
	if idx := strings.LastIndex(repo, ":"); idx >= 0 && !strings.Contains(repo[idx+1:], "/") {
		repo = repo[:idx] // 去掉 :tag
	}
	first := repo
	if idx := strings.Index(first, "/"); idx >= 0 {
		first = first[:idx]
	}
	// 已经是完整引用（含 . 或 : 的 host，或带 / 的命名空间）则不改
	if strings.Contains(first, ".") || strings.Contains(first, ":") || strings.Contains(repo, "/") {
		// 即使是完整引用，如果 registry 是 docker.io 也替换为镜像源
		if strings.HasPrefix(image, "docker.io/") {
			return "docker.m.daocloud.io/" + strings.TrimPrefix(image, "docker.io/")
		}
		return image
	}
	// 否则补全官方镜像前缀（使用国内镜像源）
	return "docker.m.daocloud.io/library/" + image
}

func (r *containerdRuntime) CreateContainer(ctx context.Context, spec *ContainerSpec) (string, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	imageName := normalizeImageRef(spec.Image)
	image, err := r.client.GetImage(ctx, imageName)
	if err != nil {
		return "", fmt.Errorf("get image %s: %w", imageName, err)
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

	// 生成 OCI spec：Windows 宿主上默认生成 Windows spec（s.Linux == nil），
	// 但 WSL2 的 runc 需要 Linux spec。显式指定 linux/amd64 平台生成。
	ociSpec, err := oci.GenerateSpecWithPlatform(ctx, r.client, "linux/amd64",
		&containers.Container{ID: containerID}, opts...)
	if err != nil {
		return "", fmt.Errorf("generate spec: %w", err)
	}

	_, err = r.client.NewContainer(ctx, containerID,
		containerd.WithImage(image),
		// 必须先设置 snapshotter，再创建快照。WithNewSnapshot 执行时读取
		// c.Snapshotter，若为空则回退到客户端默认值（Windows 上为 "windows"），
		// 导致 WSL2 containerd 返回 "snapshotter not loaded: windows"。
		containerd.WithSnapshotter("overlayfs"),
		containerd.WithNewSnapshot(containerID, image),
		containerd.WithSpec(ociSpec),
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

	// 使用 NullIO 避免跨平台命名管道问题：
	// Windows 编译的客户端默认用 `\\.\pipe\...` 命名管道传递 IO，但 task 实际
	// 跑在 WSL2 Linux containerd-shim 中，无法打开 Windows 命名管道。
	task, err := container.NewTask(ctx, cio.NullIO)
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

	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		return fmt.Errorf("delete container %s: %w", id, err)
	}

	// æ¸çæ¥å¿ç¼å²åº
	r.logs.Delete(id)

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

// GetContainerLogs 返回容器 stdout 日志
func (r *containerdRuntime) GetContainerLogs(ctx context.Context, id string) ([]byte, error) {
	if val, ok := r.logs.Load(id); ok {
		cl := val.(*containerLogs)
		cl.mu.Lock()
		defer cl.mu.Unlock()
		// 返回 stdout 的副本
		buf := make([]byte, cl.stdout.Len())
		copy(buf, cl.stdout.Bytes())
		return buf, nil
	}
	return nil, nil
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