package client

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "computing-power/proto/v1"
)

// Config 客户端配置
type Config struct {
	Address string
	Timeout time.Duration
}

// Client 调度器 gRPC 客户端
type Client struct {
	conn   *grpc.ClientConn
	api    pb.SchedulerServiceClient
	config Config
}

// New 创建调度器客户端
func New(cfg Config) (*Client, error) {
	conn, err := grpc.NewClient(
		cfg.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()), // TODO(P6): 使用 mTLS
		grpc.WithDefaultCallOptions(grpc.ForceCodecV2(pb.JSONCodec{})),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to scheduler: %w", err)
	}
	return &Client{
		conn:   conn,
		api:    pb.NewSchedulerServiceClient(conn),
		config: cfg,
	}, nil
}

// Close 关闭连接
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// ctx 创建带超时的 context
func (c *Client) ctx(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := c.config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

// API 返回底层 gRPC API 客户端
func (c *Client) API() pb.SchedulerServiceClient {
	return c.api
}

// 便捷方法封装

func (c *Client) RegisterNode(ctx context.Context, req *pb.RegisterNodeRequest) (*pb.RegisterNodeResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.api.RegisterNode(ctx, req)
}

func (c *Client) SubmitJob(ctx context.Context, req *pb.SubmitJobRequest) (*pb.SubmitJobResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.api.SubmitJob(ctx, req)
}

func (c *Client) GetJob(ctx context.Context, req *pb.GetJobRequest) (*pb.GetJobResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.api.GetJob(ctx, req)
}

func (c *Client) ListJobs(ctx context.Context, req *pb.ListJobsRequest) (*pb.ListJobsResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.api.ListJobs(ctx, req)
}

func (c *Client) CancelJob(ctx context.Context, req *pb.CancelJobRequest) (*pb.CancelJobResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.api.CancelJob(ctx, req)
}

func (c *Client) ListNodes(ctx context.Context, req *pb.ListNodesRequest) (*pb.ListNodesResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.api.ListNodes(ctx, req)
}

func (c *Client) GetNode(ctx context.Context, req *pb.GetNodeRequest) (*pb.GetNodeResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.api.GetNode(ctx, req)
}

func (c *Client) DeclareTrust(ctx context.Context, req *pb.DeclareTrustRequest) (*pb.DeclareTrustResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.api.DeclareTrust(ctx, req)
}

func (c *Client) GetTrustGraph(ctx context.Context, req *pb.GetTrustGraphRequest) (*pb.GetTrustGraphResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.api.GetTrustGraph(ctx, req)
}

func (c *Client) RevokeTrust(ctx context.Context, req *pb.RevokeTrustRequest) (*pb.RevokeTrustResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.api.RevokeTrust(ctx, req)
}

func (c *Client) CreateInviteCode(ctx context.Context, req *pb.CreateInviteCodeRequest) (*pb.CreateInviteCodeResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.api.CreateInviteCode(ctx, req)
}

func (c *Client) RedeemInviteCode(ctx context.Context, req *pb.RedeemInviteCodeRequest) (*pb.RedeemInviteCodeResponse, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	return c.api.RedeemInviteCode(ctx, req)
}