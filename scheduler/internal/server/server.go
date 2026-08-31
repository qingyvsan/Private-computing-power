package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"text/template"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "computing-power/proto/v1"

	"computing-power/pkg/trustgraph"
	"computing-power/scheduler/internal/ca"
	"computing-power/scheduler/internal/config"
	"computing-power/scheduler/internal/events"
	"computing-power/scheduler/internal/ipam"
	"computing-power/scheduler/internal/registry"

	"computing-power/scheduler/internal/scheduler"
	"computing-power/scheduler/internal/store"
)

// Server 调度器 gRPC 服务器
type Server struct {
	store    *store.Store
	registry *registry.Registry
	trust    *trustgraph.Graph
	eventBus *events.Bus
	engine   *scheduler.Engine
	stopCh   chan struct{}
	ca       *ca.Manager
	ipam     *ipam.IPAM
	cfg      *config.Config

	heartbeatInterval time.Duration
}

// New 创建调度器服务器
func New(st *store.Store, reg *registry.Registry, trust *trustgraph.Graph,
	heartbeatInterval, heartbeatTimeout time.Duration, cfg *config.Config,
	caMgr *ca.Manager, ipamMgr *ipam.IPAM) *Server {
	reg.SetHeartbeatTimeout(heartbeatTimeout)

	// 创建调度引擎
	reassignDelay, _ := time.ParseDuration(cfg.SchedulerEngine.ReassignDelay)
	eng := scheduler.New(st, reg, trust,
		cfg.SchedulerEngine.MaxRetries,
		reassignDelay,
		cfg.SchedulerEngine.ConcurrentAssignments,
		scheduler.ScoringWeights{
			ResourceMatch:  cfg.SchedulerEngine.ScoringWeights.ResourceMatch,
			NetworkQuality: cfg.SchedulerEngine.ScoringWeights.NetworkQuality,
			Reputation:     cfg.SchedulerEngine.ScoringWeights.Reputation,
			Load:           cfg.SchedulerEngine.ScoringWeights.Load,
		},
	)

	return &Server{
		store:             st,
		registry:          reg,
		trust:             trust,
		eventBus:          events.NewBus(),
		engine:            eng,
		stopCh:            make(chan struct{}),
		ca:                caMgr,
		ipam:              ipamMgr,
		cfg:               cfg,
		heartbeatInterval: heartbeatInterval,
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

	// 从 BoltDB 恢复节点到注册中心
	if nodes, err := s.store.ListNodes(); err == nil {
		s.registry.LoadNodes(nodes)
	} else {
		log.Printf("load nodes from store: %v", err)
	}

	// 启动定期故障检测循环（2 倍心跳间隔）
	checkInterval := s.heartbeatInterval * 2
	if checkInterval < time.Second {
		checkInterval = 5 * time.Second
	}
	s.registry.Start(ctx.Done(), checkInterval)

	// 启动调度引擎
	s.engine.Start(ctx)

	// 启动信任过期清理
	s.startTrustExpiryLoop(ctx)

	go func() {
		<-ctx.Done()
		close(s.stopCh)
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

	// 邀请码 / AdminKey / 冷启动验证
	if err := s.validateRegistration(ctx, nodeID, req); err != nil {
		return nil, err
	}

	// 分配 Overlay IP
	overlayIP, err := s.ipam.Allocate(nodeID)
	if err != nil {
		log.Printf("ipam allocate for %s: %v", nodeID, err)
		// IPAM 失败时使用占位 IP（不影响节点注册）
		overlayIP = "0.0.0.0"
	}

	// 签发 Nebula 证书（如果 CA 可用）
	var nebulaCert, nebulaKey, caCert []byte
	var nebulaConfig string
	if s.ca != nil && s.ca.IsCAValid() {
		ip := net.ParseIP(overlayIP)
		ips := []net.IP{}
		if ip != nil {
			ips = append(ips, ip)
		}
		cert, key, err := s.ca.IssueNodeCert(nodeID, []string{"default"}, ips)
		if err == nil {
			nebulaCert = cert
			nebulaKey = key
			caCert = s.ca.CACertPEM()
			nebulaConfig = s.buildNebulaConfig(overlayIP)
		} else {
			log.Printf("issue nebula cert for %s: %v", nodeID, err)
		}
	}

	node := &pb.Node{
		ID:                  nodeID,
		Name:                req.Name,
		OverlayIP:           overlayIP,
		PublicKey:           req.PublicKey,
		HardwareFingerprint: req.HardwareFingerprint,
		Version:             req.Version,
		Resources:           req.InitialResources,
		Status:              pb.NodeStatusOnline,
		Discoverable:        "public",
		RegisteredAt:        time.Now().UnixMilli(),
		Reputation:          1.0,
		MaxTasks:            10,
	}

	s.registry.Register(node)
	if err := s.store.SaveNode(node); err != nil {
		return nil, status.Errorf(codes.Internal, "save node: %v", err)
	}

	log.Printf("node registered: %s (%s) version %s overlay %s", nodeID, req.Name, req.Version, overlayIP)
	return &pb.RegisterNodeResponse{
		NodeID:             nodeID,
		OverlayIP:          overlayIP,
		NebulaCertificate:  nebulaCert,
		NebulaPrivateKey:   nebulaKey,
		NebulaConfig:       nebulaConfig,
		CACertificate:      caCert,
	}, nil
}

// UnregisterNode 注销节点
func (s *Server) UnregisterNode(ctx context.Context, req *pb.UnregisterNodeRequest) (*pb.UnregisterNodeResponse, error) {
	s.registry.Unregister(req.NodeID)
	if s.ipam != nil {
		if err := s.ipam.Release(req.NodeID); err != nil {
			log.Printf("ipam release for %s: %v", req.NodeID, err)
		}
	}
	if err := s.store.DeleteNode(req.NodeID); err != nil {
		log.Printf("store delete node %s: %v", req.NodeID, err)
	}
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

		// 获取节点当前状态和 φ 值
		now := time.Now()
		node := s.registry.GetNode(req.NodeID)
		phi := 0.0
		status := pb.NodeStatusOnline
		if node != nil {
			phi = node.PhiValue
			status = node.Status
		}

		resp := &pb.HeartbeatResponse{
			ServerTime:        now.UnixMilli(),
			NodeStatus:        status,
			PhiValue:          phi,
			HeartbeatInterval: s.heartbeatInterval.String(),
			Commands:           s.engine.PopCommands(req.NodeID),
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
		job.ID = fmt.Sprintf("job-%d-%d", now, time.Now().UnixNano()%100000)
	}
	if job.CreatedAt == 0 {
		job.CreatedAt = now
	}
	job.UpdatedAt = now
	job.Status = pb.JobStatusPending

	// 为 Job 的每个 Stage 生成 ID
	for i, stage := range job.Stages {
		if stage.ID == "" {
			stage.ID = fmt.Sprintf("stage-%s-%d-%d", job.ID, now, i)
		}
		stage.JobID = job.ID
		if stage.MaxConcurrency == 0 {
			stage.MaxConcurrency = 10
		}
	}

	// 执行拆分策略，创建 Unit
	var totalUnits int32
	for _, stage := range job.Stages {
		units, err := executeSplit(stage)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "split stage %s: %v", stage.ID, err)
		}
		for _, unit := range units {
			if err := s.store.SaveUnit(unit); err != nil {
				return nil, status.Errorf(codes.Internal, "save unit: %v", err)
			}
		}
		stage.Units = units
		totalUnits += int32(len(units))
	}

	if err := s.store.SaveJob(job); err != nil {
		return nil, status.Errorf(codes.Internal, "save job: %v", err)
	}

	// 发布事件
	s.eventBus.Publish(&pb.JobEvent{
		JobID:     job.ID,
		Status:    job.Status,
		Message:   fmt.Sprintf("job submitted with %d units across %d stages", totalUnits, len(job.Stages)),
		Timestamp: time.Now().UnixMilli(),
	})

	// 触发调度引擎
	s.engine.ScheduleNow()

	log.Printf("job submitted: %s type=%s owner=%s stages=%d units=%d",
		job.ID, job.Type, job.OwnerID, len(job.Stages), totalUnits)
	return &pb.SubmitJobResponse{
		JobID:   job.ID,
		Status:  job.Status,
		Message: fmt.Sprintf("job accepted with %d units across %d stages", totalUnits, len(job.Stages)),
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

// CancelJob 取消作业（级联取消所有 Unit 和 Stage）
func (s *Server) CancelJob(ctx context.Context, req *pb.CancelJobRequest) (*pb.CancelJobResponse, error) {
	// 加载作业
	job, err := s.store.GetJob(req.JobID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get job: %v", err)
	}
	if job == nil {
		return nil, status.Errorf(codes.NotFound, "job %s not found", req.JobID)
	}

	// 级联取消所有 Pending/Running 的 Unit
	units, err := s.store.ListUnitsByJob(req.JobID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list units: %v", err)
	}
	for _, u := range units {
		if u.Status == pb.UnitStatusPending || u.Status == pb.UnitStatusAssigned || u.Status == pb.UnitStatusRunning {
			if _, err := s.store.UpdateUnitStatus(u.ID, pb.UnitStatusCancelled, 0, "job cancelled"); err != nil {
				log.Printf("cancel unit %s: %v", u.ID, err)
			}
		}
	}

	// 更新所有 Stage 状态为 CANCELLED
	for _, stage := range job.Stages {
		if stage.Status != pb.StageStatusCompleted && stage.Status != pb.StageStatusFailed {
			stage.Status = pb.StageStatusSkipped
		}
	}

	// 更新作业状态
	if err := s.store.UpdateJobStatus(req.JobID, pb.JobStatusCancelled); err != nil {
		return nil, status.Errorf(codes.Internal, "cancel job: %v", err)
	}

	// 发布事件
	s.eventBus.Publish(&pb.JobEvent{
		JobID:     req.JobID,
		Status:    pb.JobStatusCancelled,
		Message:   "job cancelled by " + req.NodeID,
		Timestamp: time.Now().UnixMilli(),
	})

	// 触发调度引擎
	s.engine.ScheduleNow()

	log.Printf("job cancelled: %s by %s (%d units affected)", req.JobID, req.NodeID, len(units))
	return &pb.CancelJobResponse{Success: true}, nil
}

// WatchJob 订阅作业状态变更（流式推送）
func (s *Server) WatchJob(req *pb.WatchJobRequest, stream pb.Scheduler_WatchJobServer) error {
	// 先发送当前状态快照
	job, err := s.store.GetJob(req.JobID)
	if err != nil {
		return status.Errorf(codes.Internal, "get job: %v", err)
	}
	if job == nil {
		return status.Errorf(codes.NotFound, "job %s not found", req.JobID)
	}

	// 发送当前快照
	ev := &pb.JobEvent{
		JobID:     job.ID,
		Status:    job.Status,
		Message:   "current snapshot",
		Timestamp: time.Now().UnixMilli(),
	}
	if err := stream.Send(ev); err != nil {
		return err
	}

	// 订阅后续事件
	subscriberID := fmt.Sprintf("watch-%d", time.Now().UnixNano())
	ch := s.eventBus.Subscribe(req.JobID, subscriberID)
	defer s.eventBus.Unsubscribe(req.JobID, subscriberID)

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}
}

// ========== 任务分配 ==========

// AssignUnit 分配任务到节点（pull 模式）
func (s *Server) AssignUnit(ctx context.Context, req *pb.AssignUnitRequest) (*pb.AssignUnitResponse, error) {
	if req.NodeID == "" {
		return &pb.AssignUnitResponse{
			Accepted: false,
			Message:  "node_id is required",
		}, nil
	}

	unit, err := s.engine.AssignToNode(req.NodeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "assign unit: %v", err)
	}
	if unit == nil {
		return &pb.AssignUnitResponse{
			Accepted: false,
			Message:  "no suitable unit for node",
		}, nil
	}

	return &pb.AssignUnitResponse{
		Accepted: true,
		Message:  "unit assigned",
		UnitID:   unit.ID,
	}, nil
}

