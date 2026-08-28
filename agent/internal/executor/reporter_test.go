package executor

import (
	"context"
	"testing"
	"time"

	pb "computing-power/proto/v1"
	"google.golang.org/grpc"
)

// mockReportStream 模拟 gRPC ReportUnitStatus 流
type mockReportStream struct {
	grpc.ClientStream
	sendCh chan *pb.UnitStatusReport
	ackCh  chan *pb.UnitStatusAck
	done   chan struct{}
}

func newMockReportStream() *mockReportStream {
	return &mockReportStream{
		sendCh: make(chan *pb.UnitStatusReport, 10),
		ackCh:  make(chan *pb.UnitStatusAck, 10),
		done:   make(chan struct{}),
	}
}

func (m *mockReportStream) Send(report *pb.UnitStatusReport) error {
	m.sendCh <- report
	return nil
}

func (m *mockReportStream) Recv() (*pb.UnitStatusAck, error) {
	<-m.done
	return nil, nil
}

func (m *mockReportStream) CloseSend() error {
	close(m.done)
	return nil
}

// mockReportClient 模拟 gRPC 客户端
type mockReportClient struct {
	pb.SchedulerServiceClient
}

func (m *mockReportClient) newStream() *mockReportStream {
	return newMockReportStream()
}

func TestReporter_StartAndReport(t *testing.T) {
	// 使用真实 client 但注入 mock stream
	r := NewReporter("node-1", nil)
	stream := newMockReportStream()
	r.mu.Lock()
	r.stream = stream
	r.mu.Unlock()

	// 发送报告
	err := r.Report("unit-1", pb.UnitStatusRunning, 0, "", nil)
	if err != nil {
		t.Fatalf("Report failed: %v", err)
	}

	// 验证发送
	select {
	case report := <-stream.sendCh:
		if report.UnitID != "unit-1" {
			t.Errorf("expected unit-1, got %s", report.UnitID)
		}
		if report.Status != pb.UnitStatusRunning {
			t.Errorf("expected Running, got %v", report.Status)
		}
		if report.NodeID != "node-1" {
			t.Errorf("expected node-1, got %s", report.NodeID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for report")
	}
}

func TestReporter_ReportAfterStop(t *testing.T) {
	r := NewReporter("node-1", nil)
	stream := newMockReportStream()
	r.mu.Lock()
	r.stream = stream
	r.mu.Unlock()

	r.Stop()

	// 停止后发送不应 panic
	err := r.Report("unit-1", pb.UnitStatusCompleted, 0, "", nil)
	if err != nil {
		t.Fatalf("Report after stop should not error: %v", err)
	}
}

func TestReporter_MultipleReports(t *testing.T) {
	r := NewReporter("node-1", nil)
	stream := newMockReportStream()
	r.mu.Lock()
	r.stream = stream
	r.mu.Unlock()

	reports := []struct {
		unitID string
		status pb.UnitStatus
	}{
		{"u1", pb.UnitStatusRunning},
		{"u1", pb.UnitStatusCompleted},
		{"u2", pb.UnitStatusRunning},
		{"u2", pb.UnitStatusFailed},
	}

	for _, rr := range reports {
		err := r.Report(rr.unitID, rr.status, 0, "", nil)
		if err != nil {
			t.Fatalf("Report(%s) failed: %v", rr.unitID, err)
		}
	}

	close(stream.sendCh)
	count := 0
	for range stream.sendCh {
		count++
	}
	if count != len(reports) {
		t.Errorf("expected %d reports, got %d", len(reports), count)
	}
}

func TestReporter_Start_ContextCancel(t *testing.T) {
	// 测试 Start 在 context 取消时正常退出
	ctx, cancel := context.WithCancel(context.Background())
	r := NewReporter("node-1", nil)

	done := make(chan error, 1)
	go func() {
		done <- r.Start(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil error on cancel, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Start to return")
	}
}