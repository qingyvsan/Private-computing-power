package taskmodel

import (
	"testing"
)

func TestSplitByN(t *testing.T) {
	stage := &Stage{
		ID:    "stage-1",
		JobID: "job-1",
		Split: &SplitStrategy{
			Type: SplitTypeByN,
			ByN:  &ByNSplit{NumParts: 5},
		},
	}

	units, err := ExecuteSplit(stage)
	if err != nil {
		t.Fatalf("split by_n: %v", err)
	}
	if len(units) != 5 {
		t.Fatalf("expected 5 units, got %d", len(units))
	}
}

func TestSplitByRange(t *testing.T) {
	stage := &Stage{
		ID:    "stage-1",
		JobID: "job-1",
		Split: &SplitStrategy{
			Type: SplitTypeByRange,
			ByRange: &ByRangeSplit{
				Start:    0,
				End:      100,
				NumParts: 4,
			},
		},
	}

	units, err := ExecuteSplit(stage)
	if err != nil {
		t.Fatalf("split by_range: %v", err)
	}
	if len(units) != 4 {
		t.Fatalf("expected 4 units, got %d", len(units))
	}
	if units[0].Input != "0-24" {
		t.Fatalf("expected first unit range 0-24, got %s", units[0].Input)
	}
	if units[3].Input != "75-100" {
		t.Fatalf("expected last unit range 75-100, got %s", units[3].Input)
	}
}

func TestSplitByFile(t *testing.T) {
	stage := &Stage{
		ID:    "stage-1",
		JobID: "job-1",
		Split: &SplitStrategy{
			Type: SplitTypeByFile,
			ByFile: &ByFileSplit{
				FileList: []string{"a.mp4", "b.mp4", "c.mp4"},
			},
		},
	}

	units, err := ExecuteSplit(stage)
	if err != nil {
		t.Fatalf("split by_file: %v", err)
	}
	if len(units) != 3 {
		t.Fatalf("expected 3 units, got %d", len(units))
	}
	if units[1].Input != "b.mp4" {
		t.Fatalf("expected second unit input b.mp4, got %s", units[1].Input)
	}
}

func TestTopologicalSort(t *testing.T) {
	stages := []*Stage{
		{Name: "download", DependsOn: []string{}},
		{Name: "preprocess", DependsOn: []string{"download"}},
		{Name: "train", DependsOn: []string{"preprocess"}},
		{Name: "evaluate", DependsOn: []string{"train"}},
	}

	ordered, err := TopologicalSort(stages)
	if err != nil {
		t.Fatalf("topological sort: %v", err)
	}
	if len(ordered) != 4 {
		t.Fatalf("expected 4 stages, got %d", len(ordered))
	}
	// 第一个应该是 download
	if ordered[0].Name != "download" {
		t.Fatalf("expected first stage download, got %s", ordered[0].Name)
	}
}

func TestTopologicalSortCycle(t *testing.T) {
	stages := []*Stage{
		{Name: "a", DependsOn: []string{"b"}},
		{Name: "b", DependsOn: []string{"a"}},
	}

	if _, err := TopologicalSort(stages); err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func TestValidateWorkflow(t *testing.T) {
	job := &Job{
		Type: JobTypeWorkflow,
		Stages: []*Stage{
			{Name: "a", DependsOn: []string{"b"}},
			{Name: "b", DependsOn: []string{"a"}},
		},
	}
	if err := ValidateWorkflow(job); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestNewJob(t *testing.T) {
	job := NewJob("test", JobTypeSingle, "node-1")
	if job.ID == "" {
		t.Fatal("job ID should be generated")
	}
	if job.Status != JobStatusPending {
		t.Fatal("job should start as pending")
	}
	if job.MaxRetries != 3 {
		t.Fatalf("expected default max retries 3, got %d", job.MaxRetries)
	}
	if job.FailurePolicy != "auto_retry" {
		t.Fatalf("expected default failure policy auto_retry, got %s", job.FailurePolicy)
	}
}