// ReportUnitStatus 接收 Unit 状态上报并传播状态变更
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
				oldStatus := unit.Status
				unit.Status = report.Status
				unit.ExitCode = report.ExitCode
				unit.ErrorMessage = report.ErrorMessage
				if report.Output != nil {
					unit.Output = report.Output
				}
				if report.Status == pb.UnitStatusRunning && unit.StartedAt == 0 {
					unit.StartedAt = time.Now().UnixMilli()
				}
				if report.Status == pb.UnitStatusCompleted || report.Status == pb.UnitStatusFailed || report.Status == pb.UnitStatusCancelled {
					unit.CompletedAt = time.Now().UnixMilli()
				}
				s.store.SaveUnit(unit)

				// 状态变更时检查 Stage / Job 是否完成
				if oldStatus != report.Status {
					// 失败时触发重试调度
						if report.Status == pb.UnitStatusFailed && int(unit.RetryCount) < s.engine.MaxRetries() {
							unit.RetryCount++
							unit.Status = pb.UnitStatusPending
							unit.AssignedNode = ""
							unit.ErrorMessage = ""
							s.store.SaveUnit(unit)
							s.engine.ScheduleNow()
						} else {
							s.propagateUnitStatus(unit)
						}
				}
			}
		}
		if err := stream.Send(&pb.UnitStatusAck{Received: true}); err != nil {
			return err
		}
	}
}

