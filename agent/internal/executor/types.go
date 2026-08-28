package executor

import pb "computing-power/proto/v1"

// AssignPayload 是调度器 "assign" 命令的 JSON 负载结构
// 与 scheduler/internal/scheduler/assigner.go buildAssignCommand() 保持一致
type AssignPayload struct {
	UnitID  string `json:"unit_id"`
	StageID string `json:"stage_id"`
	JobID   string `json:"job_id"`
	Image   string `json:"image"`
	Input   string `json:"input"`
	Index   int    `json:"index"`

	// GPURequest 可选的 GPU 请求，由调度器在分配时填充
	GPURequest *GPURequestPayload `json:"gpu_request,omitempty"`
}

// GPURequestPayload 调度器下发的 GPU 请求参数
type GPURequestPayload struct {
	MemoryMB int64 `json:"memory_mb"`
	Cores    int32 `json:"cores"`
	Count    int32 `json:"count"`
}

// UnitState 跟踪本节点上单个单元的生命周期
type UnitState struct {
	UnitID      string
	ContainerID string
	Status      pb.UnitStatus
	ExitCode    int32
	ErrorMsg    string
}