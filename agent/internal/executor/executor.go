package executor

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	pb "computing-power/proto/v1"
	"computing-power/agent/internal/container"
)

// GPURequest 描述 GPU 分配请求
type GPURequest struct {
	MemoryMB int64 `json:"memory_mb"`
	Cores    int32 `json:"cores"`
	Count    int32 `json:"count"`
}

// AssignPayload 解析 "assign" 命令的负载
type AssignPayload struct {
	UnitID            string      `json:"unit_id"`
	StageID           string      `json:"stage_id"`
	JobID             string      `json:"job_id"`
	Image             string      `json:"image"`
	Input             string      `json:"input"`
	Index             int         `json:"index"`
	GPURequest        *GPURequest `json:"gpu_request,omitempty"`
	ProjectID         string      `json:"project_id,omitempty"`
	StartupCommand    string      `json:"startup_command,omitempty"`
	ProjectNodeID     string      `json:"project_node_id,omitempty"`
	BaseImage         string      `json:"base_image,omitempty"`
	ProjectDownloadURL string     `json:"project_download_url,omitempty"`
}

// Executor 处理调度器下发的命令并编排容器生命周期
type Executor struct {
	runtime      container.Runtime
	manager      *Manager
	reporter     *Reporter
	hamiMgr      *container.HAMiManager
	maxCPUCores  float64
	maxMemoryMB  int64
}

// NewExecutor 创建执行器
func NewExecutor(rt container.Runtime, mgr *Manager, rep *Reporter, hamiMgr *container.HAMiManager, maxCPUCores float64, maxMemoryMB int64) *Executor {
	return &Executor{
		runtime:     rt,
		manager:     mgr,
		reporter:    rep,
		hamiMgr:     hamiMgr,
		maxCPUCores: maxCPUCores,
		maxMemoryMB: maxMemoryMB,
	}
}

// SetRuntime 替换容器运行时（例如 WSL2 代理就绪后）
// handleAssign 每次都会检查 e.runtime.IsAvailable()，替换后即可接收容器作业。
func (e *Executor) SetRuntime(rt container.Runtime) {
	e.runtime = rt
}

// HandleCommand 处理调度器命令（非阻塞）
// 由心跳 reporter 的 processResponse 调用
func (e *Executor) HandleCommand(cmd *pb.Command) {
	if cmd == nil {
		return
	}

	switch cmd.Type {
	case "assign":
		e.handleAssign(cmd.Payload)
	default:
		log.Printf("executor: unknown command type: %s", cmd.Type)
	}
}

// handleAssign 解析 "assign" 命令负载并异步启动单元执行
func (e *Executor) handleAssign(payload []byte) {
	var ap AssignPayload
	if err := json.Unmarshal(payload, &ap); err != nil {
		log.Printf("executor: failed to parse assign payload: %v", err)
		return
	}

	// 检查运行时是否可用
	if e.runtime == nil || !e.runtime.IsAvailable() {
		log.Printf("executor: runtime not available, reporting unit %s failed", ap.UnitID)
		e.reporter.Report(ap.UnitID, pb.UnitStatusFailed, 0, "container runtime not available", nil)
		return
	}

	// 异步执行，避免阻塞心跳处理
	go e.executeUnit(ap)
}

