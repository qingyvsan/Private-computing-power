package nebula

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Manager 管理 Nebula 节点生命周期
type Manager struct {
	cfg       *Config
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	done      chan struct{}
	mu        sync.Mutex
	running   atomic.Bool
	natType   atomic.Value // string
	overlayIP atomic.Value // string
	startTime time.Time
}

// Config Nebula 管理器配置
type Config struct {
	Enabled    bool   // 是否启用 Nebula
	BinaryPath string // nebula 二进制路径
	DataDir    string // 数据目录
	ConfigPath string // 配置文件路径
	CertDir    string // 证书目录
}

// NewManager 创建 Nebula 管理器
func NewManager(cfg *Config) *Manager {
	m := &Manager{
		cfg:  cfg,
		done: make(chan struct{}),
	}
	m.natType.Store("unknown")
	m.overlayIP.Store("")
	return m
}

// Configure 写入 Nebula 证书和配置
// certPEM: 节点证书 PEM
// keyPEM: 节点私钥 PEM
// caPEM: CA 证书 PEM
// overlayIP: 分配的 Overlay IP
// lighthouseAddr: Lighthouse 地址（IP:端口）
// nebulaConfigYAML: 调度器生成的 Nebula 配置 YAML
func (m *Manager) Configure(certPEM, keyPEM, caPEM []byte, overlayIP, lighthouseAddr, nebulaConfigYAML string) error {
	if !m.cfg.Enabled {
		return fmt.Errorf("nebula is not enabled")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 创建目录
	dirs := []string{m.cfg.DataDir, m.cfg.CertDir, filepath.Dir(m.cfg.ConfigPath)}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	// 写入 CA 证书
	if err := os.WriteFile(filepath.Join(m.cfg.CertDir, "ca.crt"), caPEM, 0644); err != nil {
		return fmt.Errorf("write CA cert: %w", err)
	}

	// 写入节点证书
	if err := os.WriteFile(filepath.Join(m.cfg.CertDir, "node.crt"), certPEM, 0644); err != nil {
		return fmt.Errorf("write node cert: %w", err)
	}

	// 写入节点私钥
	if err := os.WriteFile(filepath.Join(m.cfg.CertDir, "node.key"), keyPEM, 0600); err != nil {
		return fmt.Errorf("write node key: %w", err)
	}

	// 写入 Nebula 配置
	configPath := m.cfg.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(m.cfg.DataDir, "config.yaml")
	}
	if err := os.WriteFile(configPath, []byte(nebulaConfigYAML), 0644); err != nil {
		return fmt.Errorf("write nebula config: %w", err)
	}

	m.overlayIP.Store(overlayIP)
	log.Printf("nebula: configured for overlay IP %s, lighthouse %s", overlayIP, lighthouseAddr)
	return nil
}

// Start 启动 Nebula 进程
func (m *Manager) Start() error {
	if !m.cfg.Enabled {
		return fmt.Errorf("nebula is not enabled")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running.Load() {
		return nil // 已运行
	}

	// 检查二进制是否存在
	binaryPath := m.cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = "nebula"
	}

	// 先尝试在 PATH 中查找
	_, err := exec.LookPath(binaryPath)
	if err != nil {
		return fmt.Errorf("nebula binary not found in PATH (%s): %w", binaryPath, err)
	}

	// 检查配置文件
	configPath := m.cfg.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(m.cfg.DataDir, "config.yaml")
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("nebula config not found at %s: call Configure first", configPath)
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	cmd := exec.CommandContext(ctx, binaryPath, "-config", configPath)
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start nebula: %w", err)
	}

	m.cmd = cmd
	m.running.Store(true)
	m.startTime = time.Now()
	m.done = make(chan struct{})

	// 监控进程
	go m.monitor(cmd)

	log.Printf("nebula: started (binary=%s, config=%s, pid=%d)", binaryPath, configPath, cmd.Process.Pid)
	return nil
}

// monitor 监控 Nebula 进程，等待退出后更新状态
func (m *Manager) monitor(cmd *exec.Cmd) {
	defer func() {
		m.running.Store(false)
		close(m.done)
	}()

	if err := cmd.Wait(); err != nil {
		if err.Error() != "signal: killed" {
			log.Printf("nebula: process exited: %v", err)
		}
	}
}

// Stop 停止 Nebula 进程
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running.Load() {
		return nil
	}

	if m.cancel != nil {
		m.cancel()
	}

	// 等待进程退出（最多 5 秒）
	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		if m.cmd != nil && m.cmd.Process != nil {
			m.cmd.Process.Kill()
		}
	}

	m.running.Store(false)
	log.Printf("nebula: stopped")
	return nil
}

// IsRunning 返回 Nebula 进程是否在运行
func (m *Manager) IsRunning() bool {
	return m.running.Load()
}

// GetNATType 返回 NAT 类型
func (m *Manager) GetNATType() string {
	if natType, ok := m.natType.Load().(string); ok && natType != "" {
		return natType
	}
	return "unknown"
}

// GetOverlayIP 返回 Overlay IP
func (m *Manager) GetOverlayIP() string {
	if ip, ok := m.overlayIP.Load().(string); ok {
		return ip
	}
	return ""
}

// Uptime 返回运行时间
func (m *Manager) Uptime() time.Duration {
	if !m.running.Load() {
		return 0
	}
	return time.Since(m.startTime)
}

// SetNATType 设置 NAT 类型（由外部检测更新）
func (m *Manager) SetNATType(natType string) {
	m.natType.Store(natType)
}

// IsConfigured 检查是否已配置（证书和配置文件存在）
func (m *Manager) IsConfigured() bool {
	certDir := m.cfg.CertDir
	if certDir == "" {
		return false
	}
	// 检查必要的文件是否存在
	files := []string{
		filepath.Join(certDir, "ca.crt"),
		filepath.Join(certDir, "node.crt"),
		filepath.Join(certDir, "node.key"),
	}
	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// ConfigPath 返回配置文件路径
func (m *Manager) ConfigPath() string {
	return m.cfg.ConfigPath
}

// CertDir 返回证书目录
func (m *Manager) CertDir() string {
	return m.cfg.CertDir
}