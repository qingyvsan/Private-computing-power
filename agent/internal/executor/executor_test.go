package executor

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	pb "computing-power/proto/v1"
	"computing-power/agent/internal/container"
)

// mockRuntime 模拟容器运行时
type mockRuntime struct {
	available    bool
	pullCalled   atomic.Int32
	createCalled atomic.Int32
	startCalled  atomic.Int32
	stopCalled   atomic.Int32
	removeCalled atomic.Int32
	statusCalled atomic.Int32

	pullErr   error
	createErr error
	startErr  error
	stopErr   error
	removeErr error
}

func (m *mockRuntime) IsAvailable() bool { return m.available }

func (m *mockRuntime) PullImage(ctx context.Context, image string) error {
	m.pullCalled.Add(1)
	return m.pullErr
}

func (m *mockRuntime) CreateContainer(ctx context.Context, spec *container.ContainerSpec) (string, error) {
	m.createCalled.Add(1)
	if m.createErr != nil {
		return "", m.createErr
	}
	return "container-" + spec.ID, nil
}

func (m *mockRuntime) StartContainer(ctx context.Context, id string) error {
	m.startCalled.Add(1)
	return m.startErr
}

func (m *mockRuntime) StopContainer(ctx context.Context, id string) error {
	m.stopCalled.Add(1)
	return m.stopErr
}

func (m *mockRuntime) KillContainer(ctx context.Context, id string) error { return nil }

func (m *mockRuntime) RemoveContainer(ctx context.Context, id string) error {
	m.removeCalled.Add(1)
	return m.removeErr
}

func (m *mockRuntime) GetStatus(ctx context.Context, id string) (*container.ContainerStatus, error) {
	m.statusCalled.Add(1)
	return &container.ContainerStatus{ID: id, Running: false, ExitCode: 0}, nil
}

var errTestPullFailed = errors.New("pull failed")
var errTestCreateFailed = errors.New("create failed")
var errTestStartFailed = errors.New("start failed")

func TestExecutor_HandleCommand_UnknownType(t *testing.T) {
	mgr := NewManager()
	rep := NewReporter("node-1", nil)
	rt := &mockRuntime{available: true}
	ex := NewExecutor(rt, mgr, rep, nil)

	// 未知命令类型不应 panic
	ex.HandleCommand(&pb.Command{Type: "unknown", Payload: []byte("{}")})
}

func TestExecutor_HandleAssign_ValidPayload(t *testing.T) {
	mgr := NewManager()
	rep := NewReporter("node-1", nil)
	rt := &mockRuntime{available: true}
	ex := NewExecutor(rt, mgr, rep, nil)

	payload := `{"unit_id":"u1","stage_id":"s1","job_id":"j1","image":"alpine:latest","input":"","index":0}`
	ex.HandleCommand(&pb.Command{Type: "assign", Payload: []byte(payload)})

	// 等待异步执行：pull + create + start + 快速退出
	waitForPullCalled(t, rt, 1)
	waitForCreateCalled(t, rt, 1)
	waitForStartCalled(t, rt, 1)
}

func TestExecutor_HandleAssign_RuntimeUnavailable(t *testing.T) {
	mgr := NewManager()
	rep := NewReporter("node-1", nil)
	rt := &mockRuntime{available: false}
	ex := NewExecutor(rt, mgr, rep, nil)

	payload := `{"unit_id":"u2","stage_id":"s1","job_id":"j1","image":"alpine:latest"}`
	ex.HandleCommand(&pb.Command{Type: "assign", Payload: []byte(payload)})

	// 运行时不可用，不应调用 pull
	if rt.pullCalled.Load() != 0 {
		t.Error("PullImage should not be called when runtime unavailable")
	}
}

