package executor

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	pb "computing-power/proto/v1"
	"computing-power/agent/internal/container"
)

// Executor 处理调度器下发的命令并编排容器生命周期
type Executor struct {
	runtime  container.Runtime
	manager  *Manager
	reporter *Reporter
	hamiMgr  *container.HAMiManager
}

// NewExecutor 创建执行器
func NewExecutor(rt container.Runtime, mgr *Manager, rep *Reporter, hamiMgr *container.HAMiManager) *Executor {
	return &Executor{
		runtime:  rt,
		manager:  mgr,
		reporter: rep,
		hamiMgr:  hamiMgr,
	}
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

	// 2. 拉取镜像
	ctx := context.Background()
	if err := e.runtime.PullImage(ctx, ap.Image); err != nil {
		log.Printf("executor: pull image for unit %s: %v", ap.UnitID, err)
		e.reporter.Report(ap.UnitID, pb.UnitStatusFailed, 0, "pull image: "+err.Error(), nil)
		return
	}

	// 3. 创建容器
	spec := &container.ContainerSpec{
		ID:    ap.UnitID,
		Image: ap.Image,
		Name:  ap.UnitID,
		Env:   map[string]string{},
		Resource: &container.ResourceLimit{
			CPUCores:    0, // 不限制
			MemoryBytes: 0,
		},
	}

	// GPU 分配（如果请求了 GPU 且 HAMi 可用）
	if ap.GPURequest != nil && e.hamiMgr != nil && e.hamiMgr.Enabled() {
		gpuOK := e.setupGPUEnv(ap, spec)
		if !gpuOK {
			return
		}
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
			output := collectOutput(containerID)
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

// collectOutput 收集容器输出（占位，P4 后续可扩展）
func collectOutput(containerID string) []byte {
	// TODO(P4): 通过 containerd task.IO 或日志 API 收集 stdout/stderr
	return nil
}