package executor

import (
	"sync"
	"testing"
)

func TestManager_AddGetRemove(t *testing.T) {
	m := NewManager()

	// Add
	m.Add("unit-1", "container-1")
	m.Add("unit-2", "container-2")

	// Get
	u := m.Get("unit-1")
	if u == nil {
		t.Fatal("expected unit-1 to exist")
	}
	if u.UnitID != "unit-1" || u.ContainerID != "container-1" {
		t.Errorf("got UnitID=%s ContainerID=%s", u.UnitID, u.ContainerID)
	}

	// Get non-existent
	if got := m.Get("unit-none"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}

	// Remove
	m.Remove("unit-1")
	if m.Get("unit-1") != nil {
		t.Error("expected unit-1 to be removed")
	}
	if m.Len() != 1 {
		t.Errorf("expected Len=1, got %d", m.Len())
	}
}

func TestManager_List(t *testing.T) {
	m := NewManager()
	m.Add("unit-a", "c-a")
	m.Add("unit-b", "c-b")

	ids := m.List()
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}

	got := make(map[string]bool)
	for _, id := range ids {
		got[id] = true
	}
	if !got["unit-a"] || !got["unit-b"] {
		t.Errorf("List missing expected units: %v", ids)
	}
}

func TestManager_EmptyList(t *testing.T) {
	m := NewManager()
	ids := m.List()
	if len(ids) != 0 {
		t.Errorf("expected empty list, got %v", ids)
	}
}

func TestManager_Len(t *testing.T) {
	m := NewManager()
	if m.Len() != 0 {
		t.Errorf("expected 0, got %d", m.Len())
	}
	m.Add("u1", "c1")
	if m.Len() != 1 {
		t.Errorf("expected 1, got %d", m.Len())
	}
	m.Remove("u1")
	if m.Len() != 0 {
		t.Errorf("expected 0, got %d", m.Len())
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			unitID := "unit-" + string(rune('a'+n%26))
			m.Add(unitID, "c-"+unitID)
			m.Get(unitID)
			m.List()
			m.Remove(unitID)
		}(i)
	}
	wg.Wait()
	// Should not panic or race (detected by -race)
}