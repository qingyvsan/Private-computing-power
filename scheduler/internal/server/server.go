package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "computing-power/proto/v1"

	"computing-power/scheduler/internal/registry"
	"computing-power/scheduler/internal/store"
)

// Server 调度器 gRPC 服务器
type Server struct {
	store    *store.Store
	registry *registry.Registry
}

// New 创建调度器服务器
func New(st *store.Store, reg *registry.Registry) *Server {
	return &Server{
		store:    st,
		registry: reg,
	}
}

// Register 注册 gRPC 服务
func (s *Server) Register(grpcServer *grpc.Server) {
	pb.RegisterSchedulerServiceServer(grpcServer, s)
}

// Start 启动 gRPC 服务器
func (s *Server) Start(ctx context.Context, listen string, grpcServer *grpc.Server) error {
	lis, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", listen, err)
	}
	log.Printf("gRPC server listening on %s", listen)

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}
	return nil
}

// ========== 节点管理 ==========

// RegisterNode 注册新节点
func (s *Server) RegisterNode(ctx context.Context, req *pb.RegisterNodeRequest) (*pb.RegisterNodeResponse, error) {
	nodeID := generateNodeID(req.Name)
	node := &pb.Node{
		ID:           nodeID,
		Name:         req.Name,
		PublicKey:    req.PublicKey,
		Version:      req.Version,
		Resources:    req.InitialResources,
		Status:       pb.NodeStatusOnline,
		Discoverable: "trust_only",
		RegisteredAt: time.Now().UnixMilli(),
		Reputation:   1.0,
		MaxTasks:     10,
	}

	s.registry.Register(node)
	if err := s.store.SaveNode(node); err != nil {
		return nil, status.Errorf(codes.Internal, "save node: %v", err)
	}

	log.Printf("node registered: %s (%s) version %s", nodeID, req.Name, req.Version)
	return &pb.RegisterNodeResponse{
		NodeID:    nodeID,
		OverlayIP: "10.1.0.1", // 占位，P6 阶段由 Nebula CA 分配
	}, nil
}

// UnregisterNode 注销节点
func (s *Server) UnregisterNode(ctx context.Context, req *pb.UnregisterNodeRequest) (*pb.UnregisterNodeResponse, error) {
	s.registry.Unregister(req.NodeID)
	log.Printf("node unregistered: %s reason: %s", req.NodeID, req.Reason)
	return &pb.UnregisterNodeResponse{Success: true}, nil
}

