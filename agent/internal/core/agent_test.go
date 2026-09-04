package core

import (
	"testing"
	"time"

	"computing-power/agent/internal/config"
)

func TestNew_DefaultConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Containerd.Socket = "nonexistent" // fast-fail, avoid 10s timeout
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New with default config: %v", err)
	}
	if a == nil {
		t.Fatal("New returned nil")
	}
}

func TestNew_WithUpdaterEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.Containerd.Socket = "nonexistent"
	cfg.Updater.Enabled = true
	cfg.Updater.CheckInterval = "1h"
	cfg.Updater.ManifestURL = "https://example.com/manifest"
	cfg.Updater.DownloadDir = t.TempDir()
	cfg.Updater.BackupCount = 2

	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New with updater: %v", err)
	}
	if a.updater == nil {
		t.Error("expected updater to be initialized")
	}
}

func TestNew_WithUpdaterDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.Containerd.Socket = "nonexistent"
	cfg.Updater.Enabled = false
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New with updater disabled: %v", err)
	}
	if a.updater != nil {
		t.Error("expected updater to be nil when disabled")
	}
}

func TestNodeID_EmptyBeforeRegistration(t *testing.T) {
	cfg := config.Default()
	cfg.Containerd.Socket = "nonexistent"
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.NodeID() != "" {
		t.Errorf("expected empty nodeID, got %q", a.NodeID())
	}
}

func TestSetOnRegistered(t *testing.T) {
	cfg := config.Default()
	cfg.Containerd.Socket = "nonexistent"
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	a.SetOnRegistered(func(nodeID string) {
		// callback stored successfully
	})

	if a.onRegistered == nil {
		t.Fatal("onRegistered callback was not set")
	}

	// Simulate registration
	a.nodeID = "test-node-id"
	if a.NodeID() != "test-node-id" {
		t.Errorf("expected test-node-id, got %s", a.NodeID())
	}
}

func TestNew_WithNebulaEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.Containerd.Socket = "nonexistent"
	cfg.Nebula.Enabled = true
	cfg.Nebula.BinaryPath = "nebula"
	cfg.Nebula.DataDir = t.TempDir()
	cfg.Nebula.ConfigPath = t.TempDir() + "/config.yaml"
	cfg.Nebula.CertDir = t.TempDir()

	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New with nebula: %v", err)
	}
	if a.nebulaMgr == nil {
		t.Error("expected nebula manager to be initialized")
	}
}

func TestNew_WithReportGPU(t *testing.T) {
	cfg := config.Default()
	cfg.Containerd.Socket = "nonexistent"
	cfg.Resources.ReportGPU = true
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.collector == nil {
		t.Fatal("collector is nil")
	}
}

func TestNew_WithReportNetwork(t *testing.T) {
	cfg := config.Default()
	cfg.Containerd.Socket = "nonexistent"
	cfg.Resources.ReportNetwork = true
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.collector == nil {
		t.Fatal("collector is nil")
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		fallback time.Duration
		expected time.Duration
	}{
		{"5s", time.Second, 5 * time.Second},
		{"10m", time.Second, 10 * time.Minute},
		{"1h", time.Second, time.Hour},
		{"", time.Second, time.Second},
		{"invalid", 42 * time.Second, 42 * time.Second},
		{"0", 99 * time.Second, 0},
		{"-5s", time.Second, -5 * time.Second},
	}

	for _, tt := range tests {
		got := parseDuration(tt.input, tt.fallback)
		if got != tt.expected {
			t.Errorf("parseDuration(%q, %v) = %v, want %v", tt.input, tt.fallback, got, tt.expected)
		}
	}
}