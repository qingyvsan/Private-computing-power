package v1

// ========== 节点管理 ==========

type RegisterNodeRequest struct {
	Name               string         `json:"name,omitempty" yaml:"name,omitempty"`
	PublicKey          []byte         `json:"public_key,omitempty" yaml:"public_key,omitempty"`
	HardwareFingerprint string        `json:"hardware_fingerprint,omitempty" yaml:"hardware_fingerprint,omitempty"`
	InviteCode         string         `json:"invite_code,omitempty" yaml:"invite_code,omitempty"`
	Version            string         `json:"version,omitempty" yaml:"version,omitempty"`
	InitialResources   *NodeResources `json:"initial_resources,omitempty" yaml:"initial_resources,omitempty"`
}

type RegisterNodeResponse struct {
	NodeID             string `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	OverlayIP          string `json:"overlay_ip,omitempty" yaml:"overlay_ip,omitempty"`
	NebulaCertificate  []byte `json:"nebula_certificate,omitempty" yaml:"nebula_certificate,omitempty"`
	NebulaPrivateKey   []byte `json:"nebula_private_key,omitempty" yaml:"nebula_private_key,omitempty"`
	NebulaConfig       string `json:"nebula_config,omitempty" yaml:"nebula_config,omitempty"`
	CACertificate      []byte `json:"ca_certificate,omitempty" yaml:"ca_certificate,omitempty"`
	GrpcCertificate    []byte `json:"grpc_certificate,omitempty" yaml:"grpc_certificate,omitempty"`
	GrpcPrivateKey     []byte `json:"grpc_private_key,omitempty" yaml:"grpc_private_key,omitempty"`
}

type HeartbeatRequest struct {
	NodeID      string         `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	Resources   *NodeResources `json:"resources,omitempty" yaml:"resources,omitempty"`
	RunningUnits []string      `json:"running_units,omitempty" yaml:"running_units,omitempty"`
	UnitDeltas  []*Unit        `json:"unit_deltas,omitempty" yaml:"unit_deltas,omitempty"`
}

type HeartbeatResponse struct {
	ServerTime        int64           `json:"server_time,omitempty" yaml:"server_time,omitempty"`
	NodeStatus        NodeStatus      `json:"node_status,omitempty" yaml:"node_status,omitempty"`
	PhiValue          float64         `json:"phi_value,omitempty" yaml:"phi_value,omitempty"`
	HeartbeatInterval string          `json:"heartbeat_interval,omitempty" yaml:"heartbeat_interval,omitempty"`
	Commands          []*Command      `json:"commands,omitempty" yaml:"commands,omitempty"`
}

type UnregisterNodeRequest struct {
	NodeID string `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type UnregisterNodeResponse struct {
	Success bool `json:"success,omitempty" yaml:"success,omitempty"`
}

// ========== 作业管理 ==========

type SubmitJobRequest struct {
	Job *Job `json:"job,omitempty" yaml:"job,omitempty"`
}

type SubmitJobResponse struct {
	JobID   string    `json:"job_id,omitempty" yaml:"job_id,omitempty"`
	Status  JobStatus `json:"status,omitempty" yaml:"status,omitempty"`
	Message string    `json:"message,omitempty" yaml:"message,omitempty"`
}

type CancelJobRequest struct {
	JobID  string `json:"job_id,omitempty" yaml:"job_id,omitempty"`
	NodeID string `json:"node_id,omitempty" yaml:"node_id,omitempty"`
}

type CancelJobResponse struct {
	Success bool `json:"success,omitempty" yaml:"success,omitempty"`
}

type GetJobRequest struct {
	JobID string `json:"job_id,omitempty" yaml:"job_id,omitempty"`
}

type GetJobResponse struct {
	Job *Job `json:"job,omitempty" yaml:"job,omitempty"`
}

type ListJobsRequest struct {
	NodeID       string    `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	StatusFilter JobStatus `json:"status_filter,omitempty" yaml:"status_filter,omitempty"`
	PageSize     int32     `json:"page_size,omitempty" yaml:"page_size,omitempty"`
	PageToken    string    `json:"page_token,omitempty" yaml:"page_token,omitempty"`
}

