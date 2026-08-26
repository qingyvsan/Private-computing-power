package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadConfig 加载 YAML 配置文件
func LoadConfig(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse config file %s: %w", path, err)
	}
	return nil
}

// SaveConfig 保存配置到 YAML 文件
func SaveConfig(path string, v interface{}) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config file %s: %w", path, err)
	}
	return nil
}

// StringOrEnv 支持从环境变量读取配置
// 格式: "${ENV_VAR:default_value}" 或直接字符串
func StringOrEnv(val string) string {
	if len(val) > 2 && val[0] == '$' {
		envVal := os.Getenv(val[1:])
		if envVal != "" {
			return envVal
		}
	}
	return val
}