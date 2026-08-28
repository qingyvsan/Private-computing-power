package ca

import (
	"crypto/ecdsa"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"computing-power/pkg/certutil"
)

// Manager 管理 Nebula CA 和节点证书签发
type Manager struct {
	ca      *certutil.CA
	org     string
	validity time.Duration
}

// NewManager 创建或加载 Nebula CA
// 如果 caCertPath 和 caKeyPath 存在则加载，否则生成新 CA
// org: CA 组织名称
// validity: 签发的节点证书有效期
func NewManager(caCertPath, caKeyPath, org string, validity time.Duration) (*Manager, error) {
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
			return &Manager{ca: ca, org: org, validity: validity}, nil
		}
	}

	// 生成新 CA
	ca, err := certutil.GenerateCA(org, 10*365*24*time.Hour) // 10 年有效期
	if err != nil {
		return nil, fmt.Errorf("generate CA: %w", err)
	}

	// 保存 CA 证书和密钥
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

	return &Manager{ca: ca, org: org, validity: validity}, nil
}

// IssueNodeCert 签发 Nebula 节点证书
// nodeID: 节点标识（用作 CommonName）
// groups: Nebula 分组列表
// ips: 节点 Overlay IP 列表
// 返回 certPEM, keyPEM, error
func (m *Manager) IssueNodeCert(nodeID string, groups []string, ips []net.IP) ([]byte, []byte, error) {
	cert, key, err := m.ca.IssueCert(nodeID, m.org, ips, nil, groups, m.validity)
	if err != nil {
		return nil, nil, fmt.Errorf("issue cert for %s: %w", nodeID, err)
	}

	certPEM := certutil.EncodeCertToPEM(cert)
	keyPEM, err := certutil.EncodeKeyToPEM(key)
	if err != nil {
		return nil, nil, fmt.Errorf("encode key: %w", err)
	}

	return certPEM, keyPEM, nil
}

// CACertPEM 返回 CA 证书的 PEM 编码
func (m *Manager) CACertPEM() []byte {
	return certutil.EncodeCertToPEM(m.ca.Certificate)
}

// IsCAValid 检查 CA 证书是否有效
func (m *Manager) IsCAValid() bool {
	if m.ca == nil || m.ca.Certificate == nil {
		return false
	}
	_, err := x509.ParseCertificate(m.ca.Certificate.Raw)
	return err == nil
}

// GetCA 返回底层 CA 实例
func (m *Manager) GetCA() *certutil.CA {
	return m.ca
}

// EnsureCA 确保 CA 存在，返回错误信息
func (m *Manager) EnsureCA() error {
	if m.ca == nil || m.ca.Certificate == nil {
		return fmt.Errorf("CA not initialized")
	}
	if m.ca.PrivateKey == nil {
		return fmt.Errorf("CA private key not loaded")
	}
	if _, ok := m.ca.PrivateKey.(*ecdsa.PrivateKey); !ok {
		return fmt.Errorf("CA private key is not ECDSA")
	}
	return nil
}