func TestExecutor_HandleAssign_PullFailure(t *testing.T) {
	mgr := NewManager()
	rep := NewReporter("node-1", nil)
	rt := &mockRuntime{available: true, pullErr: errTestPullFailed}
	ex := NewExecutor(rt, mgr, rep, nil)

	payload := `{"unit_id":"u3","image":"nonexistent:latest"}`
	ex.HandleCommand(&pb.Command{Type: "assign", Payload: []byte(payload)})

	waitForPullCalled(t, rt, 1)
	if rt.createCalled.Load() != 0 {
		t.Error("CreateContainer should not be called after pull failure")
	}
}

func TestExecutor_HandleAssign_CreateFailure(t *testing.T) {
	mgr := NewManager()
	rep := NewReporter("node-1", nil)
	rt := &mockRuntime{available: true, createErr: errTestCreateFailed}
	ex := NewExecutor(rt, mgr, rep, nil)

	payload := `{"unit_id":"u4","image":"alpine:latest"}`
	ex.HandleCommand(&pb.Command{Type: "assign", Payload: []byte(payload)})

	waitForPullCalled(t, rt, 1)
	waitForCreateCalled(t, rt, 1)
	if rt.startCalled.Load() != 0 {
		t.Error("StartContainer should not be called after create failure")
	}
}

func TestExecutor_HandleAssign_StartFailure(t *testing.T) {
	mgr := NewManager()
	rep := NewReporter("node-1", nil)
	rt := &mockRuntime{available: true, startErr: errTestStartFailed}
	ex := NewExecutor(rt, mgr, rep, nil)

	payload := `{"unit_id":"u5","image":"alpine:latest"}`
	ex.HandleCommand(&pb.Command{Type: "assign", Payload: []byte(payload)})

	waitForPullCalled(t, rt, 1)
	waitForCreateCalled(t, rt, 1)
	waitForStartCalled(t, rt, 1)
	waitForManagerLen(t, mgr, 0)
}

func TestExecutor_HandleCommand_NilCommand(t *testing.T) {
	mgr := NewManager()
	rep := NewReporter("node-1", nil)
	rt := &mockRuntime{available: true}
	ex := NewExecutor(rt, mgr, rep, nil)

	// nil 命令不应 panic
	ex.HandleCommand(nil)
}

func TestExecutor_HandleAssign_GPURequest_NoHAMI(t *testing.T) {
	// GPU 请求但 HAMi 未启用（hamiMgr = nil），应继续执行但不设置 GPU 环境
	mgr := NewManager()
	rep := NewReporter("node-1", nil)
	rt := &mockRuntime{available: true}
	ex := NewExecutor(rt, mgr, rep, nil)

	payload := `{"unit_id":"u6","image":"alpine:latest","gpu_request":{"memory_mb":4096,"cores":50,"count":1}}`
	ex.HandleCommand(&pb.Command{Type: "assign", Payload: []byte(payload)})

	// 应正常执行容器（忽略 GPU 请求）
	waitForPullCalled(t, rt, 1)
	waitForCreateCalled(t, rt, 1)
	waitForStartCalled(t, rt, 1)
}

// 帮助函数：等待条件满足（带超时）

func waitForPullCalled(t *testing.T, rt *mockRuntime, expected int32) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if rt.pullCalled.Load() >= expected {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Errorf("expected pullCalled >= %d, got %d", expected, rt.pullCalled.Load())
}

func waitForCreateCalled(t *testing.T, rt *mockRuntime, expected int32) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if rt.createCalled.Load() >= expected {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Errorf("expected createCalled >= %d, got %d", expected, rt.createCalled.Load())
}

func waitForStartCalled(t *testing.T, rt *mockRuntime, expected int32) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if rt.startCalled.Load() >= expected {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Errorf("expected startCalled >= %d, got %d", expected, rt.startCalled.Load())
}

func waitForManagerLen(t *testing.T, mgr *Manager, expected int) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if mgr.Len() == expected {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Errorf("expected manager len=%d, got %d", expected, mgr.Len())
}