package server

import (
	"context"
	"testing"
	"time"

	pb "computing-power/proto/v1"
	"computing-power/pkg/trustgraph"
)

// registerTestNode 注册一个测试节点到 registry 和 store
func registerTestNode(t *testing.T, srv *Server, id string, pubKey []byte) {
	t.Helper()
	node := &pb.Node{
		ID:           id,
		PublicKey:    pubKey,
		Status:       pb.NodeStatusOnline,
		Discoverable: "trust_only",
		Reputation:   1.0,
		MaxTasks:     10,
	}
	srv.registry.Register(node)
	if err := srv.store.SaveNode(node); err != nil {
		t.Fatalf("save node %s: %v", id, err)
	}
}

func TestDeclareTrust_Success(t *testing.T) {
	srv := newTestServer(t)

	// 生成密钥对并注册节点
	key, err := trustgraph.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pubPEM, _ := trustgraph.MarshalPublicKey(&key.PublicKey)
	registerTestNode(t, srv, "node-a", pubPEM)

	// 签名信任声明
	sig, err := trustgraph.SignTrust(key, "node-a", "node-b")
	if err != nil {
		t.Fatal(err)
	}

	resp, err := srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "node-b",
		Signature:    sig,
	})
	if err != nil {
		t.Fatalf("DeclareTrust: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success")
	}

	// 验证内存图
	if !srv.trust.HasTrust("node-a", "node-b") {
		t.Fatal("trust edge should exist in graph")
	}

	// 验证持久化
	edges, err := srv.store.ListTrustEdges()
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge in store, got %d", len(edges))
	}
	if edges[0].FromNode != "node-a" || edges[0].ToNode != "node-b" {
		t.Fatalf("unexpected edge: %s -> %s", edges[0].FromNode, edges[0].ToNode)
	}

	// 验证信任可达
	if !srv.trust.IsReachable("node-a", "node-b", 10) {
		t.Fatal("node-a should be reachable to node-b")
	}
}

func TestDeclareTrust_MissingFromNodeID(t *testing.T) {
	srv := newTestServer(t)
	_, err := srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		TargetNodeID: "node-b",
	})
	if err == nil {
		t.Fatal("expected error for missing from_node_id")
	}
}

func TestDeclareTrust_MissingTargetNodeID(t *testing.T) {
	srv := newTestServer(t)
	_, err := srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		FromNodeID: "node-a",
	})
	if err == nil {
		t.Fatal("expected error for missing target_node_id")
	}
}

func TestDeclareTrust_SelfTrust(t *testing.T) {
	srv := newTestServer(t)
	key, _ := trustgraph.GenerateKey()
	pubPEM, _ := trustgraph.MarshalPublicKey(&key.PublicKey)
	registerTestNode(t, srv, "node-a", pubPEM)

	sig, _ := trustgraph.SignTrust(key, "node-a", "node-a")
	_, err := srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "node-a",
		Signature:    sig,
	})
	if err == nil {
		t.Fatal("expected error for self-trust")
	}
}

func TestDeclareTrust_NodeNotFound(t *testing.T) {
	srv := newTestServer(t)
	_, err := srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		FromNodeID:   "nonexistent",
		TargetNodeID: "node-b",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent node")
	}
}

func TestDeclareTrust_NoPublicKey(t *testing.T) {
	srv := newTestServer(t)
	// 注册节点但不带公钥
	registerTestNode(t, srv, "node-a", nil)

	_, err := srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "node-b",
	})
	if err == nil {
		t.Fatal("expected error for missing public key")
	}
}

func TestDeclareTrust_BadSignature(t *testing.T) {
	srv := newTestServer(t)
	key, _ := trustgraph.GenerateKey()
	pubPEM, _ := trustgraph.MarshalPublicKey(&key.PublicKey)
	registerTestNode(t, srv, "node-a", pubPEM)

	// 用错误的密钥签名
	wrongKey, _ := trustgraph.GenerateKey()
	sig, _ := trustgraph.SignTrust(wrongKey, "node-a", "node-b")

	_, err := srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "node-b",
		Signature:    sig,
	})
	if err == nil {
		t.Fatal("expected error for bad signature")
	}
}

func TestDeclareTrust_WithExpiry(t *testing.T) {
	srv := newTestServer(t)
	key, _ := trustgraph.GenerateKey()
	pubPEM, _ := trustgraph.MarshalPublicKey(&key.PublicKey)
	registerTestNode(t, srv, "node-a", pubPEM)

	sig, _ := trustgraph.SignTrust(key, "node-a", "node-b")
	expiresAt := time.Now().Add(24 * time.Hour).UnixMilli()

	_, err := srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "node-b",
		Signature:    sig,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		t.Fatalf("DeclareTrust: %v", err)
	}

	// 过期前应可达
	if !srv.trust.HasTrust("node-a", "node-b") {
		t.Fatal("trust should be valid before expiry")
	}
}