type ListJobsResponse struct {
	Jobs          []*Job  `json:"jobs,omitempty" yaml:"jobs,omitempty"`
	TotalCount    int32   `json:"total_count,omitempty" yaml:"total_count,omitempty"`
	NextPageToken string  `json:"next_page_token,omitempty" yaml:"next_page_token,omitempty"`
}

type WatchJobRequest struct {
	JobID string `json:"job_id,omitempty" yaml:"job_id,omitempty"`
}

// ========== 任务分配 ==========

type AssignUnitRequest struct {
	NodeID    string            `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	Unit      *Unit         `json:"unit,omitempty" yaml:"unit,omitempty"`
	Image     string        `json:"image,omitempty" yaml:"image,omitempty"`
	Resources *ResourceSpec `json:"resources,omitempty" yaml:"resources,omitempty"`
	Env       map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Mounts    []string      `json:"mounts,omitempty" yaml:"mounts,omitempty"`
	GPUConfig *GPUConfig    `json:"gpu_config,omitempty" yaml:"gpu_config,omitempty"`
}

type AssignUnitResponse struct {
	Accepted bool   `json:"accepted,omitempty" yaml:"accepted,omitempty"`
	Message  string `json:"message,omitempty" yaml:"message,omitempty"`
	UnitID   string `json:"unit_id,omitempty" yaml:"unit_id,omitempty"`
}

type UnitStatusReport struct {
	UnitID       string     `json:"unit_id,omitempty" yaml:"unit_id,omitempty"`
	NodeID       string     `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	Status       UnitStatus `json:"status,omitempty" yaml:"status,omitempty"`
	ExitCode     int32      `json:"exit_code,omitempty" yaml:"exit_code,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty" yaml:"error_message,omitempty"`
	Output       []byte     `json:"output,omitempty" yaml:"output,omitempty"`
}

type UnitStatusAck struct {
	Received bool `json:"received,omitempty" yaml:"received,omitempty"`
}

// ========== 信任管理 ==========

type DeclareTrustRequest struct {
	FromNodeID   string `json:"from_node_id,omitempty" yaml:"from_node_id,omitempty"`
	TargetNodeID string `json:"target_node_id,omitempty" yaml:"target_node_id,omitempty"`
	Signature    []byte `json:"signature,omitempty" yaml:"signature,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

type DeclareTrustResponse struct {
	Success bool `json:"success,omitempty" yaml:"success,omitempty"`
}

type RevokeTrustRequest struct {
	FromNodeID   string `json:"from_node_id,omitempty" yaml:"from_node_id,omitempty"`
	TargetNodeID string `json:"target_node_id,omitempty" yaml:"target_node_id,omitempty"`
	Signature    []byte `json:"signature,omitempty" yaml:"signature,omitempty"`
}

type RevokeTrustResponse struct {
	Success bool `json:"success,omitempty" yaml:"success,omitempty"`
}

type GetTrustGraphRequest struct {
	NodeID string `json:"node_id,omitempty" yaml:"node_id,omitempty"`
}

type GetTrustGraphResponse struct {
	Edges []*TrustEdge `json:"edges,omitempty" yaml:"edges,omitempty"`
}

// ========== 证书管理 ==========

type IssueCertRequest struct {
	PublicKey []byte `json:"public_key,omitempty" yaml:"public_key,omitempty"`
	NodeID    string `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	Group     string `json:"group,omitempty" yaml:"group,omitempty"`
	IPs       string `json:"ips,omitempty" yaml:"ips,omitempty"`
	Subnets   string `json:"subnets,omitempty" yaml:"subnets,omitempty"`
}

