package ca

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"computing-power/pkg/certutil"
)

// GRPCCA 管理 gRPC mTLS 证书生命周期
// 与 Nebula CA 独立，使用 cfg.TLS.CACert/CAKey 路径
type GRPCCA struct {
	ca       *certutil.CA
	org      string
	validity time.Duration
}

// NewGRPCCA 创建或加载 gRPC CA
// caCertPath/caKeyPath 存在则加载，否则生成新 CA
// validity: 签发的节点证书有效期
func NewGRPCCA(caCertPath, caKeyPath, org string, validity time.Duration) (*GRPCCA, error) {
	var ca *certutil.CA

	// 尝试加载现有 CA
	if _, err := os.Stat(caCertPath); err == nil {
		if _, err := os.Stat(caKeyPath); err == nil {
			certPEM, err := os.ReadFile(caCertPath)
			if err != nil {
				return nil, fmt.Errorf("read CA cert: %w", err)
			}
			keyPEM, err := os.ReadFile(caKeyPath)
			if err != nil {
				return nil, fmt.Errorf("read CA key: %w", err)
			}
			ca, err = certutil.LoadCAFromPEM(certPEM, keyPEM)
			if err != nil {
				return nil, fmt.Errorf("load CA from PEM: %w", err)
			}
			return &GRPCCA{ca: ca, org: org, validity: validity}, nil
		}
	}

	// 生成新 CA（10 年有效期）
	ca, err := certutil.GenerateCA(org, 10*365*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("generate CA: %w", err)
	}

	// 持久化 CA 证书和密钥
	dir := filepath.Dir(caCertPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create CA dir: %w", err)
	}
	certPEM, keyPEM, err := ca.MarshalCA()
	if err != nil {
		return nil, fmt.Errorf("marshal CA: %w", err)
	}
	if err := os.WriteFile(caCertPath, certPEM, 0644); err != nil {
		return nil, fmt.Errorf("write CA cert: %w", err)
	}
	if err := os.WriteFile(caKeyPath, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("write CA key: %w", err)
	}

	return &GRPCCA{ca: ca, org: org, validity: validity}, nil
}

// GenerateServerCert 生成 gRPC 服务器证书
// 返回 certPEM, keyPEM, error
// 证书仅包含 ServerAuth EKU，不包含 ClientAuth
func (g *GRPCCA) GenerateServerCert() ([]byte, []byte, error) {
	ips := []net.IP{net.ParseIP("0.0.0.0")}
	dnsNames := []string{"localhost", "scheduler"}
	cert, key, err := g.ca.IssueCert("scheduler", g.org, ips, dnsNames, nil, g.validity)
	if err != nil {
		return nil, nil, fmt.Errorf("issue server cert: %w", err)
	}
	certPEM := certutil.EncodeCertToPEM(cert)
	keyPEM, err := certutil.EncodeKeyToPEM(key)
	if err != nil {
		return nil, nil, fmt.Errorf("encode server key: %w", err)
	}
	return certPEM, keyPEM, nil
}

// IssueClientCert 签发 gRPC 客户端证书
// nodeID 用作 CommonName，用于身份识别
// 返回 certPEM, keyPEM, error
func (g *GRPCCA) IssueClientCert(nodeID string) ([]byte, []byte, error) {
	cert, key, err := g.ca.IssueCert(nodeID, g.org, nil, nil, nil, g.validity)
	if err != nil {
		return nil, nil, fmt.Errorf("issue client cert for %s: %w", nodeID, err)
	}
	certPEM := certutil.EncodeCertToPEM(cert)
	keyPEM, err := certutil.EncodeKeyToPEM(key)
	if err != nil {
		return nil, nil, fmt.Errorf("encode client key: %w", err)
	}
	return certPEM, keyPEM, nil
}

// CACertPEM 返回 CA 证书的 PEM 编码
func (g *GRPCCA) CACertPEM() []byte {
	return certutil.EncodeCertToPEM(g.ca.Certificate)
}

// ServerTLSConfig 构建服务器端 TLS 配置
// 使用 VerifyClientCertIfGiven：允许无客户端证书的连接（用于首次注册）
func (g *GRPCCA) ServerTLSConfig(serverCertPEM, serverKeyPEM []byte) (*tls.Config, error) {
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("load server key pair: %w", err)
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(g.CACertPEM())

	return &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    caPool,
		ClientAuth:   tls.VerifyClientCertIfGiven,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// ClientTLSConfig 构建客户端 TLS 配置
// 如果 clientCertPEM/clientKeyPEM 为空，则不提供客户端证书（引导模式）
// serverName 用于验证服务器证书主机名
func (g *GRPCCA) ClientTLSConfig(serverName string, caCertPEM, clientCertPEM, clientKeyPEM []byte) (*tls.Config, error) {
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCertPEM)

	cfg := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
		ServerName: serverName,
	}

	if len(clientCertPEM) > 0 && len(clientKeyPEM) > 0 {
		clientCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("load client key pair: %w", err)
		}
		cfg.Certificates = []tls.Certificate{clientCert}
	}

	return cfg, nil
}

// IsCAValid 检查 CA 是否有效
func (g *GRPCCA) IsCAValid() bool {
	if g.ca == nil || g.ca.Certificate == nil {
		return false
	}
	_, err := x509.ParseCertificate(g.ca.Certificate.Raw)
	return err == nil
}