package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "computing-power/proto/v1"
	"computing-power/pkg/trustgraph"

	"computing-power/scheduler/internal/config"
	"computing-power/scheduler/internal/ipam"
	"computing-power/scheduler/internal/registry"
	"computing-power/scheduler/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return newTestServerWithConfig(t, config.Default())
}

func newTestServerWithConfig(t *testing.T, cfg *config.Config) *Server {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		st.Close()
		os.Remove(path)
	})
	reg := registry.NewRegistry(1000, 100, 4.0)
	ipamMgr, err := ipam.NewIPAM(st, "10.1.0.0/16", "10.1.0.1")
	if err != nil {
		t.Fatalf("ipam: %v", err)
	}
	srv := New(st, reg, trustgraph.NewGraph(), 3*time.Second, 30*time.Second, cfg, nil, ipamMgr, nil)
	return srv
}

func TestSubmitJob_Single(t *testing.T) {
	srv := newTestServer(t)
	job := &pb.Job{
		Name:    "test-single",
		Type:    pb.JobTypeSingle,
		OwnerID: "node-1",
		Image:   "alpine:latest",
		Stages: []*pb.Stage{
			{
				Name: "main",
				Resources: &pb.ResourceSpec{
					CPUCores:    1,
					MemoryBytes: 536870912, // 512MB
				},
			},
		},
	}

	resp, err := srv.SubmitJob(context.Background(), &pb.SubmitJobRequest{Job: job})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}
	if resp.JobID == "" {
		t.Fatal("expected non-empty job ID")
	}
	if resp.Status != pb.JobStatusPending {
		t.Errorf("expected Pending status, got %v", resp.Status)
	}

	// Verify job was stored
	got, err := srv.store.GetJob(resp.JobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got == nil {
		t.Fatal("job not found in store")
	}
	if got.Name != "test-single" {
		t.Errorf("expected Name test-single, got %s", got.Name)
	}
}

func TestSubmitJob_WithSplit_ByN(t *testing.T) {
	srv := newTestServer(t)
	job := &pb.Job{
		Name:    "test-split",
		Type:    pb.JobTypeAggregate,
		OwnerID: "node-1",
		Stages: []*pb.Stage{
			{
				Name: "parallel",
				Split: &pb.SplitStrategy{
					Type: pb.SplitTypeByN,
					ByN:  &pb.ByNSplit{NumParts: 5},
				},
			},
		},
	}

	resp, err := srv.SubmitJob(context.Background(), &pb.SubmitJobRequest{Job: job})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}

	// Verify units were created
	units, err := srv.store.ListUnitsByJob(resp.JobID)
	if err != nil {
		t.Fatalf("ListUnitsByJob: %v", err)
	}
	if len(units) != 5 {
		t.Fatalf("expected 5 units, got %d", len(units))
	}
	for i, u := range units {
		if u.Status != pb.UnitStatusPending {
			t.Errorf("unit %d: expected Pending, got %v", i, u.Status)
		}
		if u.JobID != resp.JobID {
			t.Errorf("unit %d: wrong JobID", i)
		}
	}
}

func TestSubmitJob_WithSplit_ByFile(t *testing.T) {
	srv := newTestServer(t)
	job := &pb.Job{
		Name:    "test-file-split",
		Type:    pb.JobTypeAggregate,
		OwnerID: "node-1",
		Stages: []*pb.Stage{
			{
				Name: "process-files",
				Split: &pb.SplitStrategy{
					Type: pb.SplitTypeByFile,
					ByFile: &pb.ByFileSplit{
						FileList: []string{"a.txt", "b.txt", "c.txt"},
					},
				},
			},
		},
	}

	resp, err := srv.SubmitJob(context.Background(), &pb.SubmitJobRequest{Job: job})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}

	units, err := srv.store.ListUnitsByJob(resp.JobID)
	if err != nil {
		t.Fatalf("ListUnitsByJob: %v", err)
	}
	if len(units) != 3 {
		t.Fatalf("expected 3 units, got %d", len(units))
	}
}

