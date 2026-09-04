package ca

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestPaths(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "ca.crt"), filepath.Join(dir, "ca.key")
}

func TestNewManager_CreatesCA(t *testing.T) {
	certPath, keyPath := newTestPaths(t)
	m, err := NewManager(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if !m.IsCAValid() {
		t.Error("expected CA to be valid")
	}
	if err := m.EnsureCA(); err != nil {
		t.Errorf("EnsureCA: %v", err)
	}
	// Verify files were written
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Error("CA cert file was not created")
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("CA key file was not created")
	}
}

func TestNewManager_LoadsExistingCA(t *testing.T) {
	certPath, keyPath := newTestPaths(t)
	m1, err := NewManager(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewManager #1: %v", err)
	}

	// Load from same paths
	m2, err := NewManager(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewManager #2: %v", err)
	}
	if !m2.IsCAValid() {
		t.Error("expected loaded CA to be valid")
	}
	// Both CAs should have the same cert PEM
	if string(m1.CACertPEM()) != string(m2.CACertPEM()) {
		t.Error("loaded CA cert PEM differs from original")
	}
}

func TestNewManager_InvalidCertPath(t *testing.T) {
	_, err := NewManager("/nonexistent/dir/ca.crt", "/nonexistent/dir/ca.key", "test-org", time.Hour)
	if err != nil {
		t.Fatalf("NewManager should create dirs: %v", err)
	}
}

func TestIssueNodeCert(t *testing.T) {
	certPath, keyPath := newTestPaths(t)
	m, err := NewManager(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ips := []net.IP{net.ParseIP("10.1.0.2")}
	groups := []string{"test-group"}
	certPEM, keyPEM, err := m.IssueNodeCert("node-1", groups, ips)
	if err != nil {
		t.Fatalf("IssueNodeCert: %v", err)
	}

	// Verify cert PEM
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		t.Fatal("cert PEM decode failed")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	if cert.Subject.CommonName != "node-1" {
		t.Errorf("expected CN=node-1, got %s", cert.Subject.CommonName)
	}
	if len(cert.IPAddresses) != 1 || cert.IPAddresses[0].String() != "10.1.0.2" {
		t.Errorf("expected IP 10.1.0.2, got %v", cert.IPAddresses)
	}

	// Verify key PEM
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("key PEM decode failed")
	}

	// Verify cert is signed by CA
	caPEM := m.CACertPEM()
	caBlock, _ := pem.Decode(caPEM)
	if caBlock == nil {
		t.Fatal("CA cert PEM decode failed")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}
	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: caPool}); err != nil {
		t.Errorf("cert not signed by CA: %v", err)
	}
}

func TestIssueNodeCert_NoIPNoGroups(t *testing.T) {
	certPath, keyPath := newTestPaths(t)
	m, err := NewManager(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	certPEM, keyPEM, err := m.IssueNodeCert("node-2", nil, nil)
	if err != nil {
		t.Fatalf("IssueNodeCert: %v", err)
	}
	if len(certPEM) == 0 {
		t.Error("expected non-empty cert PEM")
	}
	if len(keyPEM) == 0 {
		t.Error("expected non-empty key PEM")
	}
}

func TestCACertPEM(t *testing.T) {
	certPath, keyPath := newTestPaths(t)
	m, err := NewManager(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	pemData := m.CACertPEM()
	if len(pemData) == 0 {
		t.Fatal("expected non-empty CA cert PEM")
	}
	block, _ := pem.Decode(pemData)
	if block == nil {
		t.Fatal("CA cert PEM decode failed")
	}
	_, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}
}

func TestIsCAValid_Uninitialized(t *testing.T) {
	m := &Manager{}
	if m.IsCAValid() {
		t.Error("expected uninitialized CA to be invalid")
	}
}

func TestEnsureCA_Uninitialized(t *testing.T) {
	m := &Manager{}
	if err := m.EnsureCA(); err == nil {
		t.Error("expected error for uninitialized CA")
	}
}

func TestGetCA(t *testing.T) {
	certPath, keyPath := newTestPaths(t)
	m, err := NewManager(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ca := m.GetCA()
	if ca == nil {
		t.Fatal("GetCA returned nil")
	}
	if ca.Certificate == nil {
		t.Error("CA certificate is nil")
	}
	if ca.PrivateKey == nil {
		t.Error("CA private key is nil")
	}
}

// ========== GRPCCA tests ==========

func TestNewGRPCCA_CreatesCA(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "grpc-ca.crt")
	keyPath := filepath.Join(dir, "grpc-ca.key")
	g, err := NewGRPCCA(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewGRPCCA: %v", err)
	}
	if !g.IsCAValid() {
		t.Error("expected CA to be valid")
	}
	if len(g.CACertPEM()) == 0 {
		t.Error("expected non-empty CA cert PEM")
	}
}

func TestNewGRPCCA_LoadsExistingCA(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "grpc-ca.crt")
	keyPath := filepath.Join(dir, "grpc-ca.key")

	g1, err := NewGRPCCA(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewGRPCCA #1: %v", err)
	}

	g2, err := NewGRPCCA(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewGRPCCA #2: %v", err)
	}
	if string(g1.CACertPEM()) != string(g2.CACertPEM()) {
		t.Error("loaded CA cert PEM differs from original")
	}
}