// propagateUnitStatus 检查 Unit 所属 Stage 和 Job 的状态
func (s *Server) propagateUnitStatus(unit *pb.Unit) {
	// 查找 Unit 所属的 Job
	job, err := s.store.GetJob(unit.JobID)
	if err != nil || job == nil {
		return
	}

	// 找到对应的 Stage
	for _, stage := range job.Stages {
		if stage.ID != unit.StageID {
			continue
		}

		// 获取该 Stage 的所有 Unit
		stageUnits, err := s.store.ListUnitsByStage(stage.ID)
		if err != nil {
			return
		}

		// 检查是否所有 Unit 都处于终结状态
		allTerminal := true
		hasFailed := false
		hasRunning := false
		for _, u := range stageUnits {
			switch u.Status {
			case pb.UnitStatusFailed, pb.UnitStatusCancelled, pb.UnitStatusTimeout:
				hasFailed = true
			case pb.UnitStatusRunning, pb.UnitStatusAssigned, pb.UnitStatusPending:
				allTerminal = false
				if u.Status == pb.UnitStatusRunning {
					hasRunning = true
				}
			case pb.UnitStatusCompleted:
				// completed is terminal
			}
		}

		// 更新 Stage 状态
		var newStageStatus pb.StageStatus
		switch {
		case hasRunning:
			newStageStatus = pb.StageStatusRunning
		case allTerminal && hasFailed:
			newStageStatus = pb.StageStatusFailed
		case allTerminal && !hasFailed:
			newStageStatus = pb.StageStatusCompleted
		default:
			newStageStatus = pb.StageStatusRunning
		}
		if newStageStatus != stage.Status {
			stage.Status = newStageStatus
			s.store.UpdateStageStatus(stage.ID, newStageStatus)
		}
		break
	}

	// 检查所有 Stage 是否都终结 → 更新 Job 状态
	allStagesTerminal := true
	anyStageFailed := false
	for _, stage := range job.Stages {
		if stage.Status != pb.StageStatusCompleted && stage.Status != pb.StageStatusFailed && stage.Status != pb.StageStatusSkipped {
			allStagesTerminal = false
		}
		if stage.Status == pb.StageStatusFailed {
			anyStageFailed = true
		}
	}
	if allStagesTerminal {
		newStatus := pb.JobStatusCompleted
		if anyStageFailed {
			newStatus = pb.JobStatusFailed
		}
		s.store.UpdateJobStatus(job.ID, newStatus)
		s.eventBus.Publish(&pb.JobEvent{
			JobID:     job.ID,
			Status:    newStatus,
			Message:   fmt.Sprintf("job completed with status %s", newStatus),
			Timestamp: time.Now().UnixMilli(),
		})
		// 触发调度引擎
		s.engine.ScheduleNow()
	}
}