func TestSubmitJob_WithSplit_ByRange(t *testing.T) {
	srv := newTestServer(t)
	job := &pb.Job{
		Name:    "test-range-split",
		Type:    pb.JobTypeAggregate,
		OwnerID: "node-1",
		Stages: []*pb.Stage{
			{
				Name: "process-range",
				Split: &pb.SplitStrategy{
					Type: pb.SplitTypeByRange,
					ByRange: &pb.ByRangeSplit{
						Start:    0,
						End:      1000,
						NumParts: 4,
					},
				},
			},
		},
	}

	resp, err := srv.SubmitJob(context.Background(), &pb.SubmitJobRequest{Job: job})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}

	units, err := srv.store.ListUnitsByJob(resp.JobID)
	if err != nil {
		t.Fatalf("ListUnitsByJob: %v", err)
	}
	if len(units) != 4 {
		t.Fatalf("expected 4 units, got %d", len(units))
	}
}

func TestSubmitJob_MultipleStages(t *testing.T) {
	srv := newTestServer(t)
	job := &pb.Job{
		Name:    "test-workflow",
		Type:    pb.JobTypeWorkflow,
		OwnerID: "node-1",
		Stages: []*pb.Stage{
			{
				Name: "download",
				Split: &pb.SplitStrategy{
					Type: pb.SplitTypeByN,
					ByN:  &pb.ByNSplit{NumParts: 1},
				},
			},
			{
				Name: "process",
				DependsOn: []string{"download"},
				Split: &pb.SplitStrategy{
					Type: pb.SplitTypeByN,
					ByN:  &pb.ByNSplit{NumParts: 3},
				},
			},
		},
	}

	resp, err := srv.SubmitJob(context.Background(), &pb.SubmitJobRequest{Job: job})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}

	units, err := srv.store.ListUnitsByJob(resp.JobID)
	if err != nil {
		t.Fatalf("ListUnitsByJob: %v", err)
	}
	if len(units) != 4 {
		t.Fatalf("expected 4 units (1+3), got %d", len(units))
	}
}

