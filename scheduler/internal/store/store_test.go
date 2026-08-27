package store

import (
	"os"
	"path/filepath"
	"testing"

	pb "computing-power/proto/v1"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open test db: %v", err)
	}
	t.Cleanup(func() {
		st.Close()
		os.Remove(path)
	})
	return st
}

func TestSaveAndGetJob(t *testing.T) {
	st := newTestStore(t)
	job := &pb.Job{
		ID:      "job-1",
		Name:    "test-job",
		Type:    pb.JobTypeSingle,
		OwnerID: "node-1",
		Status:  pb.JobStatusPending,
	}
	if err := st.SaveJob(job); err != nil {
		t.Fatalf("SaveJob: %v", err)
	}
	got, err := st.GetJob("job-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got == nil {
		t.Fatal("GetJob returned nil")
	}
	if got.Name != "test-job" {
		t.Errorf("expected Name test-job, got %s", got.Name)
	}
}

func TestListJobs(t *testing.T) {
	st := newTestStore(t)
	jobs := []*pb.Job{
		{ID: "job-1", OwnerID: "alice", Status: pb.JobStatusPending},
		{ID: "job-2", OwnerID: "bob", Status: pb.JobStatusRunning},
		{ID: "job-3", OwnerID: "alice", Status: pb.JobStatusCompleted},
	}
	for _, j := range jobs {
		if err := st.SaveJob(j); err != nil {
			t.Fatalf("SaveJob: %v", err)
		}
	}

	// List all
	all, err := st.ListJobs("")
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(all))
	}

	// Filter by owner
	aliceJobs, err := st.ListJobs("alice")
	if err != nil {
		t.Fatalf("ListJobs(alice): %v", err)
	}
	if len(aliceJobs) != 2 {
		t.Fatalf("expected 2 alice jobs, got %d", len(aliceJobs))
	}
}

func TestListJobsByStatus(t *testing.T) {
	st := newTestStore(t)
	jobs := []*pb.Job{
		{ID: "job-1", Status: pb.JobStatusPending, OwnerID: "alice"},
		{ID: "job-2", Status: pb.JobStatusRunning, OwnerID: "bob"},
		{ID: "job-3", Status: pb.JobStatusPending, OwnerID: "alice"},
		{ID: "job-4", Status: pb.JobStatusCompleted, OwnerID: "bob"},
	}
	for _, j := range jobs {
		if err := st.SaveJob(j); err != nil {
			t.Fatalf("SaveJob: %v", err)
		}
	}

	pending, err := st.ListJobsByStatus(pb.JobStatusPending)
	if err != nil {
		t.Fatalf("ListJobsByStatus: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending jobs, got %d", len(pending))
	}
}

func TestUpdateJobStatus(t *testing.T) {
	st := newTestStore(t)
	job := &pb.Job{ID: "job-1", Status: pb.JobStatusPending, OwnerID: "alice"}
	if err := st.SaveJob(job); err != nil {
		t.Fatalf("SaveJob: %v", err)
	}

	if err := st.UpdateJobStatus("job-1", pb.JobStatusRunning); err != nil {
		t.Fatalf("UpdateJobStatus: %v", err)
	}

	got, err := st.GetJob("job-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Status != pb.JobStatusRunning {
		t.Errorf("expected Running, got %v", got.Status)
	}
}

func TestSaveAndGetUnit(t *testing.T) {
	st := newTestStore(t)
	unit := &pb.Unit{
		ID:      "unit-1",
		StageID: "stage-1",
		JobID:   "job-1",
		Index:   0,
		Status:  pb.UnitStatusPending,
	}
	if err := st.SaveUnit(unit); err != nil {
		t.Fatalf("SaveUnit: %v", err)
	}
	got, err := st.GetUnit("unit-1")
	if err != nil {
		t.Fatalf("GetUnit: %v", err)
	}
	if got == nil {
		t.Fatal("GetUnit returned nil")
	}
	if got.StageID != "stage-1" {
		t.Errorf("expected stage-1, got %s", got.StageID)
	}
}

func TestListUnitsByStage(t *testing.T) {
	st := newTestStore(t)
	units := []*pb.Unit{
		{ID: "u1", StageID: "stage-1", JobID: "job-1", Status: pb.UnitStatusPending},
		{ID: "u2", StageID: "stage-1", JobID: "job-1", Status: pb.UnitStatusRunning},
		{ID: "u3", StageID: "stage-2", JobID: "job-1", Status: pb.UnitStatusPending},
	}
	for _, u := range units {
		if err := st.SaveUnit(u); err != nil {
			t.Fatalf("SaveUnit: %v", err)
		}
	}

	stage1Units, err := st.ListUnitsByStage("stage-1")
	if err != nil {
		t.Fatalf("ListUnitsByStage: %v", err)
	}
	if len(stage1Units) != 2 {
		t.Fatalf("expected 2 units for stage-1, got %d", len(stage1Units))
	}
}