// ========== 信任管理 ==========

// DeclareTrust 声明信任
func (s *Server) DeclareTrust(ctx context.Context, req *pb.DeclareTrustRequest) (*pb.DeclareTrustResponse, error) {
	if req.FromNodeID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "from_node_id is required")
	}
	if req.TargetNodeID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "target_node_id is required")
	}
	if req.FromNodeID == req.TargetNodeID {
		return nil, status.Errorf(codes.InvalidArgument, "self-trust is not allowed")
	}

	// 查找声明节点的公钥
	node, err := s.store.GetNode(req.FromNodeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "lookup node: %v", err)
	}
	if node == nil {
		return nil, status.Errorf(codes.NotFound, "node %s not found", req.FromNodeID)
	}
	if len(node.PublicKey) == 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "node %s has no public key registered", req.FromNodeID)
	}

	// 验证签名
	if len(req.Signature) > 0 {
		if err := trustgraph.VerifyTrust(node.PublicKey, req.FromNodeID, req.TargetNodeID, req.Signature); err != nil {
			return nil, status.Errorf(codes.PermissionDenied, "signature verification failed: %v", err)
		}
	}

	// 解析过期时间
	var expiresAt *time.Time
	if req.ExpiresAt > 0 {
		t := time.UnixMilli(req.ExpiresAt)
		expiresAt = &t
	}

	// 更新内存图
	if err := s.trust.AddEdge(req.FromNodeID, req.TargetNodeID, req.Signature, expiresAt); err != nil {
		return nil, status.Errorf(codes.Internal, "add trust edge: %v", err)
	}

	// 持久化到 BoltDB
	edge := &pb.TrustEdge{
		FromNode:  req.FromNodeID,
		ToNode:    req.TargetNodeID,
		Signature: req.Signature,
		CreatedAt: time.Now().UnixMilli(),
		ExpiresAt: req.ExpiresAt,
	}
	if err := s.store.SaveTrustEdge(edge); err != nil {
		// 持久化失败时回滚内存状态
		_ = s.trust.RemoveEdge(req.FromNodeID, req.TargetNodeID)
		return nil, status.Errorf(codes.Internal, "save trust edge: %v", err)
	}

	log.Printf("trust declared: %s -> %s (expires=%d)", req.FromNodeID, req.TargetNodeID, req.ExpiresAt)
	return &pb.DeclareTrustResponse{Success: true}, nil
}

