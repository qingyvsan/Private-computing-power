package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
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
	eventBus *events.Bus
	engine   *scheduler.Engine
	stopCh   chan struct{}
	ca       *ca.Manager
	ipam     *ipam.IPAM

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
		eventBus:          events.NewBus(),
		engine:            eng,
		stopCh:            make(chan struct{}),
		ca:                caMgr,
		ipam:              ipamMgr,
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

	// 启动定期故障检测循环（2 倍心跳间隔）
	checkInterval := s.heartbeatInterval * 2
	if checkInterval < time.Second {
		checkInterval = 5 * time.Second
	}
	s.registry.Start(ctx.Done(), checkInterval)

	// 启动调度引擎
	s.engine.Start(ctx)

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
		ID:           nodeID,
		Name:         req.Name,
		OverlayIP:    overlayIP,
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
	if err := s.ipam.Release(req.NodeID); err != nil {
		log.Printf("ipam release for %s: %v", req.NodeID, err)
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