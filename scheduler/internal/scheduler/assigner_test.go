package scheduler

import (
	"testing"
	"time"

	pb "computing-power/proto/v1"
)

func setupTestNode(t *testing.T, eng *Engine, id string, status pb.NodeStatus, cpuUsage float64) {
	t.Helper()
	node := makeNode(id, status, cpuUsage)
	eng.registry.Register(node)
}

func TestAssignUnit_TransitionsToAssigned(t *testing.T) {
	eng, st, reg, _ := newTestEngineWithStore(t)

	setupTestNode(t, eng, "n1", pb.NodeStatusOnline, 0.3)

	job := &pb.Job{ID: "job-1", Name: "test", OwnerID: "n1", Status: pb.JobStatusPending, Image: "alpine:latest"}
	if err := st.SaveJob(job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	unit := &pb.Unit{
		ID:      "unit-1",
		JobID:   "job-1",
		StageID: "stage-1",
		Index:   0,
		Status:  pb.UnitStatusPending,
	}
	if err := st.SaveUnit(unit); err != nil {
		t.Fatalf("save unit: %v", err)
	}

	node := reg.GetNode("n1")
	if node == nil {
		t.Fatal("node not found")
	}

	ok := eng.assignUnit(unit, node, job)
	if !ok {
		t.Fatal("assignUnit returned false")
	}

	// 验证状态
	updated, err := st.GetUnit("unit-1")
	if err != nil {
		t.Fatalf("get unit: %v", err)
	}
	if updated.Status != pb.UnitStatusAssigned {
		t.Errorf("expected Assigned, got %v", updated.Status)
	}
	if updated.AssignedNode != "n1" {
		t.Errorf("expected n1, got %s", updated.AssignedNode)
	}
}

func TestAssignUnit_AlreadyAssigned(t *testing.T) {
	eng, st, reg, _ := newTestEngineWithStore(t)

	setupTestNode(t, eng, "n1", pb.NodeStatusOnline, 0.3)

	job := &pb.Job{ID: "job-1", Name: "test", OwnerID: "n1", Status: pb.JobStatusPending, Image: "alpine:latest"}
	st.SaveJob(job)

	unit := &pb.Unit{
		ID:      "unit-1",
		JobID:   "job-1",
		StageID: "stage-1",
		Status:  pb.UnitStatusRunning, // 已非 Pending
	}
	st.SaveUnit(unit)

	node := reg.GetNode("n1")
	ok := eng.assignUnit(unit, node, job)
	if ok {
		t.Error("expected false for already assigned unit")
	}
}

func TestAssignUnit_CommandQueued(t *testing.T) {
	eng, st, reg, _ := newTestEngineWithStore(t)

	setupTestNode(t, eng, "n1", pb.NodeStatusOnline, 0.3)

	job := &pb.Job{ID: "job-1", OwnerID: "n1", Status: pb.JobStatusPending, Image: "ubuntu:22.04"}
	st.SaveJob(job)

	unit := &pb.Unit{ID: "unit-1", JobID: "job-1", StageID: "stage-1", Status: pb.UnitStatusPending}
	st.SaveUnit(unit)

	node := reg.GetNode("n1")
	eng.assignUnit(unit, node, job)

	// 验证命令已入队
	cmds := eng.PopCommands("n1")
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Type != "assign" {
		t.Errorf("expected type assign, got %s", cmds[0].Type)
	}
}

func TestLoadRetryEligibleUnits_UnderLimit(t *testing.T) {
	eng, st, _, _ := newTestEngineWithStore(t)
	eng.reassignDelay = time.Millisecond // 立即重试

	now := time.Now().Add(-10 * time.Millisecond).UnixMilli()
	unit := &pb.Unit{
		ID:          "unit-1",
		JobID:       "job-1",
		StageID:     "stage-1",
		Status:      pb.UnitStatusFailed,
		RetryCount:  1, // 3 次上限，还有余量
		CompletedAt: now,
	}
	if err := st.SaveUnit(unit); err != nil {
		t.Fatalf("save unit: %v", err)
	}

	retryable, err := eng.loadRetryEligibleUnits()
	if err != nil {
		t.Fatalf("loadRetryEligibleUnits: %v", err)
	}
	if len(retryable) != 1 {
		t.Fatalf("expected 1 retryable, got %d", len(retryable))
	}
	if retryable[0].Status != pb.UnitStatusPending {
		t.Errorf("expected Pending status for retry, got %v", retryable[0].Status)
	}
}

func TestLoadRetryEligibleUnits_Exhausted(t *testing.T) {
	eng, st, _, _ := newTestEngineWithStore(t)

	now := time.Now().UnixMilli()
	unit := &pb.Unit{
		ID:          "unit-1",
		JobID:       "job-1",
		StageID:     "stage-1",
		Status:      pb.UnitStatusFailed,
		RetryCount:  3, // 已达到上限
		CompletedAt: now,
	}
	st.SaveUnit(unit)

	retryable, err := eng.loadRetryEligibleUnits()
	if err != nil {
		t.Fatalf("loadRetryEligibleUnits: %v", err)
	}
	if len(retryable) != 0 {
		t.Errorf("expected 0 retryable, got %d", len(retryable))
	}
}

func TestLoadRetryEligibleUnits_DelayNotMet(t *testing.T) {
	eng, st, _, _ := newTestEngineWithStore(t)
	eng.reassignDelay = 10 * time.Second // 10s 延迟

	unit := &pb.Unit{
		ID:          "unit-1",
		JobID:       "job-1",
		StageID:     "stage-1",
		Status:      pb.UnitStatusFailed,
		RetryCount:  1,
		CompletedAt: time.Now().UnixMilli(), // 刚失败
	}
	st.SaveUnit(unit)

	retryable, err := eng.loadRetryEligibleUnits()
	if err != nil {
		t.Fatalf("loadRetryEligibleUnits: %v", err)
	}
	if len(retryable) != 0 {
		t.Errorf("expected 0 retryable, got %d", len(retryable))
	}
}

func TestBuildAssignCommand(t *testing.T) {
	unit := &pb.Unit{ID: "unit-1", JobID: "job-1", StageID: "stage-1", Input: "test.txt", Index: 0}
	job := &pb.Job{Image: "alpine:latest"}

	cmd := buildAssignCommand(unit, job)
	if cmd.Type != "assign" {
		t.Errorf("expected type assign, got %s", cmd.Type)
	}
	if len(cmd.Payload) == 0 {
		t.Errorf("expected non-empty payload")
	}
}

func TestGetResourceSpecForUnit_StageLevel(t *testing.T) {
	job := &pb.Job{
		ID: "job-1",
		Stages: []*pb.Stage{
			{ID: "stage-1", Resources: &pb.ResourceSpec{CPUCores: 2}},
			{ID: "stage-2", Resources: &pb.ResourceSpec{CPUCores: 4}},
		},
	}
	unit := &pb.Unit{StageID: "stage-1", JobID: "job-1"}

	spec := getResourceSpecForUnit(unit, job)
	if spec == nil || spec.CPUCores != 2 {
		t.Errorf("expected 2 cores, got %v", spec)
	}
}

func TestGetResourceSpecForUnit_JobLevel(t *testing.T) {
	job := &pb.Job{
		ID:        "job-1",
		Resources: &pb.ResourceSpec{CPUCores: 8},
		Stages:    []*pb.Stage{{ID: "stage-1"}},
	}
	// Stage 没有 Resources，应该回退到 Job 级别
	unit := &pb.Unit{StageID: "stage-other", JobID: "job-1"}

	spec := getResourceSpecForUnit(unit, job)
	if spec == nil || spec.CPUCores != 8 {
		t.Errorf("expected 8 cores (job-level), got %v", spec)
	}
}

func TestAssignToNode_NoPendingUnits(t *testing.T) {
	eng, _, _, _ := newTestEngineWithStore(t)
	setupTestNode(t, eng, "n1", pb.NodeStatusOnline, 0.3)

	unit, err := eng.AssignToNode("n1")
	if err != nil {
		t.Fatalf("AssignToNode: %v", err)
	}
	if unit != nil {
		t.Errorf("expected nil, got %v", unit)
	}
}

func TestAssignToNode_NodeNotFound(t *testing.T) {
	eng, _, _, _ := newTestEngineWithStore(t)
	unit, err := eng.AssignToNode("nonexistent")
	if err != nil {
		t.Fatalf("AssignToNode: %v", err)
	}
	if unit != nil {
		t.Errorf("expected nil, got %v", unit)
	}
}

func TestFindStageByName(t *testing.T) {
	stages := []*pb.Stage{
		{Name: "stage-a", ID: "s1"},
		{Name: "stage-b", ID: "s2"},
	}
	s := findStageByName(stages, "stage-a")
	if s == nil || s.ID != "s1" {
		t.Errorf("expected s1, got %v", s)
	}
	s = findStageByName(stages, "nonexistent")
	if s != nil {
		t.Errorf("expected nil, got %v", s)
	}
}

func TestConvertStages(t *testing.T) {
	protoStages := []*pb.Stage{
		{Name: "download", DependsOn: nil},
		{Name: "process", DependsOn: []string{"download"}},
	}
	tmStages := convertStages(protoStages)
	if len(tmStages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(tmStages))
	}
	if tmStages[0].Name != "download" {
		t.Errorf("expected download, got %s", tmStages[0].Name)
	}
	if len(tmStages[1].DependsOn) != 1 || tmStages[1].DependsOn[0] != "download" {
		t.Errorf("expected depends on download, got %v", tmStages[1].DependsOn)
	}
}

