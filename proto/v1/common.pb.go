package v1

// ========== 枚举 ==========

type NodeStatus int32

const (
	NodeStatusUnspecified NodeStatus = 0
	NodeStatusOnline     NodeStatus = 1
	NodeStatusOffline    NodeStatus = 2
	NodeStatusBusy       NodeStatus = 3
	NodeStatusUnhealthy  NodeStatus = 4
	NodeStatusSuspended  NodeStatus = 5
)

func (s NodeStatus) String() string {
	switch s {
	case NodeStatusOnline:
		return "online"
	case NodeStatusOffline:
		return "offline"
	case NodeStatusBusy:
		return "busy"
	case NodeStatusUnhealthy:
		return "unhealthy"
	case NodeStatusSuspended:
		return "suspended"
	default:
		return "unknown"
	}
}

type JobType int32

const (
	JobTypeUnspecified JobType = 0
	JobTypeSingle     JobType = 1
	JobTypeAggregate  JobType = 2
	JobTypeWorkflow   JobType = 3
)

func (t JobType) String() string {
	switch t {
	case JobTypeSingle:
		return "single"
	case JobTypeAggregate:
		return "aggregate"
	case JobTypeWorkflow:
		return "workflow"
	default:
		return "unknown"
	}
}

type JobStatus int32

const (
	JobStatusUnspecified JobStatus = 0
	JobStatusPending     JobStatus = 1
	JobStatusRunning     JobStatus = 2
	JobStatusCompleted   JobStatus = 3
	JobStatusFailed      JobStatus = 4
	JobStatusCancelled   JobStatus = 5
)

