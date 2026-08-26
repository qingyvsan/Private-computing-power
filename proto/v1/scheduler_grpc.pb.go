package v1

import (
	"context"

	"google.golang.org/grpc"
)

// SchedulerServiceServer 是调度器 gRPC 服务的接口定义
type SchedulerServiceServer interface {
	// 节点管理
	RegisterNode(ctx context.Context, req *RegisterNodeRequest) (*RegisterNodeResponse, error)
	Heartbeat(stream Scheduler_HeartbeatServer) error
	UnregisterNode(ctx context.Context, req *UnregisterNodeRequest) (*UnregisterNodeResponse, error)

	// 作业管理
	SubmitJob(ctx context.Context, req *SubmitJobRequest) (*SubmitJobResponse, error)
	CancelJob(ctx context.Context, req *CancelJobRequest) (*CancelJobResponse, error)
	GetJob(ctx context.Context, req *GetJobRequest) (*GetJobResponse, error)
	ListJobs(ctx context.Context, req *ListJobsRequest) (*ListJobsResponse, error)
	WatchJob(req *WatchJobRequest, stream Scheduler_WatchJobServer) error

	// 任务分配
	AssignUnit(ctx context.Context, req *AssignUnitRequest) (*AssignUnitResponse, error)
	ReportUnitStatus(stream Scheduler_ReportUnitStatusServer) error

	// 信任管理
	DeclareTrust(ctx context.Context, req *DeclareTrustRequest) (*DeclareTrustResponse, error)
	RevokeTrust(ctx context.Context, req *RevokeTrustRequest) (*RevokeTrustResponse, error)
	GetTrustGraph(ctx context.Context, req *GetTrustGraphRequest) (*GetTrustGraphResponse, error)

	// 证书管理
	IssueCertificate(ctx context.Context, req *IssueCertRequest) (*IssueCertResponse, error)
	RenewCertificate(ctx context.Context, req *RenewCertRequest) (*RenewCertResponse, error)
	RevokeCertificate(ctx context.Context, req *RevokeCertRequest) (*RevokeCertResponse, error)

	// 邀请码
	CreateInviteCode(ctx context.Context, req *CreateInviteCodeRequest) (*CreateInviteCodeResponse, error)
	RedeemInviteCode(ctx context.Context, req *RedeemInviteCodeRequest) (*RedeemInviteCodeResponse, error)

	// 节点查询
	ListNodes(ctx context.Context, req *ListNodesRequest) (*ListNodesResponse, error)
	GetNode(ctx context.Context, req *GetNodeRequest) (*GetNodeResponse, error)
}

// Scheduler_HeartbeatServer 心跳双向流接口
type Scheduler_HeartbeatServer interface {
	Send(*HeartbeatResponse) error
	Recv() (*HeartbeatRequest, error)
	grpc.ServerStream
}

// Scheduler_WatchJobServer 作业事件流接口
type Scheduler_WatchJobServer interface {
	Send(*JobEvent) error
	grpc.ServerStream
}

// Scheduler_ReportUnitStatusServer Unit 状态上报流接口
type Scheduler_ReportUnitStatusServer interface {
	Send(*UnitStatusAck) error
	Recv() (*UnitStatusReport, error)
	grpc.ServerStream
}

// Scheduler_HeartbeatClient 心跳双向流客户端接口
type Scheduler_HeartbeatClient interface {
	Send(*HeartbeatRequest) error
	Recv() (*HeartbeatResponse, error)
	grpc.ClientStream
}

// Scheduler_ReportUnitStatusClient Unit 状态上报客户端接口
type Scheduler_ReportUnitStatusClient interface {
	Send(*UnitStatusReport) error
	Recv() (*UnitStatusAck, error)
	grpc.ClientStream
}

// Scheduler_WatchJobClient 作业事件流客户端接口
type Scheduler_WatchJobClient interface {
	Recv() (*JobEvent, error)
	grpc.ClientStream
}