// RevokeTrust 撤销信任
func (s *Server) RevokeTrust(ctx context.Context, req *pb.RevokeTrustRequest) (*pb.RevokeTrustResponse, error) {
	if req.FromNodeID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "from_node_id is required")
	}
	if req.TargetNodeID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "target_node_id is required")
	}

	// 查找声明节点的公钥
	node, err := s.store.GetNode(req.FromNodeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "lookup node: %v", err)
	}
	if node == nil {
		return nil, status.Errorf(codes.NotFound, "node %s not found", req.FromNodeID)
	}
	if len(node.PublicKey) == 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "node %s has no public key registered", req.FromNodeID)
	}

	// 验证签名
	if len(req.Signature) > 0 {
		if err := trustgraph.VerifyTrust(node.PublicKey, req.FromNodeID, req.TargetNodeID, req.Signature); err != nil {
			return nil, status.Errorf(codes.PermissionDenied, "signature verification failed: %v", err)
		}
	}

	// 幂等：如果边不存在，直接返回成功
	if !s.trust.HasTrust(req.FromNodeID, req.TargetNodeID) {
		return &pb.RevokeTrustResponse{Success: true}, nil
	}

	// 从内存图中移除
	s.trust.RemoveEdge(req.FromNodeID, req.TargetNodeID)

	// 从 BoltDB 删除
	if err := s.store.DeleteTrustEdge(req.FromNodeID, req.TargetNodeID); err != nil {
		// 删除失败时回滚内存状态
		_ = s.trust.AddEdge(req.FromNodeID, req.TargetNodeID, nil, nil)
		return nil, status.Errorf(codes.Internal, "delete trust edge: %v", err)
	}

	log.Printf("trust revoked: %s -> %s", req.FromNodeID, req.TargetNodeID)
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
	if s.ca == nil || !s.ca.IsCAValid() {
		return nil, status.Errorf(codes.FailedPrecondition, "CA not initialized")
	}

	// 解析 IP 列表
	var ips []net.IP
	if req.IPs != "" {
		for _, ipStr := range splitCSV(req.IPs) {
			if ip := net.ParseIP(ipStr); ip != nil {
				ips = append(ips, ip)
			}
		}
	}

	// 解析分组
	groups := []string{"default"}
	if req.Group != "" {
		groups = splitCSV(req.Group)
	}

	certPEM, _, err := s.ca.IssueNodeCert(req.NodeID, groups, ips)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "issue cert: %v", err)
	}

	return &pb.IssueCertResponse{
		Certificate:   certPEM,
		// 返回私钥（IssueCertResponse 没有 key 字段，但可通过其他方式传递）
		// P6 阶段：节点私钥通过 RegisterNodeResponse 返回
		CACertificate: s.ca.CACertPEM(),
		ExpiresAt:     time.Now().Add(365 * 24 * time.Hour).UnixMilli(),
	}, nil
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

const inviteCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// generateInviteCode 生成安全随机邀请码
func (s *Server) generateInviteCode() (string, error) {
	length := s.cfg.Invitation.CodeLength
	if length <= 0 {
		length = 32
	}
	code := make([]byte, length)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(inviteCharset))))
		if err != nil {
			return "", fmt.Errorf("rand: %w", err)
		}
		code[i] = inviteCharset[n.Int64()]
	}
	return string(code), nil
}

// CreateInviteCode 创建邀请码
func (s *Server) CreateInviteCode(ctx context.Context, req *pb.CreateInviteCodeRequest) (*pb.CreateInviteCodeResponse, error) {
	code, err := s.generateInviteCode()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate code: %v", err)
	}

	// 解析过期时间
	expiry := s.cfg.Invitation.CodeExpiry
	if expiry == "" {
		expiry = "72h"
	}
	expiryDur, err := time.ParseDuration(expiry)
	if err != nil {
		expiryDur = 72 * time.Hour
	}

	expiresAt := time.Now().Add(expiryDur).UnixMilli()
	if req.ExpiresAt > 0 {
		expiresAt = req.ExpiresAt
	}

	maxUses := int32(s.cfg.Invitation.MaxUsesPerCode)
	if req.MaxUses > 0 {
		maxUses = req.MaxUses
	}

	ic := &store.InviteCode{
		Code:      code,
		CreatedBy: req.NodeID,
		CreatedAt: time.Now().UnixMilli(),
		ExpiresAt: expiresAt,
		MaxUses:   maxUses,
	}

	if err := s.store.SaveInviteCode(ic); err != nil {
		return nil, status.Errorf(codes.Internal, "save invite code: %v", err)
	}

	log.Printf("invite code created: %s by %s, expires %d", code, req.NodeID, expiresAt)
	return &pb.CreateInviteCodeResponse{
		Code:      code,
		ExpiresAt: expiresAt,
	}, nil
}

