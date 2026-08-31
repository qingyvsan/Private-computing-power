package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 调度器配置
type Config struct {
	Scheduler struct {
		ID     string `yaml:"id"`
		NodeID string `yaml:"node_id"`
	} `yaml:"scheduler"`

	Server struct {
		GRPC struct {
			Listen         string `yaml:"listen"`
			MaxRecvMsgSize int    `yaml:"max_recv_msg_size"`
			MaxSendMsgSize int    `yaml:"max_send_msg_size"`
		} `yaml:"grpc"`
		HTTP struct {
			Listen  string `yaml:"listen"`
			Enabled bool   `yaml:"enabled"`
		} `yaml:"http"`
	} `yaml:"server"`

	TLS struct {
		Enabled    bool   `yaml:"enabled"`
		CACert     string `yaml:"ca_cert"`
		CAKey      string `yaml:"ca_key"`
		ServerCert string `yaml:"server_cert"`
		ServerKey  string `yaml:"server_key"`
	} `yaml:"tls"`

	Database struct {
		Engine         string `yaml:"engine"`
		Path           string `yaml:"path"`
		BackupInterval string `yaml:"backup_interval"`
		BackupDir      string `yaml:"backup_dir"`
	} `yaml:"database"`

	FailureDetection struct {
		PhiThreshold      float64 `yaml:"phi_threshold"`
		MinSamples        int     `yaml:"min_samples"`
		WindowSize        int     `yaml:"window_size"`
		HeartbeatInterval string  `yaml:"heartbeat_interval"`
		HeartbeatTimeout  string  `yaml:"heartbeat_timeout"`
	} `yaml:"failure_detection"`

	SchedulerEngine struct {
		MaxRetries            int    `yaml:"max_retries"`
		ReassignDelay         string `yaml:"reassign_delay"`
		ConcurrentAssignments int    `yaml:"concurrent_assignments"`
		ScoringWeights        struct {
			ResourceMatch float64 `yaml:"resource_match"`
			NetworkQuality float64 `yaml:"network_quality"`
			Reputation    float64 `yaml:"reputation"`
			Load          float64 `yaml:"load"`
		} `yaml:"scoring_weights"`
	} `yaml:"scheduler_engine"`

	Nebula struct {
		CACert    string `yaml:"ca_cert"`
		CAKey     string `yaml:"ca_key"`
		Lighthouse struct {
			Addr string `yaml:"addr"`
			Host string `yaml:"host"`
		} `yaml:"lighthouse"`
		Network string            `yaml:"network"`
		Groups  map[string]string `yaml:"groups"`
	} `yaml:"nebula"`

	Invitation struct {
		CodeLength     int    `yaml:"code_length"`
		CodeExpiry     string `yaml:"code_expiry"`
		MaxUsesPerCode int    `yaml:"max_uses_per_code"`
		AdminKey       string `yaml:"admin_key"`
	} `yaml:"invitation"`

	Sync struct {
		Enabled             bool   `yaml:"enabled"`
		Role                string `yaml:"role"`
		ListenAddr          string `yaml:"listen_addr"`
		PrimaryAddr         string `yaml:"primary_addr"`
		WalDir              string `yaml:"wal_dir"`
		MaxWalSize          int64  `yaml:"max_wal_size"`
		CheckpointInterval  string `yaml:"checkpoint_interval"`
		SyncBandwidthMbps   int    `yaml:"sync_bandwidth_mbps"`
		Compression         string `yaml:"compression"`
		HealthCheckInterval string `yaml:"health_check_interval"`
		FailoverTimeout     string `yaml:"failover_timeout"`
	} `yaml:"sync"`

	Logging struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
		Output string `yaml:"output"`
	} `yaml:"logging"`
}

// Default 返回默认配置
func Default() *Config {
	c := &Config{}
	c.Scheduler.ID = "scheduler-1"
	c.Scheduler.NodeID = "scheduler"
	c.Server.GRPC.Listen = "0.0.0.0:9090"
	c.Server.GRPC.MaxRecvMsgSize = 16777216
	c.Server.GRPC.MaxSendMsgSize = 16777216
	c.Server.HTTP.Listen = "0.0.0.0:8080"
	c.Server.HTTP.Enabled = false
	c.TLS.Enabled = true
	c.TLS.CACert = "configs/ca.pem"
	c.TLS.CAKey = "configs/ca-key.pem"
	c.Database.Engine = "boltdb"
	c.Database.Path = "./data/scheduler.db"
	c.FailureDetection.PhiThreshold = 4.0
	c.FailureDetection.MinSamples = 100
	c.FailureDetection.WindowSize = 1000
	c.FailureDetection.HeartbeatInterval = "3s"
	c.FailureDetection.HeartbeatTimeout = "30s"
	c.SchedulerEngine.MaxRetries = 3
	c.SchedulerEngine.ReassignDelay = "5s"
	c.SchedulerEngine.ConcurrentAssignments = 100
	c.SchedulerEngine.ScoringWeights.ResourceMatch = 0.4
	c.SchedulerEngine.ScoringWeights.NetworkQuality = 0.3
	c.SchedulerEngine.ScoringWeights.Reputation = 0.2
	c.SchedulerEngine.ScoringWeights.Load = 0.1
	c.Nebula.CACert = "configs/nebula/ca.crt"
	c.Nebula.CAKey = "configs/nebula/ca.key"
	c.Nebula.Lighthouse.Addr = "0.0.0.0:4242"
	c.Nebula.Lighthouse.Host = "10.0.0.1"
	c.Nebula.Network = "10.1.0.0/16"
	c.Nebula.Groups = map[string]string{
		"master":       "master",
		"trusted":      "trusted",
		"mutual":       "mutual",
		"discoverable": "discoverable",
		"default":      "default",
	}
	c.Invitation.CodeLength = 32
	c.Invitation.CodeExpiry = "72h"
	c.Invitation.MaxUsesPerCode = 1
	c.Invitation.AdminKey = ""

	c.Sync.Enabled = false
	c.Sync.Role = "primary"
	c.Sync.ListenAddr = "0.0.0.0:9091"
	c.Sync.PrimaryAddr = "8.138.108.183:9091"
	c.Sync.WalDir = "./data/wal"
	c.Sync.MaxWalSize = 100 * 1024 * 1024 // 100MB
	c.Sync.CheckpointInterval = "5m"
	c.Sync.SyncBandwidthMbps = 100
	c.Sync.Compression = "zstd"
	c.Sync.HealthCheckInterval = "5s"
	c.Sync.FailoverTimeout = "15s"

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