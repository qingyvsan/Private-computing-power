package server

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// contextKey 用于在上下文中存储认证信息
type contextKey string

// AuthenticatedNodeIDKey 从客户端证书中提取的节点 ID
const AuthenticatedNodeIDKey contextKey = "authenticated_node_id"

// registerNodeMethods 允许无客户端证书通过的 RPC 方法
var registerNodeMethods = map[string]bool{
	"/computingpower.v1.Scheduler/RegisterNode": true,
}

// UnaryMTLSInterceptor 一元 mTLS 拦截器
// RegisterNode 放行；其余 RPC 强制要求有效的客户端证书
func UnaryMTLSInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// RegisterNode 允许无客户端证书
		if registerNodeMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		// 从 TLS 状态提取客户端证书
		nodeID, err := extractNodeIDFromTLS(ctx)
		if err != nil {
			return nil, err
		}

		// 将节点 ID 注入上下文
		ctx = context.WithValue(ctx, AuthenticatedNodeIDKey, nodeID)
		return handler(ctx, req)
	}
}

// StreamMTLSInterceptor 流式 mTLS 拦截器
// Heartbeat 流同样需要客户端证书
func StreamMTLSInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		nodeID, err := extractNodeIDFromTLS(stream.Context())
		if err != nil {
			return err
		}

		ctx := context.WithValue(stream.Context(), AuthenticatedNodeIDKey, nodeID)
		wrapped := &wrappedServerStream{ServerStream: stream, ctx: ctx}
		return handler(srv, wrapped)
	}
}

// extractNodeIDFromTLS 从 TLS 连接中提取客户端证书的 CommonName
func extractNodeIDFromTLS(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", status.Errorf(codes.Unauthenticated, "no peer information")
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", status.Errorf(codes.Unauthenticated, "TLS required")
	}

	if len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return "", status.Errorf(codes.Unauthenticated, "client certificate required")
	}

	clientCert := tlsInfo.State.VerifiedChains[0][0]
	nodeID := clientCert.Subject.CommonName
	if nodeID == "" {
		return "", status.Errorf(codes.Unauthenticated, "client certificate missing CommonName")
	}

	return nodeID, nil
}

// GetAuthenticatedNodeID 从上下文中提取经过认证的节点 ID
func GetAuthenticatedNodeID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(AuthenticatedNodeIDKey).(string)
	return id, ok
}

// wrappedServerStream 包装 grpc.ServerStream 以覆盖 Context
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}