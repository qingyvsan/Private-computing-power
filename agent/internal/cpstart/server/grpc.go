package server

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	pb "computing-power/proto/v1"
)

// Bridge gRPC 连接管理器
type Bridge struct {
	addr   string
	creds  credentials.TransportCredentials
	conn   *grpc.ClientConn
	client pb.SchedulerServiceClient
	mu     sync.RWMutex
}

// NewBridge 创建 gRPC 桥接
// creds 为传输层凭证，传 nil 则使用不安全连接
func NewBridge(addr string, creds credentials.TransportCredentials) *Bridge {
	if creds == nil {
		creds = insecure.NewCredentials()
	}
	return &Bridge{addr: addr, creds: creds}
}

// connect 建立 gRPC 连接（延迟连接）
func (b *Bridge) connect() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.conn != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, b.addr,
		grpc.WithTransportCredentials(b.creds),
		grpc.WithDefaultCallOptions(grpc.ForceCodecV2(pb.JSONCodec{})),
		grpc.WithBlock(),
	)
	if err != nil {
		return err
	}
	b.conn = conn
	b.client = pb.NewSchedulerServiceClient(conn)
	return nil
}

// Close 关闭连接
func (b *Bridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
		b.client = nil
	}
}

// ResetAddr 更新调度器地址并重置连接
func (b *Bridge) ResetAddr(addr string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
		b.client = nil
	}
	b.addr = addr
}

// Client 返回 gRPC 客户端（延迟连接）
func (b *Bridge) Client() (pb.SchedulerServiceClient, error) {
	b.mu.RLock()
	if b.client != nil {
		defer b.mu.RUnlock()
		return b.client, nil
	}
	b.mu.RUnlock()

	if err := b.connect(); err != nil {
		return nil, err
	}

	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.client, nil
}

// Unary 便捷方法：执行一元 gRPC 调用
func Unary[T any](bridge *Bridge, fn func(pb.SchedulerServiceClient) (T, error)) (T, error) {
	client, err := bridge.Client()
	if err != nil {
		var zero T
		return zero, err
	}
	return fn(client)
}