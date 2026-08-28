package container

import (
	"strings"
	"testing"
)

func TestParseNvidiaSmiCSV_Normal(t *testing.T) {
	input := `0, GPU-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa, NVIDIA GeForce RTX 3090, 24576, 12345
1, GPU-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb, NVIDIA GeForce RTX 3080, 10240, 6789
`
	gpus, err := parseNvidiaSmiCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 2 {
		t.Fatalf("expected 2 GPUs, got %d", len(gpus))
	}

	// First GPU
	if gpus[0].UUID != "GPU-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Errorf("expected UUID GPU-aaa..., got %s", gpus[0].UUID)
	}
	if gpus[0].Model != "NVIDIA GeForce RTX 3090" {
		t.Errorf("expected RTX 3090, got %s", gpus[0].Model)
	}
	if gpus[0].MemoryTotalMB != 24576 {
		t.Errorf("expected 24576 MB total, got %d", gpus[0].MemoryTotalMB)
	}
	if gpus[0].MemoryAvailMB != 12345 {
		t.Errorf("expected 12345 MB avail, got %d", gpus[0].MemoryAvailMB)
	}

	// Second GPU
	if gpus[1].UUID != "GPU-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" {
		t.Errorf("expected UUID GPU-bbb..., got %s", gpus[1].UUID)
	}
	if gpus[1].MemoryTotalMB != 10240 {
		t.Errorf("expected 10240 MB total, got %d", gpus[1].MemoryTotalMB)
	}
}

func TestParseNvidiaSmiCSV_Empty(t *testing.T) {
	input := ""
	gpus, err := parseNvidiaSmiCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 0 {
		t.Errorf("expected 0 GPUs, got %d", len(gpus))
	}
}

func TestParseNvidiaSmiCSV_MalformedLine(t *testing.T) {
	// 第二行字段数不足，应跳过
	input := `0, GPU-aaa, RTX 3090, 24576, 12345
bad line
2, GPU-ccc, RTX 3080, 10240, 6789
`
	gpus, err := parseNvidiaSmiCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 2 {
		t.Errorf("expected 2 valid GPUs, got %d", len(gpus))
	}
}

func TestParseNvidiaSmiCSV_NAMemory(t *testing.T) {
	// memory.free 为 [N/A] 时应回退到 total
	input := `0, GPU-aaa, NVIDIA GeForce RTX 3090, 24576, [N/A]
`
	gpus, err := parseNvidiaSmiCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 1 {
		t.Fatalf("expected 1 GPU, got %d", len(gpus))
	}
	if gpus[0].MemoryAvailMB != 24576 {
		t.Errorf("expected avail=24576 (fallback to total), got %d", gpus[0].MemoryAvailMB)
	}
}

func TestParseNvidiaSmiCSV_BOM(t *testing.T) {
	// CSV 可能包含 BOM
	input := "\ufeff0, GPU-aaa, RTX 3090, 24576, 12345\n"
	gpus, err := parseNvidiaSmiCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 1 {
		t.Fatalf("expected 1 GPU, got %d", len(gpus))
	}
}

func TestParseInt64(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"12345", 12345},
		{"0", 0},
		{"", 0},
		{"[N/A]", 0},
		{"  \t", 0},
		{"-1", -1},
	}
	for _, tt := range tests {
		got := parseInt64(tt.input)
		if got != tt.want {
			t.Errorf("parseInt64(%q)=%d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseNvidiaSmiCSV_WhitespacePadding(t *testing.T) {
	// nvidia-smi 有时会在值周围加空格
	input := `0 , GPU-aaa , RTX 3090 , 24576 , 12345
`
	gpus, err := parseNvidiaSmiCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 1 {
		t.Fatalf("expected 1 GPU, got %d", len(gpus))
	}
	if gpus[0].UUID != "GPU-aaa" {
		t.Errorf("expected UUID GPU-aaa, got %s", gpus[0].UUID)
	}
}