// Heartbeat 心跳双向流
func (s *Server) Heartbeat(stream pb.Scheduler_HeartbeatServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		s.registry.ReportHeartbeat(req.NodeID, req.Resources, req.RunningUnits)

		resp := &pb.HeartbeatResponse{
			ServerTime: time.Now().UnixMilli(),
			Commands:   []*pb.Command{},
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

// ========== 作业管理 ==========

// SubmitJob 提交作业
func (s *Server) SubmitJob(ctx context.Context, req *pb.SubmitJobRequest) (*pb.SubmitJobResponse, error) {
	job := req.Job
	now := time.Now().UnixMilli()
	if job.ID == "" {
		job.ID = fmt.Sprintf("job-%d", now)
	}
	if job.CreatedAt == 0 {
		job.CreatedAt = now
	}
	job.UpdatedAt = now
	job.Status = pb.JobStatusPending

	// 为 Job 的每个 Stage 生成 ID 并创建 Unit
	for _, stage := range job.Stages {
		if stage.ID == "" {
			stage.ID = fmt.Sprintf("stage-%s-%d", job.ID, now)
		}
		stage.JobID = job.ID
		if stage.MaxConcurrency == 0 {
			stage.MaxConcurrency = 10
		}
	}

	if err := s.store.SaveJob(job); err != nil {
		return nil, status.Errorf(codes.Internal, "save job: %v", err)
	}

	log.Printf("job submitted: %s type=%s owner=%s", job.ID, job.Type, job.OwnerID)
	return &pb.SubmitJobResponse{
		JobID:   job.ID,
		Status:  job.Status,
		Message: "job accepted",
	}, nil
}

// GetJob 获取作业
func (s *Server) GetJob(ctx context.Context, req *pb.GetJobRequest) (*pb.GetJobResponse, error) {
	job, err := s.store.GetJob(req.JobID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get job: %v", err)
	}
	if job == nil {
		return nil, status.Errorf(codes.NotFound, "job %s not found", req.JobID)
	}
	return &pb.GetJobResponse{Job: job}, nil
}

// ListJobs 列出作业
func (s *Server) ListJobs(ctx context.Context, req *pb.ListJobsRequest) (*pb.ListJobsResponse, error) {
	jobs, err := s.store.ListJobs(req.NodeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list jobs: %v", err)
	}
	return &pb.ListJobsResponse{
		Jobs:       jobs,
		TotalCount: int32(len(jobs)),
	}, nil
}

// CancelJob 取消作业
func (s *Server) CancelJob(ctx context.Context, req *pb.CancelJobRequest) (*pb.CancelJobResponse, error) {
	if err := s.store.UpdateJobStatus(req.JobID, pb.JobStatusCancelled); err != nil {
		return nil, status.Errorf(codes.Internal, "cancel job: %v", err)
	}
	log.Printf("job cancelled: %s by %s", req.JobID, req.NodeID)
	return &pb.CancelJobResponse{Success: true}, nil
}

// WatchJob 订阅作业状态变更
func (s *Server) WatchJob(req *pb.WatchJobRequest, stream pb.Scheduler_WatchJobServer) error {
	// P2 阶段实现：通过事件总线推送 JobEvent
	job, err := s.store.GetJob(req.JobID)
	if err != nil {
		return status.Errorf(codes.Internal, "get job: %v", err)
	}
	if job == nil {
		return status.Errorf(codes.NotFound, "job %s not found", req.JobID)
	}
	// 发送当前状态快照
	ev := &pb.JobEvent{
		JobID:     job.ID,
		Status:    job.Status,
		Timestamp: time.Now().UnixMilli(),
	}
	if err := stream.Send(ev); err != nil {
		return err
	}
	return nil
}

// ========== 任务分配 ==========

// AssignUnit 分配任务到节点
func (s *Server) AssignUnit(ctx context.Context, req *pb.AssignUnitRequest) (*pb.AssignUnitResponse, error) {
	// P3 阶段实现完整的调度逻辑
	// 当前返回拒绝，等待调度引擎实现
	return &pb.AssignUnitResponse{
		Accepted: false,
		Message:  "scheduling engine not implemented yet",
	}, nil
}

// ReportUnitStatus 接收 Unit 状态上报
func (s *Server) ReportUnitStatus(stream pb.Scheduler_ReportUnitStatusServer) error {
	for {
		report, err := stream.Recv()
		if err != nil {
			return err
		}
		// 更新 Unit 状态
		if report.UnitID != "" {
			unit, err := s.store.GetUnit(report.UnitID)
			if err == nil && unit != nil {
				unit.Status = report.Status
				unit.ExitCode = report.ExitCode
				unit.ErrorMessage = report.ErrorMessage
				if report.Output != nil {
					unit.Output = report.Output
				}
				if report.Status == pb.UnitStatusCompleted {
					unit.CompletedAt = time.Now().UnixMilli()
				}
				s.store.SaveUnit(unit)
			}
		}
		if err := stream.Send(&pb.UnitStatusAck{Received: true}); err != nil {
			return err
		}
	}
}

// ========== 信任管理 ==========

// DeclareTrust 声明信任
func (s *Server) DeclareTrust(ctx context.Context, req *pb.DeclareTrustRequest) (*pb.DeclareTrustResponse, error) {
	// P7 阶段实现：验证签名、存储信任边
	return &pb.DeclareTrustResponse{Success: true}, nil
}

// RevokeTrust 撤销信任
func (s *Server) RevokeTrust(ctx context.Context, req *pb.RevokeTrustRequest) (*pb.RevokeTrustResponse, error) {
	return &pb.RevokeTrustResponse{Success: true}, nil
}

// GetTrustGraph 获取信任图
func (s *Server) GetTrustGraph(ctx context.Context, req *pb.GetTrustGraphRequest) (*pb.GetTrustGraphResponse, error) {
	edges, err := s.store.ListTrustEdges()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list trust edges: %v", err)
	}
	return &pb.GetTrustGraphResponse{Edges: edges}, nil
}

// ========== 证书管理 ==========

// IssueCertificate 签发证书
func (s *Server) IssueCertificate(ctx context.Context, req *pb.IssueCertRequest) (*pb.IssueCertResponse, error) {
	// P6 阶段实现：内置 CA 签发 Nebula 证书
	return &pb.IssueCertResponse{ExpiresAt: time.Now().Add(24 * time.Hour).UnixMilli()}, nil
}

// RenewCertificate 续期证书
func (s *Server) RenewCertificate(ctx context.Context, req *pb.RenewCertRequest) (*pb.RenewCertResponse, error) {
	return &pb.RenewCertResponse{ExpiresAt: time.Now().Add(24 * time.Hour).UnixMilli()}, nil
}

// RevokeCertificate 吊销证书
func (s *Server) RevokeCertificate(ctx context.Context, req *pb.RevokeCertRequest) (*pb.RevokeCertResponse, error) {
	return &pb.RevokeCertResponse{Success: true}, nil
}

// ========== 邀请码 ==========

// CreateInviteCode 创建邀请码
func (s *Server) CreateInviteCode(ctx context.Context, req *pb.CreateInviteCodeRequest) (*pb.CreateInviteCodeResponse, error) {
	return &pb.CreateInviteCodeResponse{
		Code:      "placeholder-invite-code",
		ExpiresAt: time.Now().Add(72 * time.Hour).UnixMilli(),
	}, nil
}

// RedeemInviteCode 兑换邀请码
func (s *Server) RedeemInviteCode(ctx context.Context, req *pb.RedeemInviteCodeRequest) (*pb.RedeemInviteCodeResponse, error) {
	return &pb.RedeemInviteCodeResponse{Valid: true, Message: "redeemed"}, nil
}

// ========== 节点查询 ==========

// ListNodes 列出所有节点
func (s *Server) ListNodes(ctx context.Context, req *pb.ListNodesRequest) (*pb.ListNodesResponse, error) {
	nodes := s.registry.ListAll()
	return &pb.ListNodesResponse{
		Nodes:      nodes,
		TotalCount: int32(len(nodes)),
	}, nil
}

// GetNode 获取节点详情
func (s *Server) GetNode(ctx context.Context, req *pb.GetNodeRequest) (*pb.GetNodeResponse, error) {
	node := s.registry.GetNode(req.NodeID)
	if node == nil {
		return nil, status.Errorf(codes.NotFound, "node %s not found", req.NodeID)
	}
	return &pb.GetNodeResponse{Node: node}, nil
}

// generateNodeID 生成节点 ID
func generateNodeID(name string) string {
	return fmt.Sprintf("node-%d-%s", time.Now().UnixMilli()%100000, shortName(name))
}

func shortName(name string) string {
	if len(name) <= 8 {
		return name
	}
	return name[:8]
}