package ipam

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"computing-power/scheduler/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open test db: %v", err)
	}
	t.Cleanup(func() {
		st.Close()
		os.Remove(path)
	})
	return st
}

func TestNewIPAM(t *testing.T) {
	st := newTestStore(t)
	ipam, err := NewIPAM(st, "10.1.0.0/16", "10.1.0.1")
	if err != nil {
		t.Fatalf("NewIPAM: %v", err)
	}
	if ipam.Gateway() != "10.1.0.1" {
		t.Errorf("expected gateway 10.1.0.1, got %s", ipam.Gateway())
	}
	if ipam.Network() != "10.1.0.0/16" {
		t.Errorf("expected network 10.1.0.0/16, got %s", ipam.Network())
	}
}

func TestNewIPAM_InvalidCIDR(t *testing.T) {
	st := newTestStore(t)
	_, err := NewIPAM(st, "invalid", "10.1.0.1")
	if err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}

func TestNewIPAM_GatewayOutsideNetwork(t *testing.T) {
	st := newTestStore(t)
	_, err := NewIPAM(st, "10.1.0.0/16", "192.168.1.1")
	if err == nil {
		t.Fatal("expected error for gateway outside network")
	}
}

func TestAllocate(t *testing.T) {
	st := newTestStore(t)
	ipam, err := NewIPAM(st, "10.1.0.0/16", "10.1.0.1")
	if err != nil {
		t.Fatalf("NewIPAM: %v", err)
	}

	ip1, err := ipam.Allocate("node-1")
	if err != nil {
		t.Fatalf("Allocate node-1: %v", err)
	}
	if ip1 != "10.1.0.2" {
		t.Errorf("expected 10.1.0.2, got %s", ip1)
	}

	ip2, err := ipam.Allocate("node-2")
	if err != nil {
		t.Fatalf("Allocate node-2: %v", err)
	}
	if ip2 != "10.1.0.3" {
		t.Errorf("expected 10.1.0.3, got %s", ip2)
	}
}

func TestAllocate_ReusesExisting(t *testing.T) {
	st := newTestStore(t)
	ipam, err := NewIPAM(st, "10.1.0.0/16", "10.1.0.1")
	if err != nil {
		t.Fatalf("NewIPAM: %v", err)
	}

	ip1, err := ipam.Allocate("node-1")
	if err != nil {
		t.Fatalf("First allocate: %v", err)
	}

	// 第二次分配同一节点应返回相同 IP
	ip2, err := ipam.Allocate("node-1")
	if err != nil {
		t.Fatalf("Second allocate: %v", err)
	}
	if ip1 != ip2 {
		t.Errorf("expected same IP %s, got %s", ip1, ip2)
	}
}

func TestRelease(t *testing.T) {
	st := newTestStore(t)
	ipam, err := NewIPAM(st, "10.1.0.0/16", "10.1.0.1")
	if err != nil {
		t.Fatalf("NewIPAM: %v", err)
	}

	ip, err := ipam.Allocate("node-1")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if ip == "" {
		t.Fatal("expected non-empty IP")
	}

	if err := ipam.Release("node-1"); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestAllocate_Sequential(t *testing.T) {
	st := newTestStore(t)
	ipam, err := NewIPAM(st, "10.1.0.0/16", "10.1.0.1")
	if err != nil {
		t.Fatalf("NewIPAM: %v", err)
	}

	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		ip, err := ipam.Allocate(nodeID)
		if err != nil {
			t.Fatalf("Allocate %s: %v", nodeID, err)
		}
		if seen[ip] {
			t.Fatalf("duplicate IP %s allocated to %s", ip, nodeID)
		}
		seen[ip] = true
	}
}

func TestGatewayPreserved(t *testing.T) {
	st := newTestStore(t)
	ipam, err := NewIPAM(st, "10.1.0.0/16", "10.1.0.1")
	if err != nil {
		t.Fatalf("NewIPAM: %v", err)
	}

	ip, err := ipam.Allocate("node-1")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if ip == "10.1.0.1" {
		t.Fatal("gateway IP should not be allocated to nodes")
	}
}

func TestRestoreAllocations(t *testing.T) {
	st := newTestStore(t)
	ipam, err := NewIPAM(st, "10.1.0.0/16", "10.1.0.1")
	if err != nil {
		t.Fatalf("NewIPAM: %v", err)
	}

	ip1, err := ipam.Allocate("node-1")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	// 创建新的 IPAM 实例（使用同一 store）
	ipam2, err := NewIPAM(st, "10.1.0.0/16", "10.1.0.1")
	if err != nil {
		t.Fatalf("NewIPAM2: %v", err)
	}

	ip2, err := ipam2.Allocate("node-1")
	if err != nil {
		t.Fatalf("Re-allocate: %v", err)
	}
	if ip1 != ip2 {
		t.Errorf("expected restored IP %s, got %s", ip1, ip2)
	}
}