// executeUnit 执行完整的容器生命周期
func (e *Executor) executeUnit(ap AssignPayload) {
	log.Printf("executor: executing unit %s (image=%s)", ap.UnitID, ap.Image)

	// 1. 报告 Running 状态
	if err := e.reporter.Report(ap.UnitID, pb.UnitStatusRunning, 0, "", nil); err != nil {
		log.Printf("executor: report running for unit %s: %v", ap.UnitID, err)
	}

	ctx := context.Background()

	// 2. 处理项目下载（如果存在项目）
	projectDir := ""
	if ap.ProjectID != "" {
		var err error
		projectDir, err = e.downloadProject(ctx, ap)
		if err != nil {
			log.Printf("executor: download project for unit %s: %v", ap.UnitID, err)
			e.reporter.Report(ap.UnitID, pb.UnitStatusFailed, 0, "download project: "+err.Error(), nil)
			return
		}
	}

	// 3. 确定使用的镜像
	image := ap.Image
	if ap.ProjectID != "" && ap.BaseImage != "" {
		image = ap.BaseImage
	}

	// 4. 拉取镜像
	if err := e.runtime.PullImage(ctx, image); err != nil {
		log.Printf("executor: pull image for unit %s: %v", ap.UnitID, err)
		e.reporter.Report(ap.UnitID, pb.UnitStatusFailed, 0, "pull image: "+err.Error(), nil)
		return
	}

	// 5. 创建容器
	spec := &container.ContainerSpec{
		ID:    ap.UnitID,
		Image: image,
		Name:  ap.UnitID,
		Env:   map[string]string{},
		Resource: &container.ResourceLimit{
			CPUCores:    e.maxCPUCores,
			MemoryBytes: e.maxMemoryMB * 1024 * 1024,
		},
	}

	// GPU 分配（如果请求了 GPU 且 HAMi 可用）
	if ap.GPURequest != nil && e.hamiMgr != nil && e.hamiMgr.Enabled() {
		gpuOK := e.setupGPUEnv(ap, spec)
		if !gpuOK {
			return
		}
	}
	// 如果是项目作业，挂载项目目录并设置启动命令
	if projectDir != "" {
		spec.Mounts = append(spec.Mounts, projectDir+":/workspace")
		if ap.StartupCommand != "" {
			spec.Command = []string{"sh", "-c", ap.StartupCommand}
		}
		spec.Env["WORKSPACE"] = "/workspace"
	}

	containerID, err := e.runtime.CreateContainer(ctx, spec)
	if err != nil {
		log.Printf("executor: create container for unit %s: %v", ap.UnitID, err)
		e.reporter.Report(ap.UnitID, pb.UnitStatusFailed, 0, "create container: "+err.Error(), nil)
		return
	}

	// 4. 注册到管理器
	e.manager.Add(ap.UnitID, containerID)

	// 5. 启动容器
	if err := e.runtime.StartContainer(ctx, containerID); err != nil {
		log.Printf("executor: start container for unit %s: %v", ap.UnitID, err)
		e.reporter.Report(ap.UnitID, pb.UnitStatusFailed, 0, "start container: "+err.Error(), nil)
		e.manager.Remove(ap.UnitID)
		// 清理容器和快照，避免快照名占留给重试
		e.cleanupContainer(containerID)
		return
	}

	// 6. 监控容器直到退出
	e.monitorContainer(ap.UnitID, containerID)
}

// monitorContainer 监控容器，等待退出后上报最终状态
func (e *Executor) monitorContainer(unitID, containerID string) {
	ctx := context.Background()
	pollInterval := 5 * time.Second
	maxWait := 24 * time.Hour
	startTime := time.Now()

	for time.Since(startTime) < maxWait {
		time.Sleep(pollInterval)

		status, err := e.runtime.GetStatus(ctx, containerID)
		if err != nil {
			log.Printf("executor: get status for unit %s container %s: %v", unitID, containerID, err)
			continue
		}

		if !status.Running {
			// 容器已退出
			output := e.collectOutput(ctx, containerID)
			exitCode := int32(status.ExitCode)
			errMsg := status.Error

			if exitCode == 0 {
				log.Printf("executor: unit %s completed with exit code 0", unitID)
				e.reporter.Report(unitID, pb.UnitStatusCompleted, exitCode, "", output)
			} else {
				log.Printf("executor: unit %s failed with exit code %d: %s", unitID, exitCode, errMsg)
				e.reporter.Report(unitID, pb.UnitStatusFailed, exitCode, errMsg, output)
			}

			// 清理
			e.cleanupContainer(containerID)
			e.manager.Remove(unitID)
			return
		}
	}

	// 超时
	log.Printf("executor: unit %s timed out after %v", unitID, maxWait)
	e.reporter.Report(unitID, pb.UnitStatusTimeout, 0, "execution timeout", nil)
	e.cleanupContainer(containerID)
	e.manager.Remove(unitID)
}

// cleanupContainer 清理容器资源
func (e *Executor) cleanupContainer(containerID string) {
	ctx := context.Background()
	if err := e.runtime.StopContainer(ctx, containerID); err != nil {
		log.Printf("executor: stop container %s: %v", containerID, err)
	}
	if err := e.runtime.RemoveContainer(ctx, containerID); err != nil {
		log.Printf("executor: remove container %s: %v", containerID, err)
	}

	// 释放 GPU 资源
	if e.hamiMgr != nil && e.hamiMgr.Enabled() {
		e.hamiMgr.ReleaseGPUs(containerID)
		e.hamiMgr.CleanupVGPUConfig(containerID)
	}
}