func (s JobStatus) String() string {
	switch s {
	case JobStatusPending:
		return "pending"
	case JobStatusRunning:
		return "running"
	case JobStatusCompleted:
		return "completed"
	case JobStatusFailed:
		return "failed"
	case JobStatusCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

type UnitStatus int32

const (
	UnitStatusUnspecified UnitStatus = 0
	UnitStatusPending     UnitStatus = 1
	UnitStatusAssigned    UnitStatus = 2
	UnitStatusRunning     UnitStatus = 3
	UnitStatusCompleted   UnitStatus = 4
	UnitStatusFailed      UnitStatus = 5
	UnitStatusTimeout     UnitStatus = 6
	UnitStatusRetrying    UnitStatus = 7
	UnitStatusCancelled   UnitStatus = 8
)

func (s UnitStatus) String() string {
	switch s {
	case UnitStatusPending:
		return "pending"
	case UnitStatusAssigned:
		return "assigned"
	case UnitStatusRunning:
		return "running"
	case UnitStatusCompleted:
		return "completed"
	case UnitStatusFailed:
		return "failed"
	case UnitStatusTimeout:
		return "timeout"
	case UnitStatusRetrying:
		return "retrying"
	case UnitStatusCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

type SplitType int32

const (
	SplitTypeUnspecified SplitType = 0
	SplitTypeByFile      SplitType = 1
	SplitTypeByRange     SplitType = 2
	SplitTypeByN         SplitType = 3
	SplitTypeByCustom    SplitType = 4
)

type StageStatus int32

const (
	StageStatusUnspecified StageStatus = 0
	StageStatusPending     StageStatus = 1
	StageStatusRunning     StageStatus = 2
	StageStatusCompleted   StageStatus = 3
	StageStatusFailed      StageStatus = 4
	StageStatusSkipped     StageStatus = 5
)

func (s StageStatus) String() string {
	switch s {
	case StageStatusPending:
		return "pending"
	case StageStatusRunning:
		return "running"
	case StageStatusCompleted:
		return "completed"
	case StageStatusFailed:
		return "failed"
	case StageStatusSkipped:
		return "skipped"
	default:
		return "unknown"
	}
}

// ========== 资源类型 ==========

type GPUDevice struct {
	UUID            string  `json:"uuid,omitempty" yaml:"uuid,omitempty"`
	Model           string  `json:"model,omitempty" yaml:"model,omitempty"`
	MemoryTotalMB   int64   `json:"memory_total_mb,omitempty" yaml:"memory_total_mb,omitempty"`
	MemoryUsedMB    int64   `json:"memory_used_mb,omitempty" yaml:"memory_used_mb,omitempty"`
	MemoryAvailMB   int64   `json:"memory_available_mb,omitempty" yaml:"memory_available_mb,omitempty"`
	ComputeUtil     float64 `json:"compute_util,omitempty" yaml:"compute_util,omitempty"`
}

type NetworkMetrics struct {
	RTTMs         float64 `json:"rtt_ms,omitempty" yaml:"rtt_ms,omitempty"`
	BandwidthUp   float64 `json:"bandwidth_up_mbps,omitempty" yaml:"bandwidth_up_mbps,omitempty"`
	BandwidthDown float64 `json:"bandwidth_down_mbps,omitempty" yaml:"bandwidth_down_mbps,omitempty"`
	PacketLoss    float64 `json:"packet_loss,omitempty" yaml:"packet_loss,omitempty"`
	NATType       string  `json:"nat_type,omitempty" yaml:"nat_type,omitempty"`
}

type NodeResources struct {
	CPUCores    float64         `json:"cpu_cores,omitempty" yaml:"cpu_cores,omitempty"`
	CPUUsage    float64         `json:"cpu_usage,omitempty" yaml:"cpu_usage,omitempty"`
	MemoryBytes int64           `json:"memory_bytes,omitempty" yaml:"memory_bytes,omitempty"`
	MemoryUsed  int64           `json:"memory_used,omitempty" yaml:"memory_used,omitempty"`
	DiskBytes   int64           `json:"disk_bytes,omitempty" yaml:"disk_bytes,omitempty"`
	DiskUsed    int64           `json:"disk_used,omitempty" yaml:"disk_used,omitempty"`
	GPUs        []*GPUDevice    `json:"gpus,omitempty" yaml:"gpus,omitempty"`
	Network     *NetworkMetrics `json:"network,omitempty" yaml:"network,omitempty"`
}

type GPURequest struct {
	MemoryMB int64 `json:"memory_mb,omitempty" yaml:"memory_mb,omitempty"`
	Cores    int32 `json:"cores,omitempty" yaml:"cores,omitempty"`
	Count    int32 `json:"count,omitempty" yaml:"count,omitempty"`
}

type NetworkRequirement struct {
	MinBandwidthUp   int64 `json:"min_bandwidth_up_mbps,omitempty" yaml:"min_bandwidth_up_mbps,omitempty"`
	MinBandwidthDown int64 `json:"min_bandwidth_down_mbps,omitempty" yaml:"min_bandwidth_down_mbps,omitempty"`
	MaxLatencyMs     int64 `json:"max_latency_ms,omitempty" yaml:"max_latency_ms,omitempty"`
}

type ResourceSpec struct {
	CPUCores    float64              `json:"cpu_cores,omitempty" yaml:"cpu_cores,omitempty"`
	MemoryBytes int64                `json:"memory_bytes,omitempty" yaml:"memory_bytes,omitempty"`
	DiskBytes   int64                `json:"disk_bytes,omitempty" yaml:"disk_bytes,omitempty"`
	GPUs        []*GPURequest        `json:"gpus,omitempty" yaml:"gpus,omitempty"`
	Network     *NetworkRequirement  `json:"network,omitempty" yaml:"network,omitempty"`
}

// ========== 拆分策略 ==========

type ByFileSplit struct {
	InputPattern string   `json:"input_pattern,omitempty" yaml:"input_pattern,omitempty"`
	FileList     []string `json:"file_list,omitempty" yaml:"file_list,omitempty"`
}

type ByRangeSplit struct {
	Start    int64 `json:"start,omitempty" yaml:"start,omitempty"`
	End      int64 `json:"end,omitempty" yaml:"end,omitempty"`
	NumParts int32 `json:"num_parts,omitempty" yaml:"num_parts,omitempty"`
}

type ByNSplit struct {
	NumParts int32 `json:"num_parts,omitempty" yaml:"num_parts,omitempty"`
}

type ByCustomSplit struct {
	Script string   `json:"script,omitempty" yaml:"script,omitempty"`
	Args   []string `json:"args,omitempty" yaml:"args,omitempty"`
}

type SplitStrategy struct {
	Type     SplitType      `json:"type,omitempty" yaml:"type,omitempty"`
	ByFile   *ByFileSplit   `json:"by_file,omitempty" yaml:"by_file,omitempty"`
	ByRange  *ByRangeSplit  `json:"by_range,omitempty" yaml:"by_range,omitempty"`
	ByN      *ByNSplit      `json:"by_n,omitempty" yaml:"by_n,omitempty"`
	ByCustom *ByCustomSplit `json:"by_custom,omitempty" yaml:"by_custom,omitempty"`
}

// ========== 任务模型 ==========

type Unit struct {
	ID           string     `json:"id,omitempty" yaml:"id,omitempty"`
	StageID      string     `json:"stage_id,omitempty" yaml:"stage_id,omitempty"`
	JobID        string     `json:"job_id,omitempty" yaml:"job_id,omitempty"`
	Index        int32      `json:"index,omitempty" yaml:"index,omitempty"`
	Input        string     `json:"input,omitempty" yaml:"input,omitempty"`
	AssignedNode string     `json:"assigned_node,omitempty" yaml:"assigned_node,omitempty"`
	Status       UnitStatus `json:"status,omitempty" yaml:"status,omitempty"`
	RetryCount   int32      `json:"retry_count,omitempty" yaml:"retry_count,omitempty"`
	StartedAt    int64      `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	CompletedAt  int64      `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	ExitCode     int32      `json:"exit_code,omitempty" yaml:"exit_code,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty" yaml:"error_message,omitempty"`
	Output       []byte     `json:"output,omitempty" yaml:"output,omitempty"`
}

type Stage struct {
	ID             string        `json:"id,omitempty" yaml:"id,omitempty"`
	JobID          string        `json:"job_id,omitempty" yaml:"job_id,omitempty"`
	Name           string        `json:"name,omitempty" yaml:"name,omitempty"`
	DependsOn      []string      `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Split          *SplitStrategy `json:"split,omitempty" yaml:"split,omitempty"`
	Resources      *ResourceSpec  `json:"resources,omitempty" yaml:"resources,omitempty"`
	Inputs         []string      `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs        []string      `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	MaxConcurrency int32         `json:"max_concurrency,omitempty" yaml:"max_concurrency,omitempty"`
	Status         StageStatus   `json:"status,omitempty" yaml:"status,omitempty"`
	Units          []*Unit       `json:"units,omitempty" yaml:"units,omitempty"`
}

type Job struct {
	ID                 string        `json:"id,omitempty" yaml:"id,omitempty"`
	Name               string        `json:"name,omitempty" yaml:"name,omitempty"`
	Type               JobType       `json:"type,omitempty" yaml:"type,omitempty"`
	OwnerID            string        `json:"owner_id,omitempty" yaml:"owner_id,omitempty"`
	Image              string        `json:"image,omitempty" yaml:"image,omitempty"`
	Resources          *ResourceSpec `json:"resources,omitempty" yaml:"resources,omitempty"`
	Stages             []*Stage      `json:"stages,omitempty" yaml:"stages,omitempty"`
	Status             JobStatus     `json:"status,omitempty" yaml:"status,omitempty"`
	FailurePolicy      string        `json:"failure_policy,omitempty" yaml:"failure_policy,omitempty"`
	MaxRetries         int32         `json:"max_retries,omitempty" yaml:"max_retries,omitempty"`
	MaxDurationMs      int64         `json:"max_duration_ms,omitempty" yaml:"max_duration_ms,omitempty"`
	CreatedAt          int64         `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UpdatedAt          int64         `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	CompletedAt        int64         `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	AllowSelfAssignment bool         `json:"allow_self_assignment,omitempty" yaml:"allow_self_assignment,omitempty"`
	ProjectID          string        `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	StartupCommand     string        `json:"startup_command,omitempty" yaml:"startup_command,omitempty"`
	BaseImage          string        `json:"base_image,omitempty" yaml:"base_image,omitempty"`
}

// ========== 信任图 ==========

type TrustEdge struct {
	FromNode  string `json:"from_node,omitempty" yaml:"from_node,omitempty"`
	ToNode    string `json:"to_node,omitempty" yaml:"to_node,omitempty"`
	Signature []byte `json:"signature,omitempty" yaml:"signature,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

// ========== 节点信息 ==========

type Node struct {
	ID                  string         `json:"id,omitempty" yaml:"id,omitempty"`
	Name                string         `json:"name,omitempty" yaml:"name,omitempty"`
	OverlayIP           string         `json:"overlay_ip,omitempty" yaml:"overlay_ip,omitempty"`
	PublicKey           []byte         `json:"public_key,omitempty" yaml:"public_key,omitempty"`
	Status              NodeStatus     `json:"status,omitempty" yaml:"status,omitempty"`
	Resources           *NodeResources `json:"resources,omitempty" yaml:"resources,omitempty"`
	Version             string         `json:"version,omitempty" yaml:"version,omitempty"`
	HardwareFingerprint string         `json:"hardware_fingerprint,omitempty" yaml:"hardware_fingerprint,omitempty"`
	NATType             string         `json:"nat_type,omitempty" yaml:"nat_type,omitempty"`
	TrustList           []string       `json:"trust_list,omitempty" yaml:"trust_list,omitempty"`
	BlockList           []string       `json:"block_list,omitempty" yaml:"block_list,omitempty"`
	Discoverable        string         `json:"discoverable,omitempty" yaml:"discoverable,omitempty"`
	RegisteredAt        int64          `json:"registered_at,omitempty" yaml:"registered_at,omitempty"`
	LastHeartbeat       int64          `json:"last_heartbeat,omitempty" yaml:"last_heartbeat,omitempty"`
	CurrentTasks        int32          `json:"current_tasks,omitempty" yaml:"current_tasks,omitempty"`
	MaxTasks            int32          `json:"max_tasks,omitempty" yaml:"max_tasks,omitempty"`
	Reputation          float64        `json:"reputation,omitempty" yaml:"reputation,omitempty"`
	PhiValue            float64        `json:"phi_value,omitempty" yaml:"phi_value,omitempty"`
	HeartbeatSampleCount int32         `json:"heartbeat_sample_count,omitempty" yaml:"heartbeat_sample_count,omitempty"`
}

// ========== 调度命令 ==========

type GPUDeviceAllocation struct {
	UUID      string `json:"uuid,omitempty" yaml:"uuid,omitempty"`
	MemoryMB  int64  `json:"memory_mb,omitempty" yaml:"memory_mb,omitempty"`
	Cores     int32  `json:"cores,omitempty" yaml:"cores,omitempty"`
}

type GPUConfig struct {
	Devices []*GPUDeviceAllocation `json:"devices,omitempty" yaml:"devices,omitempty"`
}

type Command struct {
	Type    string `json:"type,omitempty" yaml:"type,omitempty"`
	Payload []byte `json:"payload,omitempty" yaml:"payload,omitempty"`
}

type JobEvent struct {
	JobID     string    `json:"job_id,omitempty" yaml:"job_id,omitempty"`
	Status    JobStatus `json:"status,omitempty" yaml:"status,omitempty"`
	Message   string    `json:"message,omitempty" yaml:"message,omitempty"`
	Timestamp int64     `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
}