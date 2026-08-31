package executor

import pb "computing-power/proto/v1"

// UnitState 跟踪本节点上单个单元的生命周期
type UnitState struct {
	UnitID      string
	ContainerID string
	Status      pb.UnitStatus
	ExitCode    int32
	ErrorMsg    string
}