// RegisterSchedulerServiceServer 注册调度器服务到 gRPC 服务器
func RegisterSchedulerServiceServer(s grpc.ServiceRegistrar, srv SchedulerServiceServer) {
	s.RegisterService(&Scheduler_ServiceDesc, srv)
}

// Scheduler_ServiceDesc 调度器服务的 gRPC 描述
var Scheduler_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "computingpower.v1.Scheduler",
	HandlerType: (*SchedulerServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "RegisterNode",
			Handler:    _Scheduler_RegisterNode_Handler,
		},
		{
			MethodName: "UnregisterNode",
			Handler:    _Scheduler_UnregisterNode_Handler,
		},
		{
			MethodName: "SubmitJob",
			Handler:    _Scheduler_SubmitJob_Handler,
		},
		{
			MethodName: "CancelJob",
			Handler:    _Scheduler_CancelJob_Handler,
		},
		{
			MethodName: "GetJob",
			Handler:    _Scheduler_GetJob_Handler,
		},
		{
			MethodName: "ListJobs",
			Handler:    _Scheduler_ListJobs_Handler,
		},
		{
			MethodName: "AssignUnit",
			Handler:    _Scheduler_AssignUnit_Handler,
		},
		{
			MethodName: "DeclareTrust",
			Handler:    _Scheduler_DeclareTrust_Handler,
		},
		{
			MethodName: "RevokeTrust",
			Handler:    _Scheduler_RevokeTrust_Handler,
		},
		{
			MethodName: "GetTrustGraph",
			Handler:    _Scheduler_GetTrustGraph_Handler,
		},
		{
			MethodName: "IssueCertificate",
			Handler:    _Scheduler_IssueCertificate_Handler,
		},
		{
			MethodName: "RenewCertificate",
			Handler:    _Scheduler_RenewCertificate_Handler,
		},
		{
			MethodName: "RevokeCertificate",
			Handler:    _Scheduler_RevokeCertificate_Handler,
		},
		{
			MethodName: "CreateInviteCode",
			Handler:    _Scheduler_CreateInviteCode_Handler,
		},
		{
			MethodName: "RedeemInviteCode",
			Handler:    _Scheduler_RedeemInviteCode_Handler,
		},
		{
			MethodName: "ListNodes",
			Handler:    _Scheduler_ListNodes_Handler,
		},
		{
			MethodName: "GetNode",
			Handler:    _Scheduler_GetNode_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Heartbeat",
			Handler:       _Scheduler_Heartbeat_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "WatchJob",
			Handler:       _Scheduler_WatchJob_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "ReportUnitStatus",
			Handler:       _Scheduler_ReportUnitStatus_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "v1/scheduler.proto",
}

// Unary handlers