// downloadProject 从上传节点下载项目文件，返回项目本地路径
func (e *Executor) downloadProject(ctx context.Context, ap AssignPayload) (string, error) {
	if ap.ProjectDownloadURL == "" {
		log.Printf("executor: no download URL for project %s", ap.ProjectID)
		return "", fmt.Errorf("no download URL")
	}

	projectCacheDir := filepath.Join(os.TempDir(), "cp-projects", ap.ProjectID)
	if _, err := os.Stat(projectCacheDir); err == nil {
		// 已缓存
		log.Printf("executor: project %s already cached at %s", ap.ProjectID, projectCacheDir)
		return projectCacheDir, nil
	}

	log.Printf("executor: downloading project %s from %s", ap.ProjectID, ap.ProjectDownloadURL)

	// 下载 zip 文件
	req, err := http.NewRequestWithContext(ctx, "GET", ap.ProjectDownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	// 解压到缓存目录
	if err := os.MkdirAll(projectCacheDir, 0755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}

	for _, f := range reader.File {
		fpath := filepath.Join(projectCacheDir, f.Name)

		// 安全检查：防止路径穿越
		if !strings.HasPrefix(filepath.Clean(fpath), filepath.Clean(projectCacheDir)+string(os.PathSeparator)) {
			log.Printf("executor: skipping unsafe path: %s", f.Name)
			continue
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return "", fmt.Errorf("create dir %s: %w", filepath.Dir(fpath), err)
		}

		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("open %s: %w", f.Name, err)
		}

		out, err := os.Create(fpath)
		if err != nil {
			rc.Close()
			return "", fmt.Errorf("create %s: %w", fpath, err)
		}

		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return "", fmt.Errorf("write %s: %w", fpath, err)
		}
	}

	log.Printf("executor: project %s extracted to %s", ap.ProjectID, projectCacheDir)
	return projectCacheDir, nil
}

// setupGPUEnv 为单元设置 GPU 环境
// 分配 GPU、生成 vgpu.json、配置环境变量和设备挂载
// 返回 true 表示成功，false 表示失败（已上报错误）
func (e *Executor) setupGPUEnv(ap AssignPayload, spec *container.ContainerSpec) bool {
	gpus, err := e.hamiMgr.AllocateGPUs(ap.UnitID, ap.GPURequest.MemoryMB, ap.GPURequest.Cores, ap.GPURequest.Count)
	if err != nil {
		log.Printf("executor: allocate gpu for unit %s: %v", ap.UnitID, err)
		e.reporter.Report(ap.UnitID, pb.UnitStatusFailed, 0, "allocate gpu: "+err.Error(), nil)
		return false
	}

	// 生成 vgpu.json
	configPath, err := e.hamiMgr.WriteVGPUConfig(ap.UnitID, gpus)
	if err != nil {
		log.Printf("executor: write vgpu config for unit %s: %v", ap.UnitID, err)
		e.hamiMgr.ReleaseGPUs(ap.UnitID)
		e.reporter.Report(ap.UnitID, pb.UnitStatusFailed, 0, "write vgpu config: "+err.Error(), nil)
		return false
	}

	// 设置 GPUConfig
	for _, g := range gpus {
		spec.GPUConfig = append(spec.GPUConfig, container.GPUAllocation{
			UUID:     g.UUID,
			MemoryMB: g.MemoryMB,
			Cores:    g.Cores,
		})
	}

	// 设置环境变量
	spec.Env["LD_PRELOAD"] = e.hamiMgr.LibPath()
	var uuids []string
	for _, g := range gpus {
		uuids = append(uuids, g.UUID)
	}
	spec.Env["NVIDIA_VISIBLE_DEVICES"] = strings.Join(uuids, ",")

	// 添加 NVIDIA 设备节点
	spec.Devices = container.GetNVIDIADevices()

	// 挂载 vgpu.json（通过环境变量传递路径，容器内部读取）
	spec.Env["VGPU_CONFIG_FILE"] = configPath

	log.Printf("executor: gpu setup complete for unit %s: %d GPUs, %dMB each",
		ap.UnitID, len(gpus), ap.GPURequest.MemoryMB)
	return true
}

// collectOutput 收集容器 stdout 日志
func (e *Executor) collectOutput(ctx context.Context, containerID string) []byte {
	if e.runtime == nil {
		return nil
	}
	output, err := e.runtime.GetContainerLogs(ctx, containerID)
	if err != nil {
		log.Printf("executor: collect output for %s: %v", containerID, err)
		return nil
	}
	return output
}