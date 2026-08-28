package nebula

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	return &Config{
		Enabled:    true,
		BinaryPath: "nebula", // may not exist in test env
		DataDir:    filepath.Join(dir, "data"),
		ConfigPath: filepath.Join(dir, "data", "config.yaml"),
		CertDir:    filepath.Join(dir, "data", "certs"),
	}
}

func TestNewManager(t *testing.T) {
	cfg := newTestConfig(t)
	m := NewManager(cfg)
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.IsRunning() {
		t.Error("expected not running initially")
	}
	if m.GetNATType() != "unknown" {
		t.Errorf("expected NAT type 'unknown', got %s", m.GetNATType())
	}
}

func TestConfigure(t *testing.T) {
	cfg := newTestConfig(t)
	m := NewManager(cfg)

	certPEM := []byte("-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----")
	keyPEM := []byte("-----BEGIN EC PRIVATE KEY-----\nMIIB...\n-----END EC PRIVATE KEY-----")
	caPEM := []byte("-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----")
	configYAML := "pki:\n  ca: ca.crt\n"

	err := m.Configure(certPEM, keyPEM, caPEM, "10.1.0.2", "8.138.108.183:4242", configYAML)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Verify files were written
	if !m.IsConfigured() {
		t.Error("expected IsConfigured() after Configure()")
	}

	// Check specific files
	for _, f := range []string{"ca.crt", "node.crt", "node.key"} {
		path := filepath.Join(cfg.CertDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", path)
		}
	}

	// Check config file
	if _, err := os.Stat(cfg.ConfigPath); os.IsNotExist(err) {
		t.Error("expected config file to exist")
	}

	if m.GetOverlayIP() != "10.1.0.2" {
		t.Errorf("expected overlay IP 10.1.0.2, got %s", m.GetOverlayIP())
	}
}

func TestConfigure_NotEnabled(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Enabled = false
	m := NewManager(cfg)

	err := m.Configure([]byte("cert"), []byte("key"), []byte("ca"), "10.1.0.2", "lighthouse:4242", "config")
	if err == nil {
		t.Fatal("expected error when nebula is not enabled")
	}
}

func TestConfigure_Idempotent(t *testing.T) {
	cfg := newTestConfig(t)
	m := NewManager(cfg)

	// Configure twice
	err := m.Configure([]byte("cert1"), []byte("key1"), []byte("ca1"), "10.1.0.2", "lh:4242", "config1")
	if err != nil {
		t.Fatalf("First configure: %v", err)
	}

	err = m.Configure([]byte("cert2"), []byte("key2"), []byte("ca2"), "10.1.0.2", "lh:4242", "config2")
	if err != nil {
		t.Fatalf("Second configure: %v", err)
	}

	// Should overwrite files
	if !m.IsConfigured() {
		t.Error("expected configured after second call")
	}
}

func TestStart_NotConfigured(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Enabled = false
	m := NewManager(cfg)

	err := m.Start()
	if err == nil {
		t.Fatal("expected error when not enabled")
	}
}

func TestStart_BinaryNotFound(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.BinaryPath = "nonexistent-nebula-binary"
	m := NewManager(cfg)

	// Configure first
	m.Configure([]byte("cert"), []byte("key"), []byte("ca"), "10.1.0.2", "lh:4242", "config: test")

	err := m.Start()
	if err == nil {
		t.Fatal("expected error when binary not found")
	}
}

func TestStop_NotRunning(t *testing.T) {
	cfg := newTestConfig(t)
	m := NewManager(cfg)

	err := m.Stop()
	if err != nil {
		t.Fatalf("Stop on non-running manager: %v", err)
	}
}

func TestSetNATType(t *testing.T) {
	cfg := newTestConfig(t)
	m := NewManager(cfg)

	m.SetNATType("full_cone")
	if m.GetNATType() != "full_cone" {
		t.Errorf("expected NAT type 'full_cone', got %s", m.GetNATType())
	}
}

func TestUptime_NotRunning(t *testing.T) {
	cfg := newTestConfig(t)
	m := NewManager(cfg)

	if m.Uptime() != 0 {
		t.Errorf("expected 0 uptime when not running, got %s", m.Uptime())
	}
}

func TestIsConfigured_Initially(t *testing.T) {
	cfg := newTestConfig(t)
	m := NewManager(cfg)

	if m.IsConfigured() {
		t.Error("expected not configured initially")
	}
}

func TestConfigPath(t *testing.T) {
	cfg := newTestConfig(t)
	m := NewManager(cfg)

	if m.ConfigPath() != cfg.ConfigPath {
		t.Errorf("expected config path %s, got %s", cfg.ConfigPath, m.ConfigPath())
	}
}

func TestCertDir(t *testing.T) {
	cfg := newTestConfig(t)
	m := NewManager(cfg)

	if m.CertDir() != cfg.CertDir {
		t.Errorf("expected cert dir %s, got %s", cfg.CertDir, m.CertDir())
	}
}