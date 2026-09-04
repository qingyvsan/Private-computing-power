package heartbeat

import (
	"testing"
	"time"
)

func TestNewCollector(t *testing.T) {
	c := NewCollector(false, false, nil, "")
	if c == nil {
		t.Fatal("NewCollector returned nil")
	}
	if c.reportGPU {
		t.Error("expected reportGPU=false")
	}
	if c.reportNetwork {
		t.Error("expected reportNetwork=false")
	}
}

func TestCollect_ReturnsResources(t *testing.T) {
	c := NewCollector(false, false, nil, "")
	res := c.Collect()
	if res == nil {
		t.Fatal("Collect returned nil")
	}
	if res.CPUCores <= 0 {
		t.Errorf("expected CPU cores > 0, got %f", res.CPUCores)
	}
	if res.MemoryBytes <= 0 {
		t.Errorf("expected MemoryBytes > 0, got %d", res.MemoryBytes)
	}
	if res.DiskBytes <= 0 {
		t.Errorf("expected DiskBytes > 0, got %d", res.DiskBytes)
	}
}

func TestCollect_GPUDisabled(t *testing.T) {
	c := NewCollector(false, false, nil, "")
	res := c.Collect()
	if res == nil {
		t.Fatal("Collect returned nil")
	}
	if len(res.GPUs) != 0 {
		t.Errorf("expected 0 GPUs when reportGPU=false, got %d", len(res.GPUs))
	}
}

func TestCollect_NetworkDisabled(t *testing.T) {
	c := NewCollector(false, false, nil, "")
	res := c.Collect()
	if res.Network != nil {
		t.Error("expected nil Network when reportNetwork=false")
	}
}

func TestNewReporter(t *testing.T) {
	collector := NewCollector(false, false, nil, "")
	r := NewReporter("node-1", collector, nil, time.Second, 0, nil, nil)
	if r == nil {
		t.Fatal("NewReporter returned nil")
	}
	if r.nodeID != "node-1" {
		t.Errorf("expected nodeID=node-1, got %s", r.nodeID)
	}
	if r.interval != time.Second {
		t.Errorf("expected interval=1s, got %v", r.interval)
	}
}

func TestProbeRTT_EmptyAddr(t *testing.T) {
	rtt := probeRTT("")
	if rtt != 0 {
		t.Errorf("expected 0 for empty addr, got %f", rtt)
	}
}

func TestNewCollector_WithGPU(t *testing.T) {
	c := NewCollector(true, false, nil, "")
	if c == nil {
		t.Fatal("NewCollector returned nil")
	}
	if !c.reportGPU {
		t.Error("expected reportGPU=true")
	}
}

func TestCollect_Race(t *testing.T) {
	c := NewCollector(false, false, nil, "")
	done := make(chan struct{})
	go func() {
		c.Collect()
		close(done)
	}()
	c.Collect()
	<-done
}