package scheduler

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	pb "computing-power/proto/v1"
	"computing-power/pkg/resource"
)

// assignUnit 将单个 Unit 分配到指定节点（并发安全）
func (e *Engine) assignUnit(unit *pb.Unit, node *pb.Node, job *pb.Job) bool {
	// 获取并发信号量
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-e.ctx.Done():
		return false
	}

	// 双重检查：Unit 仍为 Pending（其他调度周期可能已分配）
	current, err := e.store.GetUnit(unit.ID)
	if err != nil || current == nil {
		return false
	}
	if current.Status != pb.UnitStatusPending && current.Status != pb.UnitStatusFailed {
		return false
	}

	// 状态迁移
	unit.AssignedNode = node.ID
	unit.Status = pb.UnitStatusAssigned
	if err := e.store.SaveUnit(unit); err != nil {
		log.Printf("engine: save assigned unit %s: %v", unit.ID, err)
		return false
	}

	// 构造 push 命令（包含项目下载 URL）
	var projectDownloadURL string
	if job.ProjectID != "" {
		ownerNode := e.registry.GetNode(job.OwnerID)
		if ownerNode != nil && ownerNode.OverlayIP != "" {
			projectDownloadURL = fmt.Sprintf("http://%s:8080/api/v1/projects/%s/download", ownerNode.OverlayIP, job.ProjectID)
		}
	}
	cmd := buildAssignCommand(unit, job, projectDownloadURL)
	e.PushCommand(node.ID, cmd)

	log.Printf("engine: assigned unit %s -> node %s (job=%s, stage=%s)",
		unit.ID, node.ID, job.ID, unit.StageID)
	return true
}

// assignUnits 批量分配 Unit，每个独立执行过滤+评分+分配
func (e *Engine) assignUnits(units []*pb.Unit, job *pb.Job) {
	for _, unit := range units {
		select {
		case <-e.ctx.Done():
			return
		default:
		}

		spec := getResourceSpecForUnit(unit, job)

		// 对于需要容器运行时的作业，设置能力要求
		spec = withContainerCapability(spec, job)

		// 获取候选节点
		allNodes := e.registry.ListOnline()
		if len(allNodes) == 0 {
			log.Printf("engine: no online nodes for unit %s", unit.ID)
			continue
		}

		// 过滤
		candidates := e.Filter(allNodes, spec, job.OwnerID, job.AllowSelfAssignment)
		if len(candidates) == 0 {
			log.Printf("engine: no candidates for unit %s (job=%s)", unit.ID, job.ID)
			continue
		}

		// 评分排序
		ranked := e.ScoreAndRank(candidates, spec)
		if len(ranked) == 0 {
			log.Printf("engine: no qualified node for unit %s", unit.ID)
			continue
		}

		// 分配给最优节点
		best := ranked[0]
		e.assignUnit(unit, best.Node, job)
	}
}

// loadRetryEligibleUnits 获取可重试的 Failed Unit
func (e *Engine) loadRetryEligibleUnits() ([]*pb.Unit, error) {
	failedUnits, err := e.store.ListUnitsByStatus(pb.UnitStatusFailed)
	if err != nil {
		return nil, err
	}
	if len(failedUnits) == 0 {
		return nil, nil
	}

	now := time.Now().UnixMilli()
	var retryable []*pb.Unit
	for _, u := range failedUnits {
		if int(u.RetryCount) >= e.maxRetries {
			continue
		}
		// 检查重试延迟
		if now-u.CompletedAt < e.reassignDelay.Milliseconds() {
			continue
		}
		// 重置为 Pending 以便重新调度
		u.Status = pb.UnitStatusPending
		u.AssignedNode = ""
		u.ErrorMessage = ""
		retryable = append(retryable, u)
	}
	return retryable, nil
}

// buildAssignCommand 构造 Agent 执行命令
func buildAssignCommand(unit *pb.Unit, job *pb.Job, projectDownloadURL string) *pb.Command {
	payload := map[string]interface{}{
		"unit_id":  unit.ID,
		"stage_id": unit.StageID,
		"job_id":   unit.JobID,
		"image":    job.Image,
		"input":    unit.Input,
		"index":    unit.Index,
	}

	// 包含 GPU 请求（如果作业需要 GPU）
	spec := getResourceSpecForUnit(unit, job)
	if spec != nil && len(spec.GPUs) > 0 {
		gpuReq := spec.GPUs[0]
		payload["gpu_request"] = map[string]interface{}{
			"memory_mb": gpuReq.MemoryMB,
			"cores":     gpuReq.Cores,
			"count":     gpuReq.Count,
		}
	}

	// 包含项目信息
	if job.ProjectID != "" {
		payload["project_id"] = job.ProjectID
		payload["startup_command"] = job.StartupCommand
		payload["base_image"] = job.BaseImage
		payload["project_node_id"] = job.OwnerID
		if projectDownloadURL != "" {
			payload["project_download_url"] = projectDownloadURL
		}
	}

	payloadBytes, _ := json.Marshal(payload)
	return &pb.Command{
		Type:    "assign",
		Payload: payloadBytes,
	}
}

