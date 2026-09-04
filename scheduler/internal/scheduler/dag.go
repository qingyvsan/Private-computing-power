package scheduler

import (
	"log"
	"time"

	pb "computing-power/proto/v1"
	"computing-power/pkg/taskmodel"
)

// scheduleJob 按 Job 类型分发调度
func (e *Engine) scheduleJob(job *pb.Job, units []*pb.Unit) {
	switch job.Type {
	case pb.JobTypeWorkflow:
		e.scheduleWorkflow(job, units)
	default:
		// Single 和 Aggregate：所有 Pending Unit 立即可分配
		e.assignUnits(units, job)
	}
}

// scheduleWorkflow 工作流调度：仅调度依赖已满足的 Stage 的 Unit
func (e *Engine) scheduleWorkflow(job *pb.Job, units []*pb.Unit) {
	// 拓扑排序
	sortedStages, err := taskmodel.TopologicalSort(convertStages(job.Stages))
	if err != nil {
		log.Printf("engine: workflow %s topological sort failed: %v", job.ID, err)
		return
	}

	// 按 StageID 分组 Pending Unit
	unitsByStage := make(map[string][]*pb.Unit)
	for _, u := range units {
		unitsByStage[u.StageID] = append(unitsByStage[u.StageID], u)
	}

	// 检查每个 Stage 的依赖是否就绪
	var readyUnits []*pb.Unit
	for _, ts := range sortedStages {
		protoStage := findStageByName(job.Stages, ts.Name)
		if protoStage == nil {
			continue
		}
		// 跳过已结束的 Stage
		if protoStage.Status == pb.StageStatusCompleted ||
			protoStage.Status == pb.StageStatusFailed ||
			protoStage.Status == pb.StageStatusSkipped {
			continue
		}
		// 检查依赖是否失败 → 级联标记为 Skipped 并取消其 Unit
		depFailed := false
		for _, depName := range protoStage.DependsOn {
			depStage := findStageByName(job.Stages, depName)
			if depStage != nil && depStage.Status == pb.StageStatusFailed {
				depFailed = true
				break
			}
		}
		if depFailed {
			e.skipStageAndUnits(job, protoStage)
			continue
		}
		// 检查依赖
		depsMet := true
		for _, depName := range protoStage.DependsOn {
			depStage := findStageByName(job.Stages, depName)
			if depStage == nil || depStage.Status != pb.StageStatusCompleted {
				depsMet = false
				break
			}
		}
		if depsMet {
			if stageUnits, ok := unitsByStage[protoStage.ID]; ok {
				readyUnits = append(readyUnits, stageUnits...)
			}
		}
	}

	if len(readyUnits) > 0 {
		log.Printf("engine: workflow %s: scheduling %d units from ready stages",
			job.ID, len(readyUnits))
		e.assignUnits(readyUnits, job)
	}
}

// skipStageAndUnits 将 Stage 标记为 Skipped 并取消其所有非终结 Unit
// 用于工作流中上游 Stage 失败时的级联处理。
func (e *Engine) skipStageAndUnits(job *pb.Job, stage *pb.Stage) {
	if stage.Status == pb.StageStatusCompleted ||
		stage.Status == pb.StageStatusFailed ||
		stage.Status == pb.StageStatusSkipped {
		return
	}
	stage.Status = pb.StageStatusSkipped
	if err := e.store.UpdateStageStatus(stage.ID, pb.StageStatusSkipped); err != nil {
		log.Printf("engine: skip stage %s: %v", stage.ID, err)
	}

	// 取消该 Stage 的所有非终结 Unit
	units, err := e.store.ListUnitsByStage(stage.ID)
	if err != nil {
		log.Printf("engine: skip stage %s list units: %v", stage.ID, err)
		return
	}
	for _, u := range units {
		if u.Status == pb.UnitStatusPending || u.Status == pb.UnitStatusAssigned || u.Status == pb.UnitStatusRunning {
			if _, err := e.store.UpdateUnitStatus(u.ID, pb.UnitStatusCancelled, 0, "upstream stage failed"); err != nil {
				log.Printf("engine: skip stage %s cancel unit %s: %v", stage.ID, u.ID, err)
			}
		}
	}
	now := time.Now()
	log.Printf("engine: stage %s (job %s) skipped due to upstream failure at %s",
		stage.ID, job.ID, now.Format("15:04:05"))
}

// findStageByName 按名称查找 Stage
func findStageByName(stages []*pb.Stage, name string) *pb.Stage {
	for _, s := range stages {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// convertStages 将 proto Stage 转为 taskmodel Stage（供 TopologicalSort 使用）
func convertStages(stages []*pb.Stage) []*taskmodel.Stage {
	result := make([]*taskmodel.Stage, len(stages))
	for i, s := range stages {
		result[i] = &taskmodel.Stage{
			Name:      s.Name,
			DependsOn: s.DependsOn,
		}
	}
	return result
}