// RedeemInviteCode 兑换邀请码
func (s *Server) RedeemInviteCode(ctx context.Context, req *pb.RedeemInviteCodeRequest) (*pb.RedeemInviteCodeResponse, error) {
	ic, err := s.store.GetInviteCode(req.Code)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get invite code: %v", err)
	}
	if ic == nil {
		return &pb.RedeemInviteCodeResponse{Valid: false, Message: "invite code not found"}, nil
	}

	// 检查过期
	if time.Now().UnixMilli() > ic.ExpiresAt {
		return &pb.RedeemInviteCodeResponse{Valid: false, Message: "invite code expired"}, nil
	}

	// 检查使用次数
	if ic.Used || ic.UsedCount >= ic.MaxUses {
		return &pb.RedeemInviteCodeResponse{Valid: false, Message: "invite code already used"}, nil
	}

	// 更新使用状态
	ic.UsedCount++
	ic.RedeemedBy = append(ic.RedeemedBy, req.NodeID)
	if ic.UsedCount >= ic.MaxUses {
		ic.Used = true
	}

	if err := s.store.SaveInviteCode(ic); err != nil {
		return nil, status.Errorf(codes.Internal, "update invite code: %v", err)
	}

	log.Printf("invite code redeemed: %s by %s", req.Code, req.NodeID)
	return &pb.RedeemInviteCodeResponse{Valid: true, Message: "invite code redeemed successfully"}, nil
}

// validateRegistration 检查注册请求是否合法（邀请码 / AdminKey / 冷启动 / 硬件指纹）
func (s *Server) validateRegistration(ctx context.Context, nodeID string, req *pb.RegisterNodeRequest) error {
	// AdminKey 绕过
	if s.cfg.Invitation.AdminKey != "" && req.InviteCode == s.cfg.Invitation.AdminKey {
		return nil
	}

	// 检查集群是否已有节点（冷启动检测）
	existingNodes, err := s.store.ListNodes()
	if err != nil {
		return status.Errorf(codes.Internal, "check existing nodes: %v", err)
	}

	if len(existingNodes) == 0 {
		// 冷启动：首个节点免邀请码
		return nil
	}

	// 已有节点：必须提供邀请码
	if req.InviteCode == "" {
		return status.Errorf(codes.PermissionDenied, "invite code required for registration")
	}

	// 兑换邀请码
	redeemResp, err := s.RedeemInviteCode(ctx, &pb.RedeemInviteCodeRequest{
		Code:               req.InviteCode,
		NodeID:             nodeID,
		PublicKey:          req.PublicKey,
		HardwareFingerprint: req.HardwareFingerprint,
	})
	if err != nil {
		return err
	}
	if !redeemResp.Valid {
		return status.Errorf(codes.PermissionDenied, "invalid invite code: %s", redeemResp.Message)
	}

	// 硬件指纹重复检查
	if req.HardwareFingerprint != "" {
		existing, err := s.store.GetNodeByFingerprint(req.HardwareFingerprint)
		if err != nil {
			return status.Errorf(codes.Internal, "check fingerprint: %v", err)
		}
		if existing != nil {
			return status.Errorf(codes.AlreadyExists, "node with this hardware fingerprint already registered: %s", existing.ID)
		}
	}

	return nil
}

// ========== 节点查询 ==========

// isNodeVisibleTo 检查节点对请求者是否可见
func (s *Server) isNodeVisibleTo(node *pb.Node, requesterID string) bool {
	switch node.Discoverable {
	case "public":
		return true
	case "hidden":
		if requesterID == "" {
			return false
		}
		return s.trust.HasTrust(requesterID, node.ID) || node.ID == requesterID
	case "trust_only":
		fallthrough
	default:
		if requesterID == "" {
			return false
		}
		return s.trust.IsReachable(requesterID, node.ID, 10) || node.ID == requesterID
	}
}