// getResourceSpecForUnit 获取 Unit 对应的资源规格（优先 Stage 级别，其次是 Job 级别）
func getResourceSpecForUnit(unit *pb.Unit, job *pb.Job) *pb.ResourceSpec {
	for _, stage := range job.Stages {
		if stage.ID == unit.StageID {
			return stage.Resources
		}
	}
	return job.Resources
}

// withContainerCapability 为容器作业的资源规格附加 "container" 能力要求。
// 本系统的所有作业都通过容器执行，因此调度时必须排除无容器运行时的节点。
// 返回副本，避免修改 store 中的原始 spec。
func withContainerCapability(spec *pb.ResourceSpec, job *pb.Job) *pb.ResourceSpec {
	if job.Image == "" && job.BaseImage == "" {
		return spec
	}
	cp := &pb.ResourceSpec{}
	if spec != nil {
		*cp = *spec
	}
	cp.RequiredCapabilities = append(cp.RequiredCapabilities, "container")
	return cp
}

// ========== pull 分发（AssignUnit RPC） ==========

// AssignToNode 为指定节点分配最优的 Pending Unit（pull 模式）
// 返回分配的 Unit；无合适 Unit 时返回 nil
func (e *Engine) AssignToNode(nodeID string) (*pb.Unit, error) {
	node := e.registry.GetNode(nodeID)
	if node == nil {
		return nil, nil
	}

	pendingUnits, err := e.store.ListUnitsByStatus(pb.UnitStatusPending)
	if err != nil {
		return nil, err
	}
	if len(pendingUnits) == 0 {
		return nil, nil
	}

	var bestUnit *pb.Unit
	bestScore := -1.0

	for _, unit := range pendingUnits {
		job, err := e.store.GetJob(unit.JobID)
		if err != nil || job == nil {
			continue
		}
		// 跳过未就绪的工作流 Stage
		if job.Type == pb.JobTypeWorkflow && !e.isStageReady(job, unit.StageID) {
			continue
		}
		// 跳过已结束的 Job
		if job.Status != pb.JobStatusPending && job.Status != pb.JobStatusRunning {
			continue
		}

		spec := getResourceSpecForUnit(unit, job)
		spec = withContainerCapability(spec, job)
		if !e.nodePassesFilter(node, spec, job.OwnerID, job.AllowSelfAssignment) {
			continue
		}

		score := e.scoreNode(node, spec)
		if score > bestScore {
			bestScore = score
			bestUnit = unit
		}
	}

	if bestUnit == nil {
		return nil, nil
	}

	// 分配
	bestUnit.AssignedNode = nodeID
	bestUnit.Status = pb.UnitStatusAssigned
	if err := e.store.SaveUnit(bestUnit); err != nil {
		return nil, err
	}

	log.Printf("engine: pull-assigned unit %s -> node %s", bestUnit.ID, nodeID)
	return bestUnit, nil
}

// nodePassesFilter 检查单个节点是否通过所有过滤条件
func (e *Engine) nodePassesFilter(node *pb.Node, spec *pb.ResourceSpec, ownerID string, allowSelfAssignment bool) bool {
	if node.Status != pb.NodeStatusOnline && node.Status != pb.NodeStatusBusy {
		return false
	}
	if spec != nil && node.Resources != nil && !resource.Fits(node.Resources, spec) {
		return false
	}
	// 能力检查
	if spec != nil && len(spec.RequiredCapabilities) > 0 && !hasAllCapabilities(node.Capabilities, spec.RequiredCapabilities) {
		return false
	}
	if ownerID != "" && node.ID == ownerID && !allowSelfAssignment {
		return false
	}
	if ownerID != "" && node.ID != ownerID && !e.trust.IsReachable(ownerID, node.ID, 10) {
		return false
	}

	if ownerID != "" {
		if contains(node.BlockList, ownerID) {
			return false
		}
		ownerNode := e.registry.GetNode(ownerID)
		if ownerNode != nil && contains(ownerNode.BlockList, node.ID) {
			return false
		}
	}
	return true
}

// isStageReady 检查工作流中 Stage 的依赖是否都已完成
func (e *Engine) isStageReady(job *pb.Job, stageID string) bool {
	var stage *pb.Stage
	for _, s := range job.Stages {
		if s.ID == stageID {
			stage = s
			break
		}
	}
	if stage == nil {
		return false
	}
	for _, depName := range stage.DependsOn {
		depStage := findStageByName(job.Stages, depName)
		if depStage == nil {
			return false
		}
		// 上游失败 → 当前 stage 不可能继续执行
		if depStage.Status == pb.StageStatusFailed {
			return false
		}
		if depStage.Status != pb.StageStatusCompleted {
			return false
		}
	}
	return true
}