func TestGenerateServerCert(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "grpc-ca.crt")
	keyPath := filepath.Join(dir, "grpc-ca.key")
	g, err := NewGRPCCA(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewGRPCCA: %v", err)
	}

	certPEM, keyPEM, err := g.GenerateServerCert()
	if err != nil {
		t.Fatalf("GenerateServerCert: %v", err)
	}
	if len(certPEM) == 0 {
		t.Error("expected non-empty server cert PEM")
	}
	if len(keyPEM) == 0 {
		t.Error("expected non-empty server key PEM")
	}

	// Verify cert CN
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("cert PEM decode failed")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	if cert.Subject.CommonName != "scheduler" {
		t.Errorf("expected CN=scheduler, got %s", cert.Subject.CommonName)
	}
}

func TestIssueClientCert(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "grpc-ca.crt")
	keyPath := filepath.Join(dir, "grpc-ca.key")
	g, err := NewGRPCCA(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewGRPCCA: %v", err)
	}

	certPEM, keyPEM, err := g.IssueClientCert("node-42")
	if err != nil {
		t.Fatalf("IssueClientCert: %v", err)
	}
	if len(certPEM) == 0 {
		t.Error("expected non-empty client cert PEM")
	}
	if len(keyPEM) == 0 {
		t.Error("expected non-empty client key PEM")
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("cert PEM decode failed")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	if cert.Subject.CommonName != "node-42" {
		t.Errorf("expected CN=node-42, got %s", cert.Subject.CommonName)
	}
}

func TestServerTLSConfig(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "grpc-ca.crt")
	keyPath := filepath.Join(dir, "grpc-ca.key")
	g, err := NewGRPCCA(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewGRPCCA: %v", err)
	}

	serverCert, serverKey, err := g.GenerateServerCert()
	if err != nil {
		t.Fatalf("GenerateServerCert: %v", err)
	}

	tlsCfg, err := g.ServerTLSConfig(serverCert, serverKey)
	if err != nil {
		t.Fatalf("ServerTLSConfig: %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Errorf("expected 1 server cert, got %d", len(tlsCfg.Certificates))
	}
	if tlsCfg.MinVersion != 0x0303 { // TLS 1.2
		t.Errorf("expected MinVersion TLS 1.2, got %d", tlsCfg.MinVersion)
	}
}

func TestClientTLSConfig(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "grpc-ca.crt")
	keyPath := filepath.Join(dir, "grpc-ca.key")
	g, err := NewGRPCCA(certPath, keyPath, "test-org", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("NewGRPCCA: %v", err)
	}

	// Without client cert (bootstrap mode)
	tlsCfg, err := g.ClientTLSConfig("scheduler", g.CACertPEM(), nil, nil)
	if err != nil {
		t.Fatalf("ClientTLSConfig (bootstrap): %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if len(tlsCfg.Certificates) != 0 {
		t.Error("expected no client cert in bootstrap mode")
	}

	// With client cert (mTLS mode)
	clientCert, clientKey, err := g.IssueClientCert("test-node")
	if err != nil {
		t.Fatalf("IssueClientCert: %v", err)
	}
	tlsCfg, err = g.ClientTLSConfig("scheduler", g.CACertPEM(), clientCert, clientKey)
	if err != nil {
		t.Fatalf("ClientTLSConfig (mTLS): %v", err)
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Errorf("expected 1 client cert, got %d", len(tlsCfg.Certificates))
	}
}

func TestGRPCCA_IsCAValid_Uninitialized(t *testing.T) {
	g := &GRPCCA{}
	if g.IsCAValid() {
		t.Error("expected uninitialized GRPCCA to be invalid")
	}
}