type IssueCertResponse struct {
	Certificate   []byte `json:"certificate,omitempty" yaml:"certificate,omitempty"`
	CACertificate []byte `json:"ca_certificate,omitempty" yaml:"ca_certificate,omitempty"`
	ExpiresAt     int64  `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

type RenewCertRequest struct {
	NodeID    string `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	PublicKey []byte `json:"public_key,omitempty" yaml:"public_key,omitempty"`
}

type RenewCertResponse struct {
	Certificate []byte `json:"certificate,omitempty" yaml:"certificate,omitempty"`
	ExpiresAt   int64  `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

type RevokeCertRequest struct {
	NodeID string `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type RevokeCertResponse struct {
	Success bool `json:"success,omitempty" yaml:"success,omitempty"`
}

// ========== 邀请码 ==========

type CreateInviteCodeRequest struct {
	NodeID    string `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	MaxUses   int32  `json:"max_uses,omitempty" yaml:"max_uses,omitempty"`
}

type CreateInviteCodeResponse struct {
	Code      string `json:"code,omitempty" yaml:"code,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

type RedeemInviteCodeRequest struct {
	Code               string `json:"code,omitempty" yaml:"code,omitempty"`
	NodeID             string `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	PublicKey          []byte `json:"public_key,omitempty" yaml:"public_key,omitempty"`
	HardwareFingerprint string `json:"hardware_fingerprint,omitempty" yaml:"hardware_fingerprint,omitempty"`
}

type RedeemInviteCodeResponse struct {
	Valid   bool   `json:"valid,omitempty" yaml:"valid,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

// ========== 节点查询 ==========

type ListNodesRequest struct {
	StatusFilter NodeStatus `json:"status_filter,omitempty" yaml:"status_filter,omitempty"`
	PageSize     int32      `json:"page_size,omitempty" yaml:"page_size,omitempty"`
	PageToken    string     `json:"page_token,omitempty" yaml:"page_token,omitempty"`
	RequesterID  string     `json:"requester_id,omitempty" yaml:"requester_id,omitempty"`
}

type ListNodesResponse struct {
	Nodes         []*Node `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	TotalCount    int32   `json:"total_count,omitempty" yaml:"total_count,omitempty"`
	NextPageToken string  `json:"next_page_token,omitempty" yaml:"next_page_token,omitempty"`
}

type GetNodeRequest struct {
	NodeID      string `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	RequesterID string `json:"requester_id,omitempty" yaml:"requester_id,omitempty"`
}

type GetNodeResponse struct {
	Node *Node `json:"node,omitempty" yaml:"node,omitempty"`
}

// ========== WAL 同步 ==========

type SyncWALEntry struct {
	Sequence   uint64 `json:"sequence,omitempty" yaml:"sequence,omitempty"`
	Type       uint32 `json:"type,omitempty" yaml:"type,omitempty"`
	Key        string `json:"key,omitempty" yaml:"key,omitempty"`
	Data       []byte `json:"data,omitempty" yaml:"data,omitempty"`
	Compressed bool   `json:"compressed,omitempty" yaml:"compressed,omitempty"`
}

type SyncWALRequest struct {
	LastSequence uint64 `json:"last_sequence,omitempty" yaml:"last_sequence,omitempty"`
	StandbyID    string `json:"standby_id,omitempty" yaml:"standby_id,omitempty"`
}

type SyncWALResponse struct {
	Entries    []*SyncWALEntry `json:"entries,omitempty" yaml:"entries,omitempty"`
	More       bool            `json:"more,omitempty" yaml:"more,omitempty"`
	Compressed bool            `json:"compressed,omitempty" yaml:"compressed,omitempty"`
}

type HealthCheckRequest struct {
	Timestamp int64 `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
}

type HealthCheckResponse struct {
	Timestamp int64  `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
	Role      string `json:"role,omitempty" yaml:"role,omitempty"`
	Sequence  uint64 `json:"sequence,omitempty" yaml:"sequence,omitempty"`
	LeaderID  string `json:"leader_id,omitempty" yaml:"leader_id,omitempty"`
}