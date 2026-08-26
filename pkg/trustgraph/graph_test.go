package trustgraph

import (
	"testing"
	"time"
)

func TestAddAndCheckTrust(t *testing.T) {
	g := NewGraph()
	if err := g.AddEdge("a", "b", []byte("sig"), nil); err != nil {
		t.Fatalf("add edge: %v", err)
	}

	if !g.HasTrust("a", "b") {
		t.Fatal("a should trust b")
	}
	if g.HasTrust("b", "a") {
		t.Fatal("b should NOT trust a (unidirectional)")
	}
}

func TestSelfTrustRejected(t *testing.T) {
	g := NewGraph()
	if err := g.AddEdge("a", "a", []byte("sig"), nil); err == nil {
		t.Fatal("self-trust should be rejected")
	}
}

func TestExpiredTrust(t *testing.T) {
	g := NewGraph()
	expiresAt := time.Now().Add(-1 * time.Hour)
	if err := g.AddEdge("a", "b", []byte("sig"), &expiresAt); err != nil {
		t.Fatalf("add edge: %v", err)
	}

	if g.HasTrust("a", "b") {
		t.Fatal("expired trust should not count")
	}
}

func TestReachability(t *testing.T) {
	g := NewGraph()
	// a -> b, b -> c
	g.AddEdge("a", "b", []byte("sig1"), nil)
	g.AddEdge("b", "c", []byte("sig2"), nil)

	if !g.IsReachable("a", "c", 10) {
		t.Fatal("a should reach c via b")
	}
	if g.IsReachable("c", "a", 10) {
		t.Fatal("c should NOT reach a (directed)")
	}
}

func TestPruneExpired(t *testing.T) {
	g := NewGraph()
	expired := time.Now().Add(-1 * time.Hour)
	valid := time.Now().Add(24 * time.Hour)

	g.AddEdge("a", "b", []byte("sig1"), &expired)
	g.AddEdge("a", "c", []byte("sig2"), &valid)

	count := g.PruneExpired()
	if count != 1 {
		t.Fatalf("expected 1 expired edge pruned, got %d", count)
	}
	if g.HasTrust("a", "b") {
		t.Fatal("expired edge should be removed")
	}
	if !g.HasTrust("a", "c") {
		t.Fatal("valid edge should remain")
	}
}

func TestSignAndVerify(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	sig, err := SignTrust(key, "node-a", "node-b")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	pubPEM, err := MarshalPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}

	// 验证正确的签名
	if err := VerifyTrust(pubPEM, "node-a", "node-b", sig); err != nil {
		t.Fatalf("verify should succeed: %v", err)
	}

	// 验证错误的签名（换节点）
	if err := VerifyTrust(pubPEM, "node-a", "node-c", sig); err == nil {
		t.Fatal("verify with wrong nodes should fail")
	}
}
