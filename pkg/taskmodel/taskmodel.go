package taskmodel

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

// ========== 枚举 ==========

type JobType int

const (
	JobTypeSingle    JobType = 1
	JobTypeAggregate JobType = 2
	JobTypeWorkflow  JobType = 3
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

type JobStatus int

const (
	JobStatusPending   JobStatus = 0
	JobStatusRunning   JobStatus = 1
	JobStatusCompleted JobStatus = 2
	JobStatusFailed    JobStatus = 3
	JobStatusCancelled JobStatus = 4
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

type StageStatus int

const (
	StageStatusPending   StageStatus = 0
	StageStatusRunning   StageStatus = 1
	StageStatusCompleted StageStatus = 2
	StageStatusFailed    StageStatus = 3
	StageStatusSkipped   StageStatus = 4
)

type UnitStatus int

const (
	UnitStatusPending   UnitStatus = 0
	UnitStatusAssigned  UnitStatus = 1
	UnitStatusRunning   UnitStatus = 2
	UnitStatusCompleted UnitStatus = 3
	UnitStatusFailed    UnitStatus = 4
	UnitStatusTimeout   UnitStatus = 5
	UnitStatusRetrying  UnitStatus = 6
	UnitStatusCancelled UnitStatus = 7
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

type SplitType int

const (
	SplitTypeByFile   SplitType = 1
	SplitTypeByRange  SplitType = 2
	SplitTypeByN      SplitType = 3
	SplitTypeByCustom SplitType = 4
)

func (t SplitType) String() string {
	switch t {
	case SplitTypeByFile:
		return "by_file"
	case SplitTypeByRange:
		return "by_range"
	case SplitTypeByN:
		return "by_n"
	case SplitTypeByCustom:
		return "by_custom"
	default:
		return "unknown"
	}
}

// ========== 资源规格 ==========

type ResourceSpec struct {
	CPUCores    float64              `json:"cpu_cores" yaml:"cpu_cores"`
	MemoryBytes int64                `json:"memory_bytes" yaml:"memory_bytes"`
	DiskBytes   int64                `json:"disk_bytes" yaml:"disk_bytes"`
	GPUs        []GPURequest         `json:"gpus,omitempty" yaml:"gpus,omitempty"`
	Network     *NetworkRequirements `json:"network,omitempty" yaml:"network,omitempty"`
}

type GPURequest struct {
	MemoryMB int64 `json:"memory_mb" yaml:"memory_mb"`
	Cores    int32 `json:"cores" yaml:"cores"`
	Count    int32 `json:"count" yaml:"count"`
}

type NetworkRequirements struct {
	MinBandwidthUp   int64 `json:"min_bandwidth_up" yaml:"min_bandwidth_up"`
	MinBandwidthDown int64 `json:"min_bandwidth_down" yaml:"min_bandwidth_down"`
	MaxLatencyMs     int64 `json:"max_latency_ms" yaml:"max_latency_ms"`
}

// ========== 拆分策略 ==========

type SplitStrategy struct {
	Type     SplitType      `json:"type" yaml:"type"`
	ByFile   *ByFileSplit   `json:"by_file,omitempty" yaml:"by_file,omitempty"`
	ByRange  *ByRangeSplit  `json:"by_range,omitempty" yaml:"by_range,omitempty"`
	ByN      *ByNSplit      `json:"by_n,omitempty" yaml:"by_n,omitempty"`
	ByCustom *ByCustomSplit `json:"by_custom,omitempty" yaml:"by_custom,omitempty"`
}

type ByFileSplit struct {
	InputPattern string   `json:"input_pattern" yaml:"input_pattern"`
	FileList     []string `json:"file_list,omitempty" yaml:"file_list,omitempty"`
}

type ByRangeSplit struct {
	Start    int64 `json:"start" yaml:"start"`
	End      int64 `json:"end" yaml:"end"`
	NumParts int32 `json:"num_parts" yaml:"num_parts"`
}

type ByNSplit struct {
	NumParts int32 `json:"num_parts" yaml:"num_parts"`
}

type ByCustomSplit struct {
	Script string   `json:"script" yaml:"script"`
	Args   []string `json:"args,omitempty" yaml:"args,omitempty"`
}

// ========== 任务模型 ==========

// Unit 是调度的最小可执行单元
type Unit struct {
	ID           string     `json:"id" yaml:"id"`
	StageID      string     `json:"stage_id" yaml:"stage_id"`
	JobID        string     `json:"job_id" yaml:"job_id"`
	Index        int        `json:"index" yaml:"index"`
	Input        string     `json:"input" yaml:"input"`
	AssignedNode string     `json:"assigned_node" yaml:"assigned_node"`
	Status       UnitStatus `json:"status" yaml:"status"`
	RetryCount   int        `json:"retry_count" yaml:"retry_count"`
	StartedAt    *time.Time `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	ExitCode     int        `json:"exit_code" yaml:"exit_code"`
	ErrorMessage string     `json:"error_message,omitempty" yaml:"error_message,omitempty"`
	Output       []byte     `json:"output,omitempty" yaml:"output,omitempty"`
}

// Stage 是作业的执行阶段
type Stage struct {
	ID             string        `json:"id" yaml:"id"`
	JobID          string        `json:"job_id" yaml:"job_id"`
	Name           string        `json:"name" yaml:"name"`
	DependsOn      []string      `json:"depends_on" yaml:"depends_on"`
	Split          *SplitStrategy `json:"split,omitempty" yaml:"split,omitempty"`
	Resources      *ResourceSpec  `json:"resources" yaml:"resources"`
	Inputs         []string      `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs        []string      `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	MaxConcurrency int           `json:"max_concurrency" yaml:"max_concurrency"`
	Status         StageStatus   `json:"status" yaml:"status"`
	Units          []*Unit       `json:"units,omitempty" yaml:"units,omitempty"`
}

// Job 是用户提交的完整作业
type Job struct {
	ID            string        `json:"id" yaml:"id"`
	Name          string        `json:"name" yaml:"name"`
	Type          JobType       `json:"type" yaml:"type"`
	OwnerID       string        `json:"owner_id" yaml:"owner_id"`
	Image         string        `json:"image" yaml:"image"`
	Resources     *ResourceSpec  `json:"resources,omitempty" yaml:"resources,omitempty"`
	Stages        []*Stage      `json:"stages" yaml:"stages"`
	Status        JobStatus     `json:"status" yaml:"status"`
	FailurePolicy string        `json:"failure_policy" yaml:"failure_policy"`
	MaxRetries    int           `json:"max_retries" yaml:"max_retries"`
	MaxDuration   time.Duration `json:"max_duration" yaml:"max_duration"`
	CreatedAt     time.Time     `json:"created_at" yaml:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at" yaml:"updated_at"`
	CompletedAt   *time.Time    `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
}

// ========== 工具函数 ==========

// GenerateID 生成唯一 ID
func GenerateID(prefix string) string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%s-%x", prefix, b)
}

// NewJob 创建新作业
func NewJob(name string, jobType JobType, ownerID string) *Job {
	now := time.Now()
	j := &Job{
		ID:         GenerateID("job"),
		Name:       name,
		Type:       jobType,
		OwnerID:    ownerID,
		Status:     JobStatusPending,
		MaxRetries: 3,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if jobType == JobTypeSingle {
		j.FailurePolicy = "auto_retry"
	}
	return j
}

// NewStage 创建新阶段
func NewStage(jobID, name string, resources *ResourceSpec) *Stage {
	return &Stage{
		ID:             GenerateID("stage"),
		JobID:          jobID,
		Name:           name,
		Resources:      resources,
		Status:         StageStatusPending,
		MaxConcurrency: 10,
		Units:          make([]*Unit, 0),
	}
}

// NewUnit 创建新工作单元
func NewUnit(stageID, jobID string, index int, input string) *Unit {
	return &Unit{
		ID:     GenerateID("unit"),
		StageID: stageID,
		JobID:  jobID,
		Index:  index,
		Input:  input,
		Status: UnitStatusPending,
	}
}

// ========== 拆分逻辑 ==========

// ExecuteSplit 执行拆分策略，返回拆分后的 Unit 列表
func ExecuteSplit(stage *Stage) ([]*Unit, error) {
	if stage.Split == nil {
		// 没有拆分策略，创建一个 Unit
		unit := NewUnit(stage.ID, stage.JobID, 0, "")
		return []*Unit{unit}, nil
	}

	switch stage.Split.Type {
	case SplitTypeByFile:
		return splitByFile(stage)
	case SplitTypeByRange:
		return splitByRange(stage)
	case SplitTypeByN:
		return splitByN(stage)
	case SplitTypeByCustom:
		return splitByCustom(stage)
	default:
		return nil, fmt.Errorf("unknown split type: %v", stage.Split.Type)
	}
}

func splitByFile(stage *Stage) ([]*Unit, error) {
	if stage.Split.ByFile == nil {
		return nil, fmt.Errorf("by_file split config is nil")
	}
	var files []string
	if len(stage.Split.ByFile.FileList) > 0 {
		files = stage.Split.ByFile.FileList
	} else if stage.Split.ByFile.InputPattern != "" {
		// 注意：这里只是创建占位，实际文件列表由调度器运行时解析
		files = []string{stage.Split.ByFile.InputPattern}
	} else {
		return nil, fmt.Errorf("no file list or input pattern provided")
	}

	units := make([]*Unit, len(files))
	for i, file := range files {
		units[i] = NewUnit(stage.ID, stage.JobID, i, file)
	}
	return units, nil
}

func splitByRange(stage *Stage) ([]*Unit, error) {
	if stage.Split.ByRange == nil {
		return nil, fmt.Errorf("by_range split config is nil")
	}
	cfg := stage.Split.ByRange
	total := cfg.End - cfg.Start
	partSize := total / int64(cfg.NumParts)
	remainder := total % int64(cfg.NumParts)

	units := make([]*Unit, cfg.NumParts)
	start := cfg.Start
	for i := int32(0); i < cfg.NumParts; i++ {
		end := start + partSize - 1
		if int64(i) < remainder {
			end++
		}
		if i == cfg.NumParts-1 {
			end = cfg.End
		}
		units[i] = NewUnit(stage.ID, stage.JobID, int(i), fmt.Sprintf("%d-%d", start, end))
		start = end + 1
	}
	return units, nil
}

func splitByN(stage *Stage) ([]*Unit, error) {
	if stage.Split.ByN == nil {
		return nil, fmt.Errorf("by_n split config is nil")
	}
	units := make([]*Unit, stage.Split.ByN.NumParts)
	for i := int32(0); i < stage.Split.ByN.NumParts; i++ {
		units[i] = NewUnit(stage.ID, stage.JobID, int(i), fmt.Sprintf("part-%d", i))
	}
	return units, nil
}

func splitByCustom(stage *Stage) ([]*Unit, error) {
	if stage.Split.ByCustom == nil {
		return nil, fmt.Errorf("by_custom split config is nil")
	}
	// 自定义拆分：由用户提供拆分列表，这里创建一个占位 Unit
	unit := NewUnit(stage.ID, stage.JobID, 0, "")
	unit.Input = strings.Join(stage.Split.ByCustom.Args, " ")
	return []*Unit{unit}, nil
}

// ========== 工作流 DAG 工具 ==========

// TopologicalSort 对工作流的 Stage 进行拓扑排序
// 返回排序后的 Stage 列表，如果存在环则返回错误
func TopologicalSort(stages []*Stage) ([]*Stage, error) {
	// 构建入度表和依赖图
	inDegree := make(map[string]int)
	dependencies := make(map[string][]string) // stage -> 依赖它的 stages
	nameMap := make(map[string]*Stage)

	for _, stage := range stages {
		nameMap[stage.Name] = stage
		if _, ok := inDegree[stage.Name]; !ok {
			inDegree[stage.Name] = 0
		}
	}

	for _, stage := range stages {
		for _, dep := range stage.DependsOn {
			if _, ok := nameMap[dep]; !ok {
				return nil, fmt.Errorf("stage %q depends on %q which does not exist", stage.Name, dep)
			}
			dependencies[dep] = append(dependencies[dep], stage.Name)
			inDegree[stage.Name]++
		}
	}

	// Kahn 算法
	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	var result []*Stage
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		result = append(result, nameMap[name])

		for _, dependent := range dependencies[name] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(result) != len(stages) {
		return nil, fmt.Errorf("cycle detected in workflow stages")
	}
	return result, nil
}

// ValidateWorkflow 验证工作流配置
func ValidateWorkflow(job *Job) error {
	if job.Type != JobTypeWorkflow {
		return nil
	}
	if len(job.Stages) == 0 {
		return fmt.Errorf("workflow must have at least one stage")
	}

	// 检查 Stage 名称唯一性
	names := make(map[string]bool)
	for _, stage := range job.Stages {
		if names[stage.Name] {
			return fmt.Errorf("duplicate stage name: %q", stage.Name)
		}
		names[stage.Name] = true
	}

	// 检查依赖关系
	for _, stage := range job.Stages {
		for _, dep := range stage.DependsOn {
			if !names[dep] {
				return fmt.Errorf("stage %q depends on %q which does not exist", stage.Name, dep)
			}
		}
	}

	// 拓扑排序检测环
	_, err := TopologicalSort(job.Stages)
	return err
}