func TestGetJob(t *testing.T) {
	srv := newTestServer(t)
	// Submit a job first
	job := &pb.Job{
		Name:    "test-get",
		Type:    pb.JobTypeSingle,
		OwnerID: "node-1",
		Stages: []*pb.Stage{
			{Name: "main"},
		},
	}
	submitResp, err := srv.SubmitJob(context.Background(), &pb.SubmitJobRequest{Job: job})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}

	// Get the job
	resp, err := srv.GetJob(context.Background(), &pb.GetJobRequest{JobID: submitResp.JobID})
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if resp.Job == nil {
		t.Fatal("expected non-nil job")
	}
	if resp.Job.Name != "test-get" {
		t.Errorf("expected Name test-get, got %s", resp.Job.Name)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	srv := newTestServer(t)
	_, err := srv.GetJob(context.Background(), &pb.GetJobRequest{JobID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestListJobs(t *testing.T) {
	srv := newTestServer(t)
	// Submit two jobs with explicit IDs to avoid flaky ID collision
	for _, name := range []string{"job-a", "job-b"} {
		job := &pb.Job{
			ID:      fmt.Sprintf("list-job-%s", name),
			Name:    name,
			Type:    pb.JobTypeSingle,
			OwnerID: "node-1",
		}
		if _, err := srv.SubmitJob(context.Background(), &pb.SubmitJobRequest{Job: job}); err != nil {
			t.Fatalf("SubmitJob %s: %v", name, err)
		}
	}

	resp, err := srv.ListJobs(context.Background(), &pb.ListJobsRequest{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if resp.TotalCount != 2 {
		t.Errorf("expected 2 jobs, got %d", resp.TotalCount)
	}
	if len(resp.Jobs) != 2 {
		t.Errorf("expected 2 jobs in list, got %d", len(resp.Jobs))
	}
}

func TestCancelJob(t *testing.T) {
	srv := newTestServer(t)
	job := &pb.Job{
		Name:    "test-cancel",
		Type:    pb.JobTypeSingle,
		OwnerID: "node-1",
		Stages: []*pb.Stage{
			{
				Name: "main",
				Split: &pb.SplitStrategy{
					Type: pb.SplitTypeByN,
					ByN:  &pb.ByNSplit{NumParts: 3},
				},
			},
		},
	}
	submitResp, err := srv.SubmitJob(context.Background(), &pb.SubmitJobRequest{Job: job})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}

	// Cancel
	cancelResp, err := srv.CancelJob(context.Background(), &pb.CancelJobRequest{
		JobID:  submitResp.JobID,
		NodeID: "node-1",
	})
	if err != nil {
		t.Fatalf("CancelJob: %v", err)
	}
	if !cancelResp.Success {
		t.Fatal("expected success")
	}

	// Verify job status
	got, err := srv.GetJob(context.Background(), &pb.GetJobRequest{JobID: submitResp.JobID})
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Job.Status != pb.JobStatusCancelled {
		t.Errorf("expected Cancelled, got %v", got.Job.Status)
	}

	// Verify units cancelled
	units, _ := srv.store.ListUnitsByJob(submitResp.JobID)
	for _, u := range units {
		if u.Status != pb.UnitStatusCancelled {
			t.Errorf("unit %s: expected Cancelled, got %v", u.ID, u.Status)
		}
	}
}

func TestCancelJob_NotFound(t *testing.T) {
	srv := newTestServer(t)
	_, err := srv.CancelJob(context.Background(), &pb.CancelJobRequest{
		JobID:  "nonexistent",
		NodeID: "node-1",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestSubmitJob_InvalidSplit(t *testing.T) {
	srv := newTestServer(t)
	job := &pb.Job{
		Name:    "bad-split",
		Type:    pb.JobTypeAggregate,
		OwnerID: "node-1",
		Stages: []*pb.Stage{
			{
				Name: "bad",
				Split: &pb.SplitStrategy{
					Type:   pb.SplitTypeByFile,
					ByFile: &pb.ByFileSplit{}, // empty file list
				},
			},
		},
	}
	_, err := srv.SubmitJob(context.Background(), &pb.SubmitJobRequest{Job: job})
	if err == nil {
		t.Fatal("expected error for invalid split config")
	}
}

// ========== 邀请码测试 ==========

func TestCreateInviteCode(t *testing.T) {
	srv := newTestServer(t)
	resp, err := srv.CreateInviteCode(context.Background(), &pb.CreateInviteCodeRequest{
		NodeID: "node-1",
	})
	if err != nil {
		t.Fatalf("CreateInviteCode: %v", err)
	}
	if resp.Code == "" {
		t.Fatal("expected non-empty invite code")
	}
	if len(resp.Code) != 32 {
		t.Errorf("expected code length 32, got %d", len(resp.Code))
	}
	if resp.ExpiresAt <= time.Now().UnixMilli() {
		t.Error("expected ExpiresAt in the future")
	}
}

func TestCreateInviteCode_CustomExpiry(t *testing.T) {
	srv := newTestServer(t)
	future := time.Now().Add(24 * time.Hour).UnixMilli()
	resp, err := srv.CreateInviteCode(context.Background(), &pb.CreateInviteCodeRequest{
		NodeID:    "node-1",
		ExpiresAt: future,
	})
	if err != nil {
		t.Fatalf("CreateInviteCode: %v", err)
	}
	if resp.ExpiresAt != future {
		t.Errorf("expected ExpiresAt %d, got %d", future, resp.ExpiresAt)
	}
}

func TestRedeemInviteCode_Valid(t *testing.T) {
	srv := newTestServer(t)
	createResp, err := srv.CreateInviteCode(context.Background(), &pb.CreateInviteCodeRequest{
		NodeID: "node-1",
	})
	if err != nil {
		t.Fatalf("CreateInviteCode: %v", err)
	}

	redeemResp, err := srv.RedeemInviteCode(context.Background(), &pb.RedeemInviteCodeRequest{
		Code:   createResp.Code,
		NodeID: "node-2",
	})
	if err != nil {
		t.Fatalf("RedeemInviteCode: %v", err)
	}
	if !redeemResp.Valid {
		t.Fatalf("expected Valid=true, got Valid=false: %s", redeemResp.Message)
	}
}

func TestRedeemInviteCode_NotFound(t *testing.T) {
	srv := newTestServer(t)
	resp, err := srv.RedeemInviteCode(context.Background(), &pb.RedeemInviteCodeRequest{
		Code:   "nonexistent-code",
		NodeID: "node-1",
	})
	if err != nil {
		t.Fatalf("RedeemInviteCode: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected Valid=false for nonexistent code")
	}
}

func TestRedeemInviteCode_AlreadyUsed(t *testing.T) {
	srv := newTestServer(t)
	createResp, err := srv.CreateInviteCode(context.Background(), &pb.CreateInviteCodeRequest{
		NodeID: "node-1",
	})
	if err != nil {
		t.Fatalf("CreateInviteCode: %v", err)
	}

	// 第一次兑换
	redeemResp, err := srv.RedeemInviteCode(context.Background(), &pb.RedeemInviteCodeRequest{
		Code:   createResp.Code,
		NodeID: "node-2",
	})
	if err != nil {
		t.Fatalf("RedeemInviteCode: %v", err)
	}
	if !redeemResp.Valid {
		t.Fatalf("expected first redeem to succeed: %s", redeemResp.Message)
	}

	// 第二次兑换（应失败）
	redeemResp2, err := srv.RedeemInviteCode(context.Background(), &pb.RedeemInviteCodeRequest{
		Code:   createResp.Code,
		NodeID: "node-3",
	})
	if err != nil {
		t.Fatalf("RedeemInviteCode: %v", err)
	}
	if redeemResp2.Valid {
		t.Fatal("expected second redeem to fail")
	}
}

func TestRegisterNode_ColdStart(t *testing.T) {
	srv := newTestServer(t)
	resp, err := srv.RegisterNode(context.Background(), &pb.RegisterNodeRequest{
		Name: "first-node",
	})
	if err != nil {
		t.Fatalf("RegisterNode (cold start): %v", err)
	}
	if resp.NodeID == "" {
		t.Fatal("expected non-empty node ID")
	}
}

func TestRegisterNode_WithoutInvite(t *testing.T) {
	srv := newTestServer(t)
	// 先注册一个节点（冷启动）
	_, err := srv.RegisterNode(context.Background(), &pb.RegisterNodeRequest{
		Name: "node-1",
	})
	if err != nil {
		t.Fatalf("first node: %v", err)
	}

	// 第二个节点无邀请码应失败
	_, err = srv.RegisterNode(context.Background(), &pb.RegisterNodeRequest{
		Name: "node-2",
	})
	if err == nil {
		t.Fatal("expected error for registration without invite code")
	}
}

func TestRegisterNode_WithValidInvite(t *testing.T) {
	srv := newTestServer(t)
	// 先注册一个节点（冷启动）
	_, err := srv.RegisterNode(context.Background(), &pb.RegisterNodeRequest{
		Name: "node-1",
	})
	if err != nil {
		t.Fatalf("first node: %v", err)
	}

	// 创建邀请码
	inviteResp, err := srv.CreateInviteCode(context.Background(), &pb.CreateInviteCodeRequest{
		NodeID: "node-1",
	})
	if err != nil {
		t.Fatalf("CreateInviteCode: %v", err)
	}

	// 持邀请码注册
	_, err = srv.RegisterNode(context.Background(), &pb.RegisterNodeRequest{
		Name:       "node-2",
		InviteCode: inviteResp.Code,
	})
	if err != nil {
		t.Fatalf("RegisterNode with invite code: %v", err)
	}
}

func TestRegisterNode_WithAdminKey(t *testing.T) {
	cfg := config.Default()
	cfg.Invitation.AdminKey = "my-admin-key-123"
	srv := newTestServerWithConfig(t, cfg)

	// 先注册一个节点（冷启动）
	_, err := srv.RegisterNode(context.Background(), &pb.RegisterNodeRequest{
		Name: "node-1",
	})
	if err != nil {
		t.Fatalf("first node: %v", err)
	}

	// 第二个节点使用 AdminKey 绕过邀请码
	_, err = srv.RegisterNode(context.Background(), &pb.RegisterNodeRequest{
		Name:       "node-2",
		InviteCode: "my-admin-key-123",
	})
	if err != nil {
		t.Fatalf("RegisterNode with admin key: %v", err)
	}
}

func TestRegisterNode_DuplicateFingerprint(t *testing.T) {
	srv := newTestServer(t)
	// 注册第一个节点
	firstResp, err := srv.RegisterNode(context.Background(), &pb.RegisterNodeRequest{
		Name:                "node-1",
		HardwareFingerprint: "abc-123",
	})
	if err != nil {
		t.Fatalf("first node: %v", err)
	}

	// 第二个节点需要邀请码
	inviteResp, err := srv.CreateInviteCode(context.Background(), &pb.CreateInviteCodeRequest{
		NodeID: "node-1",
	})
	if err != nil {
		t.Fatalf("CreateInviteCode: %v", err)
	}

	// 使用相同硬件指纹注册应幂等成功（复用原有节点）
	resp, err := srv.RegisterNode(context.Background(), &pb.RegisterNodeRequest{
		Name:                "node-2",
		InviteCode:          inviteResp.Code,
		HardwareFingerprint: "abc-123",
	})
	if err != nil {
		t.Fatalf("re-register with same fingerprint: %v", err)
	}
	if resp.NodeID != firstResp.NodeID {
		t.Fatalf("expected re-registered node ID %q, got %q", firstResp.NodeID, resp.NodeID)
	}
}