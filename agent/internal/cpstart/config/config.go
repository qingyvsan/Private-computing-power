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
		Name    string `yaml:"name"`
		DataDir string `yaml:"data_dir"`
	} `yaml:"agent"`
	Scheduler struct {
		Address string `yaml:"address"`
	} `yaml:"scheduler"`
	Resources struct {
		MaxCPUCores float64 `yaml:"max_cpu_cores"`
		MaxMemoryMB int64   `yaml:"max_memory_mb"`
		ReportGPU   bool    `yaml:"report_gpu"`
	} `yaml:"resources"`
	Console struct {
		Port     int  `yaml:"port"`
		AutoOpen bool `yaml:"auto_open"`
	} `yaml:"console"`
	InviteCode string `yaml:"invite_code"`
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

// ToAgentConfig 将 cpstart 配置转换为 agent 配置
func (c *Config) ToAgentConfig() *agentcfg.Config {
	ac := agentcfg.Default()
	ac.Agent.Name = c.Agent.Name
	ac.Agent.DataDir = c.Agent.DataDir
	ac.Scheduler.Address = c.Scheduler.Address
	ac.Scheduler.InviteCode = c.InviteCode
	ac.Resources.ReportGPU = c.Resources.ReportGPU
	return ac
}