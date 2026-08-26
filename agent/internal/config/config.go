package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config Agent 配置
type Config struct {
	Agent struct {
		ID      string `yaml:"id"`
		Name    string `yaml:"name"`
		DataDir string `yaml:"data_dir"`
		TempDir string `yaml:"temp_dir"`
	} `yaml:"agent"`

	Scheduler struct {
		Address            string `yaml:"address"`
		TLSCert            string `yaml:"tls_cert"`
		TLSKey             string `yaml:"tls_key"`
		CACert             string `yaml:"ca_cert"`
		ReconnectInitial   string `yaml:"reconnect_initial"`
		ReconnectMax       string `yaml:"reconnect_max"`
		ReconnectMultiplier float64 `yaml:"reconnect_multiplier"`
	} `yaml:"scheduler"`

	Heartbeat struct {
		Interval          string `yaml:"interval"`
		Jitter            string `yaml:"jitter"`
		FullReportInterval string `yaml:"full_report_interval"`
		IncludeRunningTasks bool  `yaml:"include_running_tasks"`
	} `yaml:"heartbeat"`

	Containerd struct {
		Socket        string `yaml:"socket"`
		Namespace     string `yaml:"namespace"`
		Snapshotter   string `yaml:"snapshotter"`
		PullTimeout   string `yaml:"pull_timeout"`
		DragonflyProxy bool  `yaml:"dragonfly_proxy"`
	} `yaml:"containerd"`

	Kata struct {
		Enabled     bool   `yaml:"enabled"`
		RuntimeClass string `yaml:"runtime_class"`
		ConfigPath  string `yaml:"config_path"`
	} `yaml:"kata"`

	HAMI struct {
		Enabled     bool   `yaml:"enabled"`
		LibPath     string `yaml:"lib_path"`
		ConfigDir   string `yaml:"config_dir"`
		DefaultMemoryMB int64 `yaml:"default_memory_mb"`
		DefaultCores int32  `yaml:"default_cores"`
	} `yaml:"hami"`

	Resources struct {
		CollectInterval     string `yaml:"collect_interval"`
		ReportGPU           bool   `yaml:"report_gpu"`
		ReportNetwork       bool   `yaml:"report_network"`
		NetworkProbeInterval string `yaml:"network_probe_interval"`
	} `yaml:"resources"`

	Nebula struct {
		ConfigPath string `yaml:"config_path"`
		BinaryPath string `yaml:"binary_path"`
		DataDir    string `yaml:"data_dir"`
		CertDir    string `yaml:"cert_dir"`
	} `yaml:"nebula"`

	Sync struct {
		Enabled          bool   `yaml:"enabled"`
		WALDir           string `yaml:"wal_dir"`
		CheckpointInterval string `yaml:"checkpoint_interval"`
		SyncBandwidthMbps int  `yaml:"sync_bandwidth_mbps"`
		Compression      string `yaml:"compression"`
	} `yaml:"sync"`

	Updater struct {
		Enabled      bool   `yaml:"enabled"`
		CheckInterval string `yaml:"check_interval"`
		ManifestURL  string `yaml:"manifest_url"`
		DownloadDir  string `yaml:"download_dir"`
		BackupCount  int    `yaml:"backup_count"`
	} `yaml:"updater"`

	Logging struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
		Output string `yaml:"output"`
	} `yaml:"logging"`
}

// Default 返回默认配置
func Default() *Config {
	c := &Config{}
	c.Agent.ID = ""
	c.Agent.Name = ""
	c.Agent.DataDir = "./data/agent"
	c.Agent.TempDir = "./tmp/agent"
	c.Scheduler.Address = "localhost:9090"
	c.Scheduler.ReconnectInitial = "1s"
	c.Scheduler.ReconnectMax = "60s"
	c.Scheduler.ReconnectMultiplier = 2.0
	c.Heartbeat.Interval = "3s"
	c.Heartbeat.Jitter = "0.5s"
	c.Heartbeat.FullReportInterval = "60s"
	c.Heartbeat.IncludeRunningTasks = true
	c.Containerd.Socket = "/run/containerd/containerd.sock"
	c.Containerd.Namespace = "computing-power"
	c.Containerd.Snapshotter = "overlayfs"
	c.Containerd.PullTimeout = "10m"
	c.Containerd.DragonflyProxy = true
	c.Kata.Enabled = false
	c.Kata.RuntimeClass = "kata-qemu"
	c.Kata.ConfigPath = "/etc/kata-containers/configuration.toml"
	c.HAMI.Enabled = false
	c.HAMI.LibPath = "/usr/lib/libvgpu.so"
	c.HAMI.ConfigDir = "./data/agent/hami"
	c.HAMI.DefaultMemoryMB = 1024
	c.HAMI.DefaultCores = 10
	c.Resources.CollectInterval = "10s"
	c.Resources.ReportGPU = true
	c.Resources.ReportNetwork = true
	c.Resources.NetworkProbeInterval = "30s"
	c.Nebula.ConfigPath = "./data/agent/nebula/config.yaml"
	c.Nebula.BinaryPath = "nebula"
	c.Nebula.DataDir = "./data/agent/nebula"
	c.Nebula.CertDir = "./data/agent/nebula/certs"
	c.Sync.Enabled = false
	c.Sync.WALDir = "./data/agent/wal"
	c.Sync.CheckpointInterval = "5m"
	c.Sync.SyncBandwidthMbps = 100
	c.Sync.Compression = "zstd"
	c.Updater.Enabled = false
	c.Updater.CheckInterval = "6h"
	c.Updater.ManifestURL = "https://update.computing-power.local/v1/manifest.json"
	c.Updater.DownloadDir = "./data/agent/updates"
	c.Updater.BackupCount = 2
	c.Logging.Level = "info"
	c.Logging.Format = "json"
	c.Logging.Output = "stdout"
	return c
}

// Load 从文件加载配置
func Load(path string) (*Config, error) {
	c := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return c, nil
}