func TestRevokeTrust_Success(t *testing.T) {
	srv := newTestServer(t)
	key, _ := trustgraph.GenerateKey()
	pubPEM, _ := trustgraph.MarshalPublicKey(&key.PublicKey)
	registerTestNode(t, srv, "node-a", pubPEM)

	// 先声明信任
	sig, _ := trustgraph.SignTrust(key, "node-a", "node-b")
	_, err := srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "node-b",
		Signature:    sig,
	})
	if err != nil {
		t.Fatalf("DeclareTrust: %v", err)
	}

	// 撤销信任
	sig2, _ := trustgraph.SignTrust(key, "node-a", "node-b")
	resp, err := srv.RevokeTrust(context.Background(), &pb.RevokeTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "node-b",
		Signature:    sig2,
	})
	if err != nil {
		t.Fatalf("RevokeTrust: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success")
	}

	// 验证边已移除
	if srv.trust.HasTrust("node-a", "node-b") {
		t.Fatal("trust edge should be removed from graph")
	}

	// 验证持久化已删除
	edges, _ := srv.store.ListTrustEdges()
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges in store, got %d", len(edges))
	}
}

func TestRevokeTrust_Idempotent(t *testing.T) {
	srv := newTestServer(t)
	key, _ := trustgraph.GenerateKey()
	pubPEM, _ := trustgraph.MarshalPublicKey(&key.PublicKey)
	registerTestNode(t, srv, "node-a", pubPEM)

	// 撤销不存在的边
	sig, _ := trustgraph.SignTrust(key, "node-a", "nonexistent")
	resp, err := srv.RevokeTrust(context.Background(), &pb.RevokeTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "nonexistent",
		Signature:    sig,
	})
	if err != nil {
		t.Fatalf("RevokeTrust: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success for idempotent revoke")
	}
}

func TestRevokeTrust_MissingFields(t *testing.T) {
	srv := newTestServer(t)
	// 缺少 FromNodeID
	_, err := srv.RevokeTrust(context.Background(), &pb.RevokeTrustRequest{
		TargetNodeID: "node-b",
	})
	if err == nil {
		t.Fatal("expected error for missing from_node_id")
	}
}

func TestRevokeTrust_BadSignature(t *testing.T) {
	srv := newTestServer(t)
	key, _ := trustgraph.GenerateKey()
	pubPEM, _ := trustgraph.MarshalPublicKey(&key.PublicKey)
	registerTestNode(t, srv, "node-a", pubPEM)

	wrongKey, _ := trustgraph.GenerateKey()
	sig, _ := trustgraph.SignTrust(wrongKey, "node-a", "node-b")
	_, err := srv.RevokeTrust(context.Background(), &pb.RevokeTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "node-b",
		Signature:    sig,
	})
	if err == nil {
		t.Fatal("expected error for bad signature")
	}
}

func TestGetTrustGraph_AfterDeclare(t *testing.T) {
	srv := newTestServer(t)
	key, _ := trustgraph.GenerateKey()
	pubPEM, _ := trustgraph.MarshalPublicKey(&key.PublicKey)
	registerTestNode(t, srv, "node-a", pubPEM)

	sig, _ := trustgraph.SignTrust(key, "node-a", "node-b")
	_, err := srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "node-b",
		Signature:    sig,
	})
	if err != nil {
		t.Fatalf("DeclareTrust: %v", err)
	}

	resp, err := srv.GetTrustGraph(context.Background(), &pb.GetTrustGraphRequest{})
	if err != nil {
		t.Fatalf("GetTrustGraph: %v", err)
	}
	if len(resp.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(resp.Edges))
	}
	if resp.Edges[0].FromNode != "node-a" || resp.Edges[0].ToNode != "node-b" {
		t.Fatalf("unexpected edge: %s -> %s", resp.Edges[0].FromNode, resp.Edges[0].ToNode)
	}
}

func TestListNodes_Visibility_Public(t *testing.T) {
	srv := newTestServer(t)
	node := &pb.Node{
		ID:           "node-public",
		Status:       pb.NodeStatusOnline,
		Discoverable: "public",
	}
	srv.registry.Register(node)

	resp, err := srv.ListNodes(context.Background(), &pb.ListNodesRequest{
		RequesterID: "other-node",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(resp.Nodes))
	}
}