// ListNodes 列出所有节点
func (s *Server) ListNodes(ctx context.Context, req *pb.ListNodesRequest) (*pb.ListNodesResponse, error) {
	nodes := s.registry.ListAll()
	filtered := make([]*pb.Node, 0, len(nodes))
	for _, n := range nodes {
		if s.isNodeVisibleTo(n, req.RequesterID) {
			filtered = append(filtered, n)
		}
	}
	return &pb.ListNodesResponse{
		Nodes:      filtered,
		TotalCount: int32(len(filtered)),
	}, nil
}

// GetNode 获取节点详情
func (s *Server) GetNode(ctx context.Context, req *pb.GetNodeRequest) (*pb.GetNodeResponse, error) {
	node := s.registry.GetNode(req.NodeID)
	if node == nil {
		return nil, status.Errorf(codes.NotFound, "node %s not found", req.NodeID)
	}
	if !s.isNodeVisibleTo(node, req.RequesterID) {
		return nil, status.Errorf(codes.NotFound, "node %s not found", req.NodeID)
	}
	return &pb.GetNodeResponse{Node: node}, nil
}

// ========== 信任过期清理 ==========

// startTrustExpiryLoop 启动定期过期边清理
func (s *Server) startTrustExpiryLoop(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.pruneExpiredTrust()
			}
		}
	}()
	log.Printf("trust expiry loop started (interval=5m)")
}

// pruneExpiredTrust 清理过期信任边
func (s *Server) pruneExpiredTrust() {
	// 清理内存图
	count := s.trust.PruneExpired()
	if count > 0 {
		log.Printf("trust expiry: pruned %d expired edges from memory", count)
	}

	// 清理 BoltDB 中的过期边
	edges, err := s.store.ListTrustEdges()
	if err != nil {
		log.Printf("trust expiry: list edges: %v", err)
		return
	}
	now := time.Now()
	pruned := 0
	for _, e := range edges {
		if e.ExpiresAt > 0 && now.After(time.UnixMilli(e.ExpiresAt)) {
			if err := s.store.DeleteTrustEdge(e.FromNode, e.ToNode); err != nil {
				log.Printf("trust expiry: delete edge %s->%s: %v", e.FromNode, e.ToNode, err)
			} else {
				pruned++
			}
		}
	}
	if pruned > 0 {
		log.Printf("trust expiry: pruned %d expired edges from store", pruned)
	}
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

// buildNebulaConfig 生成 Nebula 节点配置 YAML
func (s *Server) buildNebulaConfig(overlayIP string) string {
	lighthouseIP := s.ipam.Gateway()
	lighthouseAddr := "8.138.108.183:4242" // 默认生产环境地址

	const tpl = `pki:
  ca: /etc/nebula/ca.crt
  cert: /etc/nebula/node.crt
  key: /etc/nebula/node.key

static_host_map:
  "{{.LighthouseIP}}": ["{{.LighthouseAddr}}"]

lighthouse:
  am_lighthouse: false
  interval: 60
  hosts:
    - "{{.LighthouseIP}}"

listen:
  host: 0.0.0.0
  port: 0

punchy:
  punch: true
  respond: true

relay:
  am_relay: false

tun:
  disabled: false
  dev: cp0
  drop_local_broadcast: false
  drop_multicast: false
  mtu: 1300

firewall:
  outbound_action: accept
  inbound_action: drop
  default_action: drop
  conntrack:
    tcp_timeout: 12m
    udp_timeout: 3m
    default_timeout: 10m
  outbound:
    - port: any
      proto: any
      host: any
  inbound:
    - port: any
      proto: icmp
      host: any
`
	data := struct {
		LighthouseIP   string
		LighthouseAddr string
	}{
		LighthouseIP:   lighthouseIP,
		LighthouseAddr: lighthouseAddr,
	}

	var buf bytes.Buffer
	tmpl, err := template.New("nebula").Parse(tpl)
	if err != nil {
		log.Printf("build nebula config template: %v", err)
		return ""
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Printf("build nebula config execute: %v", err)
		return ""
	}
	return buf.String()
}

// splitCSV 解析逗号分隔的字符串
func splitCSV(s string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if i > start {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	return result
}