func _Scheduler_RegisterNode_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &RegisterNodeRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).RegisterNode(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/RegisterNode",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).RegisterNode(ctx, req.(*RegisterNodeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_UnregisterNode_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &UnregisterNodeRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).UnregisterNode(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/UnregisterNode",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).UnregisterNode(ctx, req.(*UnregisterNodeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_SubmitJob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &SubmitJobRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).SubmitJob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/SubmitJob",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).SubmitJob(ctx, req.(*SubmitJobRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_CancelJob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &CancelJobRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).CancelJob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/CancelJob",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).CancelJob(ctx, req.(*CancelJobRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_GetJob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &GetJobRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).GetJob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/GetJob",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).GetJob(ctx, req.(*GetJobRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_ListJobs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &ListJobsRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).ListJobs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/ListJobs",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).ListJobs(ctx, req.(*ListJobsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_AssignUnit_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &AssignUnitRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).AssignUnit(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/AssignUnit",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).AssignUnit(ctx, req.(*AssignUnitRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_DeclareTrust_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &DeclareTrustRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).DeclareTrust(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/DeclareTrust",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).DeclareTrust(ctx, req.(*DeclareTrustRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_RevokeTrust_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &RevokeTrustRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).RevokeTrust(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/RevokeTrust",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).RevokeTrust(ctx, req.(*RevokeTrustRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_GetTrustGraph_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &GetTrustGraphRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).GetTrustGraph(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/GetTrustGraph",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).GetTrustGraph(ctx, req.(*GetTrustGraphRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_IssueCertificate_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &IssueCertRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).IssueCertificate(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/IssueCertificate",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).IssueCertificate(ctx, req.(*IssueCertRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_RenewCertificate_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &RenewCertRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).RenewCertificate(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/RenewCertificate",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).RenewCertificate(ctx, req.(*RenewCertRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_RevokeCertificate_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &RevokeCertRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).RevokeCertificate(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/RevokeCertificate",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).RevokeCertificate(ctx, req.(*RevokeCertRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_CreateInviteCode_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &CreateInviteCodeRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).CreateInviteCode(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/CreateInviteCode",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).CreateInviteCode(ctx, req.(*CreateInviteCodeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_RedeemInviteCode_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &RedeemInviteCodeRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).RedeemInviteCode(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/RedeemInviteCode",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).RedeemInviteCode(ctx, req.(*RedeemInviteCodeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_ListNodes_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &ListNodesRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).ListNodes(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/ListNodes",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).ListNodes(ctx, req.(*ListNodesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_GetNode_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := &GetNodeRequest{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServiceServer).GetNode(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/computingpower.v1.Scheduler/GetNode",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServiceServer).GetNode(ctx, req.(*GetNodeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Stream handlers

func _Scheduler_Heartbeat_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(SchedulerServiceServer).Heartbeat(&schedulerHeartbeatServer{stream})
}

func _Scheduler_WatchJob_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := &WatchJobRequest{}
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(SchedulerServiceServer).WatchJob(m, &schedulerWatchJobServer{stream})
}

func _Scheduler_ReportUnitStatus_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(SchedulerServiceServer).ReportUnitStatus(&schedulerReportUnitStatusServer{stream})
}

// Stream server implementations

type schedulerHeartbeatServer struct {
	grpc.ServerStream
}

func (x *schedulerHeartbeatServer) Send(m *HeartbeatResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *schedulerHeartbeatServer) Recv() (*HeartbeatRequest, error) {
	m := &HeartbeatRequest{}
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

type schedulerWatchJobServer struct {
	grpc.ServerStream
}

func (x *schedulerWatchJobServer) Send(m *JobEvent) error {
	return x.ServerStream.SendMsg(m)
}

type schedulerReportUnitStatusServer struct {
	grpc.ServerStream
}

func (x *schedulerReportUnitStatusServer) Send(m *UnitStatusAck) error {
	return x.ServerStream.SendMsg(m)
}

func (x *schedulerReportUnitStatusServer) Recv() (*UnitStatusReport, error) {
	m := &UnitStatusReport{}
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// ========== Client-side helper functions ==========

// NewSchedulerServiceClient 创建调度器客户端
func NewSchedulerServiceClient(cc grpc.ClientConnInterface) SchedulerServiceClient {
	return &schedulerServiceClient{cc}
}

type SchedulerServiceClient interface {
	RegisterNode(ctx context.Context, in *RegisterNodeRequest, opts ...grpc.CallOption) (*RegisterNodeResponse, error)
	Heartbeat(ctx context.Context, opts ...grpc.CallOption) (Scheduler_HeartbeatClient, error)
	UnregisterNode(ctx context.Context, in *UnregisterNodeRequest, opts ...grpc.CallOption) (*UnregisterNodeResponse, error)
	SubmitJob(ctx context.Context, in *SubmitJobRequest, opts ...grpc.CallOption) (*SubmitJobResponse, error)
	CancelJob(ctx context.Context, in *CancelJobRequest, opts ...grpc.CallOption) (*CancelJobResponse, error)
	GetJob(ctx context.Context, in *GetJobRequest, opts ...grpc.CallOption) (*GetJobResponse, error)
	ListJobs(ctx context.Context, in *ListJobsRequest, opts ...grpc.CallOption) (*ListJobsResponse, error)
	WatchJob(ctx context.Context, in *WatchJobRequest, opts ...grpc.CallOption) (Scheduler_WatchJobClient, error)
	AssignUnit(ctx context.Context, in *AssignUnitRequest, opts ...grpc.CallOption) (*AssignUnitResponse, error)
	ReportUnitStatus(ctx context.Context, opts ...grpc.CallOption) (Scheduler_ReportUnitStatusClient, error)
	DeclareTrust(ctx context.Context, in *DeclareTrustRequest, opts ...grpc.CallOption) (*DeclareTrustResponse, error)
	RevokeTrust(ctx context.Context, in *RevokeTrustRequest, opts ...grpc.CallOption) (*RevokeTrustResponse, error)
	GetTrustGraph(ctx context.Context, in *GetTrustGraphRequest, opts ...grpc.CallOption) (*GetTrustGraphResponse, error)
	IssueCertificate(ctx context.Context, in *IssueCertRequest, opts ...grpc.CallOption) (*IssueCertResponse, error)
	RenewCertificate(ctx context.Context, in *RenewCertRequest, opts ...grpc.CallOption) (*RenewCertResponse, error)
	RevokeCertificate(ctx context.Context, in *RevokeCertRequest, opts ...grpc.CallOption) (*RevokeCertResponse, error)
	CreateInviteCode(ctx context.Context, in *CreateInviteCodeRequest, opts ...grpc.CallOption) (*CreateInviteCodeResponse, error)
	RedeemInviteCode(ctx context.Context, in *RedeemInviteCodeRequest, opts ...grpc.CallOption) (*RedeemInviteCodeResponse, error)
	ListNodes(ctx context.Context, in *ListNodesRequest, opts ...grpc.CallOption) (*ListNodesResponse, error)
	GetNode(ctx context.Context, in *GetNodeRequest, opts ...grpc.CallOption) (*GetNodeResponse, error)
}

type schedulerServiceClient struct {
	cc grpc.ClientConnInterface
}

func (c *schedulerServiceClient) RegisterNode(ctx context.Context, in *RegisterNodeRequest, opts ...grpc.CallOption) (*RegisterNodeResponse, error) {
	out := &RegisterNodeResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/RegisterNode", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) Heartbeat(ctx context.Context, opts ...grpc.CallOption) (Scheduler_HeartbeatClient, error) {
	stream, err := c.cc.NewStream(ctx, &Scheduler_ServiceDesc.Streams[0], "/computingpower.v1.Scheduler/Heartbeat", opts...)
	if err != nil {
		return nil, err
	}
	return &schedulerHeartbeatClient{stream}, nil
}

func (c *schedulerServiceClient) UnregisterNode(ctx context.Context, in *UnregisterNodeRequest, opts ...grpc.CallOption) (*UnregisterNodeResponse, error) {
	out := &UnregisterNodeResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/UnregisterNode", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) SubmitJob(ctx context.Context, in *SubmitJobRequest, opts ...grpc.CallOption) (*SubmitJobResponse, error) {
	out := &SubmitJobResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/SubmitJob", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) CancelJob(ctx context.Context, in *CancelJobRequest, opts ...grpc.CallOption) (*CancelJobResponse, error) {
	out := &CancelJobResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/CancelJob", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) GetJob(ctx context.Context, in *GetJobRequest, opts ...grpc.CallOption) (*GetJobResponse, error) {
	out := &GetJobResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/GetJob", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) ListJobs(ctx context.Context, in *ListJobsRequest, opts ...grpc.CallOption) (*ListJobsResponse, error) {
	out := &ListJobsResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/ListJobs", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) WatchJob(ctx context.Context, in *WatchJobRequest, opts ...grpc.CallOption) (Scheduler_WatchJobClient, error) {
	stream, err := c.cc.NewStream(ctx, &Scheduler_ServiceDesc.Streams[1], "/computingpower.v1.Scheduler/WatchJob", opts...)
	if err != nil {
		return nil, err
	}
	if err := stream.SendMsg(in); err != nil {
		return nil, err
	}
	return &schedulerWatchJobClient{stream}, nil
}

func (c *schedulerServiceClient) AssignUnit(ctx context.Context, in *AssignUnitRequest, opts ...grpc.CallOption) (*AssignUnitResponse, error) {
	out := &AssignUnitResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/AssignUnit", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) ReportUnitStatus(ctx context.Context, opts ...grpc.CallOption) (Scheduler_ReportUnitStatusClient, error) {
	stream, err := c.cc.NewStream(ctx, &Scheduler_ServiceDesc.Streams[2], "/computingpower.v1.Scheduler/ReportUnitStatus", opts...)
	if err != nil {
		return nil, err
	}
	return &schedulerReportUnitStatusClient{stream}, nil
}

func (c *schedulerServiceClient) DeclareTrust(ctx context.Context, in *DeclareTrustRequest, opts ...grpc.CallOption) (*DeclareTrustResponse, error) {
	out := &DeclareTrustResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/DeclareTrust", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) RevokeTrust(ctx context.Context, in *RevokeTrustRequest, opts ...grpc.CallOption) (*RevokeTrustResponse, error) {
	out := &RevokeTrustResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/RevokeTrust", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) GetTrustGraph(ctx context.Context, in *GetTrustGraphRequest, opts ...grpc.CallOption) (*GetTrustGraphResponse, error) {
	out := &GetTrustGraphResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/GetTrustGraph", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) IssueCertificate(ctx context.Context, in *IssueCertRequest, opts ...grpc.CallOption) (*IssueCertResponse, error) {
	out := &IssueCertResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/IssueCertificate", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) RenewCertificate(ctx context.Context, in *RenewCertRequest, opts ...grpc.CallOption) (*RenewCertResponse, error) {
	out := &RenewCertResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/RenewCertificate", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) RevokeCertificate(ctx context.Context, in *RevokeCertRequest, opts ...grpc.CallOption) (*RevokeCertResponse, error) {
	out := &RevokeCertResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/RevokeCertificate", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) CreateInviteCode(ctx context.Context, in *CreateInviteCodeRequest, opts ...grpc.CallOption) (*CreateInviteCodeResponse, error) {
	out := &CreateInviteCodeResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/CreateInviteCode", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) RedeemInviteCode(ctx context.Context, in *RedeemInviteCodeRequest, opts ...grpc.CallOption) (*RedeemInviteCodeResponse, error) {
	out := &RedeemInviteCodeResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/RedeemInviteCode", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) ListNodes(ctx context.Context, in *ListNodesRequest, opts ...grpc.CallOption) (*ListNodesResponse, error) {
	out := &ListNodesResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/ListNodes", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerServiceClient) GetNode(ctx context.Context, in *GetNodeRequest, opts ...grpc.CallOption) (*GetNodeResponse, error) {
	out := &GetNodeResponse{}
	err := c.cc.Invoke(ctx, "/computingpower.v1.Scheduler/GetNode", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Client stream implementations

type schedulerHeartbeatClient struct {
	grpc.ClientStream
}

func (x *schedulerHeartbeatClient) Send(m *HeartbeatRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *schedulerHeartbeatClient) Recv() (*HeartbeatResponse, error) {
	m := &HeartbeatResponse{}
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

type schedulerWatchJobClient struct {
	grpc.ClientStream
}

func (x *schedulerWatchJobClient) Recv() (*JobEvent, error) {
	m := &JobEvent{}
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

type schedulerReportUnitStatusClient struct {
	grpc.ClientStream
}

func (x *schedulerReportUnitStatusClient) Send(m *UnitStatusReport) error {
	return x.ClientStream.SendMsg(m)
}

func (x *schedulerReportUnitStatusClient) Recv() (*UnitStatusAck, error) {
	m := &UnitStatusAck{}
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}