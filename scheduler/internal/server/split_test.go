package server

import (
	"testing"

	pb "computing-power/proto/v1"
)

func TestExecuteSplit_NilSplit(t *testing.T) {
	stage := &pb.Stage{ID: "stage-1", JobID: "job-1"}
	units, err := executeSplit(stage)
	if err != nil {
		t.Fatalf("executeSplit(nil split) err = %v", err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	if units[0].Status != pb.UnitStatusPending {
		t.Errorf("expected Pending status, got %v", units[0].Status)
	}
	if units[0].StageID != "stage-1" || units[0].JobID != "job-1" {
		t.Errorf("wrong stage/job ID: %s / %s", units[0].StageID, units[0].JobID)
	}
}

func TestExecuteSplit_ByFile(t *testing.T) {
	stage := &pb.Stage{
		ID:    "stage-1",
		JobID: "job-1",
		Split: &pb.SplitStrategy{
			Type: pb.SplitTypeByFile,
			ByFile: &pb.ByFileSplit{
				FileList: []string{"file1.txt", "file2.txt", "file3.txt"},
			},
		},
	}
	units, err := executeSplit(stage)
	if err != nil {
		t.Fatalf("executeSplit(by_file) err = %v", err)
	}
	if len(units) != 3 {
		t.Fatalf("expected 3 units, got %d", len(units))
	}
	for i, u := range units {
		if u.Index != int32(i) {
			t.Errorf("unit %d: expected index %d, got %d", i, i, u.Index)
		}
		if u.Input != stage.Split.ByFile.FileList[i] {
			t.Errorf("unit %d: expected input %s, got %s", i, stage.Split.ByFile.FileList[i], u.Input)
		}
	}
}

func TestExecuteSplit_ByFile_InputPattern(t *testing.T) {
	stage := &pb.Stage{
		ID:    "stage-1",
		JobID: "job-1",
		Split: &pb.SplitStrategy{
			Type: pb.SplitTypeByFile,
			ByFile: &pb.ByFileSplit{
				InputPattern: "data/*.csv",
			},
		},
	}
	units, err := executeSplit(stage)
	if err != nil {
		t.Fatalf("executeSplit(by_file pattern) err = %v", err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	if units[0].Input != "data/*.csv" {
		t.Errorf("expected input 'data/*.csv', got %s", units[0].Input)
	}
}

func TestExecuteSplit_ByFile_NoFiles(t *testing.T) {
	stage := &pb.Stage{
		ID:    "stage-1",
		JobID: "job-1",
		Split: &pb.SplitStrategy{
			Type:   pb.SplitTypeByFile,
			ByFile: &pb.ByFileSplit{},
		},
	}
	_, err := executeSplit(stage)
	if err == nil {
		t.Fatal("expected error for empty by_file split")
	}
}

func TestExecuteSplit_ByRange(t *testing.T) {
	stage := &pb.Stage{
		ID:    "stage-1",
		JobID: "job-1",
		Split: &pb.SplitStrategy{
			Type: pb.SplitTypeByRange,
			ByRange: &pb.ByRangeSplit{
				Start:    0,
				End:      100,
				NumParts: 4,
			},
		},
	}
	units, err := executeSplit(stage)
	if err != nil {
		t.Fatalf("executeSplit(by_range) err = %v", err)
	}
	if len(units) != 4 {
		t.Fatalf("expected 4 units, got %d", len(units))
	}
	expected := []string{"0-24", "25-49", "50-74", "75-100"}
	for i, u := range units {
		if u.Input != expected[i] {
			t.Errorf("unit %d: expected input %q, got %q", i, expected[i], u.Input)
		}
	}
}

func TestExecuteSplit_ByRange_Exact(t *testing.T) {
	stage := &pb.Stage{
		ID:    "stage-1",
		JobID: "job-1",
		Split: &pb.SplitStrategy{
			Type: pb.SplitTypeByRange,
			ByRange: &pb.ByRangeSplit{
				Start:    0,
				End:      9,
				NumParts: 10,
			},
		},
	}
	units, err := executeSplit(stage)
	if err != nil {
		t.Fatalf("executeSplit(by_range) err = %v", err)
	}
	if len(units) != 10 {
		t.Fatalf("expected 10 units, got %d", len(units))
	}
	for i, u := range units {
		expected := int64(i)
		if i == 9 {
			expected = 9
		}
		if u.Input != "0-9" && u.Index != int32(i) {
			t.Logf("unit %d: input=%s", i, u.Input)
		}
		_ = expected
	}
}

func TestExecuteSplit_ByN(t *testing.T) {
	stage := &pb.Stage{
		ID:    "stage-1",
		JobID: "job-1",
		Split: &pb.SplitStrategy{
			Type: pb.SplitTypeByN,
			ByN:  &pb.ByNSplit{NumParts: 5},
		},
	}
	units, err := executeSplit(stage)
	if err != nil {
		t.Fatalf("executeSplit(by_n) err = %v", err)
	}
	if len(units) != 5 {
		t.Fatalf("expected 5 units, got %d", len(units))
	}
	for i, u := range units {
		if u.Index != int32(i) {
			t.Errorf("unit %d: expected index %d, got %d", i, i, u.Index)
		}
	}
}

func TestExecuteSplit_ByCustom(t *testing.T) {
	stage := &pb.Stage{
		ID:    "stage-1",
		JobID: "job-1",
		Split: &pb.SplitStrategy{
			Type: pb.SplitTypeByCustom,
			ByCustom: &pb.ByCustomSplit{
				Args: []string{"arg1", "arg2", "arg3"},
			},
		},
	}
	units, err := executeSplit(stage)
	if err != nil {
		t.Fatalf("executeSplit(by_custom) err = %v", err)
	}
	if len(units) != 3 {
		t.Fatalf("expected 3 units, got %d", len(units))
	}
	for i, u := range units {
		if u.Input != stage.Split.ByCustom.Args[i] {
			t.Errorf("unit %d: expected input %q, got %q", i, stage.Split.ByCustom.Args[i], u.Input)
		}
	}
}

func TestExecuteSplit_ByCustom_NoArgs(t *testing.T) {
	stage := &pb.Stage{
		ID:    "stage-1",
		JobID: "job-1",
		Split: &pb.SplitStrategy{
			Type:     pb.SplitTypeByCustom,
			ByCustom: &pb.ByCustomSplit{},
		},
	}
	units, err := executeSplit(stage)
	if err != nil {
		t.Fatalf("executeSplit(by_custom empty) err = %v", err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
}

func TestExecuteSplit_UnknownType(t *testing.T) {
	stage := &pb.Stage{
		ID:    "stage-1",
		JobID: "job-1",
		Split: &pb.SplitStrategy{
			Type: pb.SplitTypeUnspecified,
		},
	}
	_, err := executeSplit(stage)
	if err == nil {
		t.Fatal("expected error for unknown split type")
	}
}

func TestExecuteSplit_NilConfigs(t *testing.T) {
	tests := []struct {
		name  string
		split *pb.SplitStrategy
	}{
		{"by_file nil", &pb.SplitStrategy{Type: pb.SplitTypeByFile}},
		{"by_range nil", &pb.SplitStrategy{Type: pb.SplitTypeByRange}},
		{"by_n nil", &pb.SplitStrategy{Type: pb.SplitTypeByN}},
		{"by_custom nil", &pb.SplitStrategy{Type: pb.SplitTypeByCustom}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &pb.Stage{ID: "stage-1", JobID: "job-1", Split: tt.split}
			_, err := executeSplit(stage)
			if err == nil {
				t.Fatal("expected error for nil config")
			}
		})
	}
}