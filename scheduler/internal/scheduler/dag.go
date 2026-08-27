package scheduler

import (
	"log"

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