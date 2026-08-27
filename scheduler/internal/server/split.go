package server

import (
	"fmt"

	pb "computing-power/proto/v1"
)

// executeSplit 执行拆分策略，返回拆分后的 Unit 列表
func executeSplit(stage *pb.Stage) ([]*pb.Unit, error) {
	if stage.Split == nil {
		// 没有拆分策略，创建一个默认 Unit
		unit := newUnit(stage.ID, stage.JobID, 0, "")
		return []*pb.Unit{unit}, nil
	}

	switch stage.Split.Type {
	case pb.SplitTypeByFile:
		if stage.Split.ByFile == nil {
			return nil, fmt.Errorf("by_file split config is nil")
		}
		return splitByFile(stage, stage.Split.ByFile)
	case pb.SplitTypeByRange:
		if stage.Split.ByRange == nil {
			return nil, fmt.Errorf("by_range split config is nil")
		}
		return splitByRange(stage, stage.Split.ByRange)
	case pb.SplitTypeByN:
		if stage.Split.ByN == nil {
			return nil, fmt.Errorf("by_n split config is nil")
		}
		return splitByN(stage, stage.Split.ByN)
	case pb.SplitTypeByCustom:
		if stage.Split.ByCustom == nil {
			return nil, fmt.Errorf("by_custom split config is nil")
		}
		return splitByCustom(stage, stage.Split.ByCustom)
	default:
		return nil, fmt.Errorf("unknown split type: %v", stage.Split.Type)
	}
}

// splitByFile 每个文件创建一个 Unit
func splitByFile(stage *pb.Stage, cfg *pb.ByFileSplit) ([]*pb.Unit, error) {
	var files []string
	if len(cfg.FileList) > 0 {
		files = cfg.FileList
	} else if cfg.InputPattern != "" {
		files = []string{cfg.InputPattern}
	} else {
		return nil, fmt.Errorf("no file list or input pattern provided for by_file split")
	}

	units := make([]*pb.Unit, len(files))
	for i, file := range files {
		units[i] = newUnit(stage.ID, stage.JobID, int32(i), file)
	}
	return units, nil
}

// splitByRange 将范围 [start, end] 等分为 N 段，每段创建一个 Unit
func splitByRange(stage *pb.Stage, cfg *pb.ByRangeSplit) ([]*pb.Unit, error) {
	total := cfg.End - cfg.Start
	partSize := total / int64(cfg.NumParts)
	remainder := total % int64(cfg.NumParts)

	units := make([]*pb.Unit, cfg.NumParts)
	start := cfg.Start
	for i := int32(0); i < cfg.NumParts; i++ {
		end := start + partSize - 1
		if int64(i) < remainder {
			end++
		}
		if i == cfg.NumParts-1 {
			end = cfg.End
		}
		units[i] = newUnit(stage.ID, stage.JobID, i, fmt.Sprintf("%d-%d", start, end))
		start = end + 1
	}
	return units, nil
}

// splitByN 创建 N 个相同 Unit
func splitByN(stage *pb.Stage, cfg *pb.ByNSplit) ([]*pb.Unit, error) {
	units := make([]*pb.Unit, cfg.NumParts)
	for i := int32(0); i < cfg.NumParts; i++ {
		units[i] = newUnit(stage.ID, stage.JobID, i, fmt.Sprintf("part-%d", i))
	}
	return units, nil
}

// splitByCustom 每个自定义输入创建一个 Unit
func splitByCustom(stage *pb.Stage, cfg *pb.ByCustomSplit) ([]*pb.Unit, error) {
	if len(cfg.Args) == 0 {
		// 无参数时创建一个空 Unit
		unit := newUnit(stage.ID, stage.JobID, 0, "")
		return []*pb.Unit{unit}, nil
	}
	units := make([]*pb.Unit, len(cfg.Args))
	for i, arg := range cfg.Args {
		units[i] = newUnit(stage.ID, stage.JobID, int32(i), arg)
	}
	return units, nil
}

// newUnit 创建 proto Unit（包内可见，用于拆分逻辑）
func newUnit(stageID, jobID string, index int32, input string) *pb.Unit {
	return &pb.Unit{
		ID:     fmt.Sprintf("unit-%s-%d", stageID, index),
		StageID: stageID,
		JobID:  jobID,
		Index:  index,
		Input:  input,
		Status: pb.UnitStatusPending,
	}
}