func TestListNodes_Visibility_TrustOnly_NotTrusted(t *testing.T) {
	srv := newTestServer(t)
	node := &pb.Node{
		ID:           "node-trusted",
		Status:       pb.NodeStatusOnline,
		Discoverable: "trust_only",
	}
	srv.registry.Register(node)

	// 非信任节点不可见
	resp, err := srv.ListNodes(context.Background(), &pb.ListNodesRequest{
		RequesterID: "other-node",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 0 {
		t.Fatalf("expected 0 nodes (not trusted), got %d", len(resp.Nodes))
	}
}

func TestListNodes_Visibility_TrustOnly_Trusted(t *testing.T) {
	srv := newTestServer(t)
	key, _ := trustgraph.GenerateKey()
	pubPEM, _ := trustgraph.MarshalPublicKey(&key.PublicKey)
	registerTestNode(t, srv, "node-a", pubPEM)

	node := &pb.Node{
		ID:           "node-b",
		Status:       pb.NodeStatusOnline,
		Discoverable: "trust_only",
	}
	srv.registry.Register(node)

	// 建立信任关系
	sig, _ := trustgraph.SignTrust(key, "node-a", "node-b")
	_, err := srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "node-b",
		Signature:    sig,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 信任后应可见（node-a 自身 + node-b 信任可达 = 2 个）
	resp, err := srv.ListNodes(context.Background(), &pb.ListNodesRequest{
		RequesterID: "node-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 2 {
		t.Fatalf("expected 2 nodes (self + trusted), got %d", len(resp.Nodes))
	}
}

func TestListNodes_Visibility_Self(t *testing.T) {
	srv := newTestServer(t)
	node := &pb.Node{
		ID:           "node-self",
		Status:       pb.NodeStatusOnline,
		Discoverable: "trust_only",
	}
	srv.registry.Register(node)

	// 自身始终可见
	resp, err := srv.ListNodes(context.Background(), &pb.ListNodesRequest{
		RequesterID: "node-self",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 node (self), got %d", len(resp.Nodes))
	}
}

func TestListNodes_Visibility_Hidden(t *testing.T) {
	srv := newTestServer(t)
	key, _ := trustgraph.GenerateKey()
	pubPEM, _ := trustgraph.MarshalPublicKey(&key.PublicKey)
	registerTestNode(t, srv, "node-a", pubPEM)

	node := &pb.Node{
		ID:           "node-hidden",
		Status:       pb.NodeStatusOnline,
		Discoverable: "hidden",
	}
	srv.registry.Register(node)

	// 未建立信任时不可见（node-a 自身仍可见 = 1 个）
	resp, err := srv.ListNodes(context.Background(), &pb.ListNodesRequest{
		RequesterID: "node-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 node (self only), got %d", len(resp.Nodes))
	}

	// 建立直接信任后可见
	sig, _ := trustgraph.SignTrust(key, "node-a", "node-hidden")
	_, err = srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "node-hidden",
		Signature:    sig,
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err = srv.ListNodes(context.Background(), &pb.ListNodesRequest{
		RequesterID: "node-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 2 {
		t.Fatalf("expected 2 nodes (self + hidden-but-trusted), got %d", len(resp.Nodes))
	}
}

func TestGetNode_Visibility(t *testing.T) {
	srv := newTestServer(t)
	node := &pb.Node{
		ID:           "node-secret",
		Status:       pb.NodeStatusOnline,
		Discoverable: "trust_only",
	}
	srv.registry.Register(node)

	// 非信任请求者不可见
	_, err := srv.GetNode(context.Background(), &pb.GetNodeRequest{
		NodeID:      "node-secret",
		RequesterID: "other-node",
	})
	if err == nil {
		t.Fatal("expected error for non-trusted requester")
	}

	// 自身可见
	resp, err := srv.GetNode(context.Background(), &pb.GetNodeRequest{
		NodeID:      "node-secret",
		RequesterID: "node-secret",
	})
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if resp.Node.ID != "node-secret" {
		t.Fatalf("unexpected node: %s", resp.Node.ID)
	}
}

func TestTrustExpiry_PruneExpired(t *testing.T) {
	srv := newTestServer(t)
	key, _ := trustgraph.GenerateKey()
	pubPEM, _ := trustgraph.MarshalPublicKey(&key.PublicKey)
	registerTestNode(t, srv, "node-a", pubPEM)

	// 添加一个已过期的信任边
	expiresAt := time.Now().Add(-1 * time.Hour).UnixMilli()
	sig, _ := trustgraph.SignTrust(key, "node-a", "node-b")
	_, err := srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "node-b",
		Signature:    sig,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		t.Fatalf("DeclareTrust: %v", err)
	}

	// 添加一个未过期的边
	sig2, _ := trustgraph.SignTrust(key, "node-a", "node-c")
	_, err = srv.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
		FromNodeID:   "node-a",
		TargetNodeID: "node-c",
		Signature:    sig2,
		ExpiresAt:    time.Now().Add(24 * time.Hour).UnixMilli(),
	})
	if err != nil {
		t.Fatalf("DeclareTrust: %v", err)
	}

	// 验证过期前有 2 条边
	if srv.trust.Size() != 2 {
		t.Fatalf("expected 2 edges before prune, got %d", srv.trust.Size())
	}

	// 执行清理
	srv.pruneExpiredTrust()

	// 过期边应该被清理，剩下 1 条
	if srv.trust.Size() != 1 {
		t.Fatalf("expected 1 edge after prune, got %d", srv.trust.Size())
	}

	// 验证可达性
	if srv.trust.HasTrust("node-a", "node-b") {
		t.Fatal("expired edge should be removed")
	}
	if !srv.trust.HasTrust("node-a", "node-c") {
		t.Fatal("non-expired edge should remain")
	}
}