func TestListUnitsByJob(t *testing.T) {
	st := newTestStore(t)
	units := []*pb.Unit{
		{ID: "u1", JobID: "job-1", StageID: "s1", Status: pb.UnitStatusPending},
		{ID: "u2", JobID: "job-1", StageID: "s1", Status: pb.UnitStatusRunning},
		{ID: "u3", JobID: "job-2", StageID: "s2", Status: pb.UnitStatusPending},
	}
	for _, u := range units {
		if err := st.SaveUnit(u); err != nil {
			t.Fatalf("SaveUnit: %v", err)
		}
	}

	job1Units, err := st.ListUnitsByJob("job-1")
	if err != nil {
		t.Fatalf("ListUnitsByJob: %v", err)
	}
	if len(job1Units) != 2 {
		t.Fatalf("expected 2 units for job-1, got %d", len(job1Units))
	}
}

func TestUpdateUnitStatus(t *testing.T) {
	st := newTestStore(t)
	unit := &pb.Unit{
		ID:      "unit-1",
		StageID: "stage-1",
		JobID:   "job-1",
		Status:  pb.UnitStatusPending,
	}
	if err := st.SaveUnit(unit); err != nil {
		t.Fatalf("SaveUnit: %v", err)
	}

	updated, err := st.UpdateUnitStatus("unit-1", pb.UnitStatusCompleted, 0, "")
	if err != nil {
		t.Fatalf("UpdateUnitStatus: %v", err)
	}
	if updated.Status != pb.UnitStatusCompleted {
		t.Errorf("expected Completed, got %v", updated.Status)
	}
	if updated.CompletedAt == 0 {
		t.Errorf("CompletedAt should be set")
	}

	// Verify persisted
	got, err := st.GetUnit("unit-1")
	if err != nil {
		t.Fatalf("GetUnit: %v", err)
	}
	if got.Status != pb.UnitStatusCompleted {
		t.Errorf("expected Completed after reload, got %v", got.Status)
	}
}

func TestUpdateUnitStatus_NotFound(t *testing.T) {
	st := newTestStore(t)
	_, err := st.UpdateUnitStatus("nonexistent", pb.UnitStatusCompleted, 0, "")
	if err == nil {
		t.Fatal("expected error for nonexistent unit")
	}
}

func TestUpdateStageStatus(t *testing.T) {
	st := newTestStore(t)
	stage := &pb.Stage{
		ID:     "stage-1",
		JobID:  "job-1",
		Name:   "test-stage",
		Status: pb.StageStatusPending,
	}
	job := &pb.Job{
		ID:       "job-1",
		Name:     "test-job",
		OwnerID:  "alice",
		Status:   pb.JobStatusPending,
		Stages:   []*pb.Stage{stage},
	}
	if err := st.SaveJob(job); err != nil {
		t.Fatalf("SaveJob: %v", err)
	}

	if err := st.UpdateStageStatus("stage-1", pb.StageStatusRunning); err != nil {
		t.Fatalf("UpdateStageStatus: %v", err)
	}

	got, err := st.GetJob("job-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if len(got.Stages) != 1 || got.Stages[0].Status != pb.StageStatusRunning {
		t.Errorf("expected stage Running, got %v", got.Stages[0].Status)
	}
}

func TestDeleteJob(t *testing.T) {
	st := newTestStore(t)
	job := &pb.Job{ID: "job-1", Name: "test", OwnerID: "alice", Status: pb.JobStatusPending}
	if err := st.SaveJob(job); err != nil {
		t.Fatalf("SaveJob: %v", err)
	}
	units := []*pb.Unit{
		{ID: "u1", JobID: "job-1", StageID: "s1", Status: pb.UnitStatusPending},
		{ID: "u2", JobID: "job-1", StageID: "s1", Status: pb.UnitStatusPending},
	}
	for _, u := range units {
		if err := st.SaveUnit(u); err != nil {
			t.Fatalf("SaveUnit: %v", err)
		}
	}

	if err := st.DeleteJob("job-1"); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}

	// Verify job deleted
	got, _ := st.GetJob("job-1")
	if got != nil {
		t.Error("job should be deleted")
	}

	// Verify units deleted
	jobUnits, err := st.ListUnitsByJob("job-1")
	if err != nil {
		t.Fatalf("ListUnitsByJob: %v", err)
	}
	if len(jobUnits) != 0 {
		t.Errorf("expected 0 units after delete, got %d", len(jobUnits))
	}
}

func TestListUnitsByStatus(t *testing.T) {
	st := newTestStore(t)
	units := []*pb.Unit{
		{ID: "u1", StageID: "s1", JobID: "j1", Status: pb.UnitStatusPending},
		{ID: "u2", StageID: "s1", JobID: "j1", Status: pb.UnitStatusRunning},
		{ID: "u3", StageID: "s1", JobID: "j1", Status: pb.UnitStatusCompleted},
	}
	for _, u := range units {
		if err := st.SaveUnit(u); err != nil {
			t.Fatalf("SaveUnit: %v", err)
		}
	}

	pending, err := st.ListUnitsByStatus(pb.UnitStatusPending)
	if err != nil {
		t.Fatalf("ListUnitsByStatus: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending unit, got %d", len(pending))
	}
}