package agent

import (
	"testing"
	"time"

	cpstartcfg "computing-power/agent/internal/cpstart/config"
)

func TestNewRunner(t *testing.T) {
	cfg := cpstartcfg.Default()
	r := NewRunner(cfg)
	if r == nil {
		t.Fatal("expected non-nil runner")
	}
	if r.Status() != AgentStopped {
		t.Errorf("expected Stopped, got %v", r.Status())
	}
}

func TestRunnerStatusString(t *testing.T) {
	tests := []struct {
		s    AgentStatus
		want string
	}{
		{AgentStopped, "stopped"},
		{AgentStarting, "starting"},
		{AgentRunning, "running"},
		{AgentError, "error"},
		{AgentStatus(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("AgentStatus(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestRunnerStartStop(t *testing.T) {
	cfg := cpstartcfg.Default()
	cfg.Scheduler.Address = "localhost:19090" // 不存在的端口，预期连接失败

	r := NewRunner(cfg)
	if err := r.Start(); err != nil {
		// 可能失败（连接被拒绝），也可能成功（后台 goroutine 尝试连接）
		// 两种都算正常
		t.Logf("Start returned: %v", err)
	}

	// 等待一小段时间，让状态更新
	time.Sleep(100 * time.Millisecond)

	// 停止
	r.Stop()

	// 等待退出
	done := make(chan struct{})
	go func() {
		r.Wait()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Wait timed out")
	}
}

func TestRunnerDoubleStart(t *testing.T) {
	cfg := cpstartcfg.Default()
	cfg.Scheduler.Address = "localhost:19091"

	r := NewRunner(cfg)
	if err := r.Start(); err != nil {
		t.Logf("first Start: %v", err)
	}
	// 第二次 Start 应该返回 nil（已是 running/starting）
	if err := r.Start(); err != nil {
		t.Errorf("second Start should return nil, got %v", err)
	}
	r.Stop()
}

func TestRunnerLocalResources(t *testing.T) {
	cfg := cpstartcfg.Default()
	r := NewRunner(cfg)
	res := r.LocalResources()
	if res == nil {
		t.Fatal("expected non-nil resources")
	}
	if res.CPUCores <= 0 {
		t.Errorf("expected positive CPU cores, got %f", res.CPUCores)
	}
}

func TestRunnerNodeID(t *testing.T) {
	cfg := cpstartcfg.Default()
	r := NewRunner(cfg)
	if r.NodeID() != "" {
		t.Errorf("expected empty NodeID before start, got %s", r.NodeID())
	}
}

func TestRunnerLastError(t *testing.T) {
	cfg := cpstartcfg.Default()
	r := NewRunner(cfg)
	if r.LastError() != nil {
		t.Errorf("expected nil error, got %v", r.LastError())
	}
}