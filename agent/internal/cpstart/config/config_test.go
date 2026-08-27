package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.Console.Port != 8080 {
		t.Errorf("expected port 8080, got %d", c.Console.Port)
	}
	if c.Scheduler.Address != "localhost:9090" {
		t.Errorf("expected localhost:9090, got %s", c.Scheduler.Address)
	}
	if c.Agent.Name == "" {
		t.Error("expected non-empty agent name")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	c := Default()
	c.Agent.Name = "test-node"
	c.Scheduler.Address = "10.0.0.1:9090"
	c.Console.Port = 9090

	path := filepath.Join(dir, "cpstart.yaml")
	if err := c.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Agent.Name != "test-node" {
		t.Errorf("expected test-node, got %s", loaded.Agent.Name)
	}
	if loaded.Scheduler.Address != "10.0.0.1:9090" {
		t.Errorf("expected 10.0.0.1:9090, got %s", loaded.Scheduler.Address)
	}
	if loaded.Console.Port != 9090 {
		t.Errorf("expected port 9090, got %d", loaded.Console.Port)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/cpstart.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestConfigPath(t *testing.T) {
	c := Default()
	c.Agent.DataDir = "/tmp/cpstart"
	expected := filepath.Join("/tmp/cpstart", "cpstart.yaml")
	if c.ConfigPath() != expected {
		t.Errorf("expected %s, got %s", expected, c.ConfigPath())
	}
}

func TestToAgentConfig(t *testing.T) {
	c := Default()
	c.Agent.Name = "test-agent"
	c.Agent.DataDir = "/data/cpstart"
	c.Scheduler.Address = "192.168.1.1:9090"
	c.Resources.ReportGPU = false

	ac := c.ToAgentConfig()
	if ac.Agent.Name != "test-agent" {
		t.Errorf("expected test-agent, got %s", ac.Agent.Name)
	}
	if ac.Scheduler.Address != "192.168.1.1:9090" {
		t.Errorf("expected 192.168.1.1:9090, got %s", ac.Scheduler.Address)
	}
	if ac.Resources.ReportGPU != false {
		t.Errorf("expected ReportGPU=false, got %v", ac.Resources.ReportGPU)
	}
}

func TestSave_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	c := Default()
	path := filepath.Join(dir, "cpstart.yaml")
	if err := c.Save(path); err != nil {
		t.Fatalf("Save with nested dir: %v", err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}
}