func TestScheduleJob_Single(t *testing.T) {
	eng, st, _, _ := newTestEngineWithStore(t)
	setupTestNode(t, eng, "n1", pb.NodeStatusOnline, 0.3)

	job := &pb.Job{
		ID: "job-1", OwnerID: "n1", Status: pb.JobStatusPending, Type: pb.JobTypeSingle,
		Image: "alpine:latest",
		Stages: []*pb.Stage{{ID: "stage-1", Resources: &pb.ResourceSpec{CPUCores: 1, MemoryBytes: 512}}},
	}
	st.SaveJob(job)

	unit := &pb.Unit{ID: "unit-1", JobID: "job-1", StageID: "stage-1", Status: pb.UnitStatusPending}
	st.SaveUnit(unit)

	eng.scheduleJob(job, []*pb.Unit{unit})

	updated, _ := st.GetUnit("unit-1")
	if updated.Status != pb.UnitStatusAssigned {
		t.Errorf("expected Assigned, got %v", updated.Status)
	}
}

func TestScheduleJob_WorkflowBlocked(t *testing.T) {
	eng, st, _, _ := newTestEngineWithStore(t)
	setupTestNode(t, eng, "n1", pb.NodeStatusOnline, 0.3)

	job := &pb.Job{
		ID: "job-1", OwnerID: "n1", Status: pb.JobStatusPending, Type: pb.JobTypeWorkflow,
		Image: "alpine:latest",
		Stages: []*pb.Stage{
			{ID: "stage-1", Name: "download", Status: pb.StageStatusPending, Resources: &pb.ResourceSpec{CPUCores: 1}},
			{ID: "stage-2", Name: "process", DependsOn: []string{"download"}, Status: pb.StageStatusPending, Resources: &pb.ResourceSpec{CPUCores: 1}},
		},
	}
	st.SaveJob(job)

	// stage-2 的 unit 不应被调度（依赖的 download 未完成）
	unit := &pb.Unit{ID: "unit-2", JobID: "job-1", StageID: "stage-2", Status: pb.UnitStatusPending}
	st.SaveUnit(unit)

	eng.scheduleJob(job, []*pb.Unit{unit})

	updated, _ := st.GetUnit("unit-2")
	if updated.Status == pb.UnitStatusAssigned {
		t.Errorf("expected unit not assigned (dependency not met), but got Assigned")
	}
}

