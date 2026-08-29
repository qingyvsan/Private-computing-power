package updater

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Config 更新器配置
type Config struct {
	Enabled        bool
	CheckInterval  time.Duration
	ManifestURL    string
	DownloadDir    string
	BackupCount    int
	CurrentVersion string
	BinaryPath     string
	Platform       string // e.g. "linux-amd64"
}

// UpdateInfo 更新检查结果
type UpdateInfo struct {
	ManifestVersion string
	Available       bool
	Package         PackageInfo
}

// Updater 自动更新器
type Updater struct {
	cfg    Config
	client *http.Client
	mu     sync.Mutex
	info   *UpdateInfo
}

// New 创建更新器
func New(cfg Config) *Updater {
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 6 * time.Hour
	}
	if cfg.BackupCount <= 0 {
		cfg.BackupCount = 2
	}
	if cfg.Platform == "" {
		cfg.Platform = fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	}

	return &Updater{
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Minute,
		},
	}
}

// Start 启动定时检查（goroutine）
func (u *Updater) Start(ctx context.Context) {
	if !u.cfg.Enabled {
		log.Printf("updater: disabled")
		return
	}
	if u.cfg.ManifestURL == "" {
		log.Printf("updater: manifest URL not configured")
		return
	}

	log.Printf("updater: started (interval=%s, manifest=%s, platform=%s)",
		u.cfg.CheckInterval, u.cfg.ManifestURL, u.cfg.Platform)

	go func() {
		// 首次启动延迟，避免刚启动就检查；测试用小间隔时不延迟
		initialDelay := 30 * time.Second
		if u.cfg.CheckInterval < initialDelay {
			initialDelay = u.cfg.CheckInterval / 2
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(initialDelay):
		}

		ticker := time.NewTicker(u.cfg.CheckInterval)
		defer ticker.Stop()

		for {
			u.checkAndLog(ctx)
			select {
			case <-ctx.Done():
				log.Printf("updater: stopped")
				return
			case <-ticker.C:
			}
		}
	}()
}

// CheckNow 立即检查更新
func (u *Updater) CheckNow(ctx context.Context) (*UpdateInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.cfg.ManifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	manifest, err := ParseManifest(body)
	if err != nil {
		return nil, err
	}

	// 检查版本
	current := u.cfg.CurrentVersion
	remote := manifest.Version

	info := &UpdateInfo{
		ManifestVersion: remote,
	}

	// 简单版本比较（字符串比较，dev 版本不更新）
	if current == "dev" || strings.HasPrefix(current, "dev-") {
		info.Available = false
		u.mu.Lock()
		u.info = info
		u.mu.Unlock()
		return info, nil
	}

	// 解析平台
	goos, goarch := parsePlatform(u.cfg.Platform)
	bundle, ok := manifest.ForPlatform(goos, goarch)
	if !ok {
		info.Available = false
		u.mu.Lock()
		u.info = info
		u.mu.Unlock()
		return info, nil
	}

	// 版本比较：remote > current 时有更新
	if remote > current {
		info.Available = true
		info.Package = bundle.Core
	}

	u.mu.Lock()
	u.info = info
	u.mu.Unlock()

	return info, nil
}

// Apply 应用更新（下载 + 替换 + 备份）
func (u *Updater) Apply(ctx context.Context, info *UpdateInfo) error {
	if !info.Available {
		return fmt.Errorf("no update available")
	}

	downloadDir := filepath.Join(u.cfg.DownloadDir, info.ManifestVersion)
	tmpPath, err := DownloadPackage(ctx, info.Package, downloadDir)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	backupDir := filepath.Join(u.cfg.DownloadDir, "backups")
	if err := SwapBinary(tmpPath, u.cfg.BinaryPath, u.cfg.BackupCount, backupDir); err != nil {
		return fmt.Errorf("swap binary: %w", err)
	}

	log.Printf("updater: applied version %s (binary: %s)", info.ManifestVersion, u.cfg.BinaryPath)
	return nil
}

// Status 返回最近一次更新检查结果
func (u *Updater) Status() *UpdateInfo {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.info == nil {
		return nil
	}
	cp := *u.info
	return &cp
}

func (u *Updater) checkAndLog(ctx context.Context) {
	info, err := u.CheckNow(ctx)
	if err != nil {
		log.Printf("updater: check failed: %v", err)
		return
	}
	if info.Available {
		log.Printf("updater: update available: %s -> %s", u.cfg.CurrentVersion, info.ManifestVersion)
	} else {
		log.Printf("updater: no update (current=%s, remote=%s)", u.cfg.CurrentVersion, info.ManifestVersion)
	}
}

// parsePlatform 将 "linux-amd64" 拆分为 "linux", "amd64"
func parsePlatform(platform string) (string, string) {
	parts := strings.SplitN(platform, "-", 2)
	if len(parts) != 2 {
		return runtime.GOOS, runtime.GOARCH
	}
	return parts[0], parts[1]
}

// Ensure interface compliance
var _ = (io.ReadCloser)(nil)
var _ = os.FileMode(0)