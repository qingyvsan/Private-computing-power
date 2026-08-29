package updater

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Manifest 更新清单，描述一个版本的所有分发包信息
type Manifest struct {
	Version     string                  `json:"version"`
	ReleaseDate time.Time               `json:"release_date"`
	MinVersion  string                  `json:"min_version,omitempty"`
	Platforms   map[string]PlatformBundle `json:"platforms"` // key: "linux-amd64"
}

// PlatformBundle 单个平台的三层分发包
type PlatformBundle struct {
	Core    PackageInfo `json:"core"`
	Runtime PackageInfo `json:"runtime,omitempty"`
	GPU     PackageInfo `json:"gpu,omitempty"`
}

// PackageInfo 单个包的信息
type PackageInfo struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// ParseManifest 解析 JSON 格式的更新清单
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate 校验清单必填字段
func (m *Manifest) Validate() error {
	if m.Version == "" {
		return fmt.Errorf("manifest: version is required")
	}
	if len(m.Platforms) == 0 {
		return fmt.Errorf("manifest: at least one platform is required")
	}
	for key, bundle := range m.Platforms {
		if bundle.Core.URL == "" || bundle.Core.SHA256 == "" {
			return fmt.Errorf("manifest: platform %q: core package url and sha256 are required", key)
		}
		parts := strings.Split(key, "-")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("manifest: invalid platform key %q (expected os-arch)", key)
		}
	}
	return nil
}

// ForPlatform 获取指定平台的包信息
func (m *Manifest) ForPlatform(goos, goarch string) (*PlatformBundle, bool) {
	key := fmt.Sprintf("%s-%s", goos, goarch)
	bundle, ok := m.Platforms[key]
	if !ok {
		return nil, false
	}
	return &bundle, true
}