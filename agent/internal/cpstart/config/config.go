package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	agentcfg "computing-power/agent/internal/config"
)

// Config cpstart 配置
type Config struct {
	Agent struct {
		Name    string `json:"name" yaml:"name"`
		DataDir string `json:"data_dir" yaml:"data_dir"`
	} `json:"agent" yaml:"agent"`
	Scheduler struct {
		Address string `json:"address" yaml:"address"`
	} `json:"scheduler" yaml:"scheduler"`
	Resources struct {
		MaxCPUCores float64 `json:"max_cpu_cores" yaml:"max_cpu_cores"`
		MaxMemoryMB int64   `json:"max_memory_mb" yaml:"max_memory_mb"`
		ReportGPU   bool    `json:"report_gpu" yaml:"report_gpu"`
	} `json:"resources" yaml:"resources"`
	Console struct {
		Port     int  `json:"port" yaml:"port"`
		AutoOpen bool `json:"auto_open" yaml:"auto_open"`
	} `json:"console" yaml:"console"`
	TLS struct {
		CACert string `json:"ca_cert" yaml:"ca_cert"`
		Cert   string `json:"cert" yaml:"cert"`
		Key    string `json:"key" yaml:"key"`
	} `json:"tls" yaml:"tls"`
	InviteCode string `json:"invite_code" yaml:"invite_code"`

	Nebula struct {
		Enabled bool `json:"enabled" yaml:"enabled"`
	} `json:"nebula" yaml:"nebula"`

	HAMI struct {
		Enabled bool `json:"enabled" yaml:"enabled"`
	} `json:"hami" yaml:"hami"`

	Updater struct {
		Enabled bool `json:"enabled" yaml:"enabled"`
	} `json:"updater" yaml:"updater"`

	WSL2 struct {
		Enabled   bool `json:"enabled" yaml:"enabled"`       // 是否启用 WSL2 容器后端（默认 true）
		ProxyPort int  `json:"proxy_port" yaml:"proxy_port"` // socat 代理 TCP 端口，0 = 默认 19090

		DistroName       string `json:"distro_name" yaml:"distro_name"`             // WSL2 发行版名称，空 = 默认 Ubuntu-24.04
		InstallPath      string `json:"install_path" yaml:"install_path"`           // Windows 端安装目录，空 = 默认 ~/wsl/{distro}
		ContainerdSocket string `json:"containerd_socket" yaml:"containerd_socket"` // WSL2 内 containerd socket 路径，空 = 默认 /run/containerd/containerd.sock
	} `json:"wsl2" yaml:"wsl2"`
}

// Default 返回默认配置
func Default() *Config {
	hostname, _ := os.Hostname()
	c := &Config{}
	c.Agent.Name = hostname
	c.Agent.DataDir = "./data/cpstart"
	c.Scheduler.Address = "8.138.108.183:9090"
	c.Resources.MaxCPUCores = 0 // 0 = 全部
	c.Resources.MaxMemoryMB = 0 // 0 = 全部
	c.Resources.ReportGPU = true
	c.Console.Port = 8080
	c.Console.AutoOpen = true
	c.WSL2.Enabled = true
	c.WSL2.ProxyPort = 0 // 0 = 使用默认 19090
	c.WSL2.DistroName = "Ubuntu-24.04"
	c.WSL2.InstallPath = ""
	c.WSL2.ContainerdSocket = ""
	return c
}

// Load 从文件加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	c := Default()
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return c, nil
}

// Save 保存配置到文件
func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// ConfigPath 返回配置文件的完整路径
func (c *Config) ConfigPath() string {
	return filepath.Join(c.Agent.DataDir, "cpstart.yaml")
}

// ContainerdNamespace 返回 containerd namespace（与 agent 配置一致）
func (c *Config) ContainerdNamespace() string {
	return agentcfg.Default().Containerd.Namespace
}

// ToAgentConfig 将 cpstart 配置转换为 agent 配置
func (c *Config) ToAgentConfig() *agentcfg.Config {
	ac := agentcfg.Default()
	ac.Agent.Name = c.Agent.Name
	ac.Agent.DataDir = c.Agent.DataDir
	ac.Scheduler.Address = c.Scheduler.Address
	ac.Scheduler.InviteCode = c.InviteCode
	ac.Resources.ReportGPU = c.Resources.ReportGPU
	ac.Resources.MaxCPUCores = c.Resources.MaxCPUCores
	ac.Resources.MaxMemoryMB = c.Resources.MaxMemoryMB
	ac.Nebula.Enabled = c.Nebula.Enabled
	ac.HAMI.Enabled = c.HAMI.Enabled
	ac.Updater.Enabled = c.Updater.Enabled
	return ac
}