func TestScheduleJob_WorkflowReady(t *testing.T) {
	eng, st, _, _ := newTestEngineWithStore(t)
	setupTestNode(t, eng, "n1", pb.NodeStatusOnline, 0.3)

	job := &pb.Job{
		ID: "job-1", OwnerID: "n1", Status: pb.JobStatusRunning, Type: pb.JobTypeWorkflow,
		Image: "alpine:latest",
		Stages: []*pb.Stage{
			{ID: "stage-1", Name: "download", Status: pb.StageStatusCompleted, Resources: &pb.ResourceSpec{CPUCores: 1}},
			{ID: "stage-2", Name: "process", DependsOn: []string{"download"}, Status: pb.StageStatusPending, Resources: &pb.ResourceSpec{CPUCores: 1}},
		},
	}
	st.SaveJob(job)

	// stage-2 的 unit 应被调度（download 已完成）
	unit := &pb.Unit{ID: "unit-2", JobID: "job-1", StageID: "stage-2", Status: pb.UnitStatusPending}
	st.SaveUnit(unit)

	eng.scheduleJob(job, []*pb.Unit{unit})

	updated, _ := st.GetUnit("unit-2")
	if updated.Status != pb.UnitStatusAssigned {
		t.Errorf("expected unit assigned, got %v", updated.Status)
	}
}