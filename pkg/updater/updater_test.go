package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ========== manifest tests ==========

func TestParseManifest(t *testing.T) {
	data := `{
		"version": "1.0.0",
		"release_date": "2026-08-29T00:00:00Z",
		"platforms": {
			"linux-amd64": {
				"core": {
					"url": "https://example.com/core.tar.gz",
					"sha256": "abc123",
					"size": 50000000
				}
			}
		}
	}`
	m, err := ParseManifest([]byte(data))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", m.Version)
	}
	if !m.ReleaseDate.Equal(time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("unexpected release_date: %v", m.ReleaseDate)
	}
}

func TestParseManifest_Invalid(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"empty version", `{"version": "", "platforms": {"linux-amd64": {"core": {"url": "x", "sha256": "y"}}}}`},
		{"no platforms", `{"version": "1.0.0", "platforms": {}}`},
		{"missing core url", `{"version": "1.0.0", "platforms": {"linux-amd64": {"core": {"sha256": "y"}}}}`},
		{"missing core sha256", `{"version": "1.0.0", "platforms": {"linux-amd64": {"core": {"url": "x"}}}}`},
		{"invalid platform key", `{"version": "1.0.0", "platforms": {"badkey": {"core": {"url": "x", "sha256": "y"}}}}`},
		{"invalid json", `{bad json}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseManifest([]byte(tt.data))
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestManifest_ForPlatform(t *testing.T) {
	m := &Manifest{
		Version: "1.0.0",
		Platforms: map[string]PlatformBundle{
			"linux-amd64": {
				Core: PackageInfo{URL: "http://example.com/linux-amd64/core.tar.gz", SHA256: "x"},
			},
			"windows-amd64": {
				Core: PackageInfo{URL: "http://example.com/windows-amd64/core.tar.gz", SHA256: "y"},
			},
		},
	}
	bundle, ok := m.ForPlatform("linux", "amd64")
	if !ok {
		t.Fatal("expected linux-amd64 platform")
	}
	if bundle.Core.URL != "http://example.com/linux-amd64/core.tar.gz" {
		t.Errorf("unexpected url: %s", bundle.Core.URL)
	}
	_, ok = m.ForPlatform("darwin", "arm64")
	if ok {
		t.Fatal("unexpected platform found")
	}
}

// ========== download tests ==========

func TestDownloadPackage(t *testing.T) {
	// 准备测试内容
	content := []byte("test package content")
	hash := sha256.Sum256(content)
	expectedHash := hex.EncodeToString(hash[:])

	// 启动测试 HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	info := PackageInfo{
		URL:    server.URL,
		SHA256: expectedHash,
		Size:   int64(len(content)),
	}

	destDir := t.TempDir()
	path, err := DownloadPackage(context.Background(), info, destDir)
	if err != nil {
		t.Fatalf("DownloadPackage: %v", err)
	}
	defer os.Remove(path)

	// 验证文件内容
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %s, expected %s", string(got), string(content))
	}
}

func TestDownloadPackage_HashMismatch(t *testing.T) {
	content := []byte("test content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	info := PackageInfo{
		URL:    server.URL,
		SHA256: "badhash",
	}

	destDir := t.TempDir()
	_, err := DownloadPackage(context.Background(), info, destDir)
	if err == nil {
		t.Fatal("expected hash mismatch error")
	}
	if !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDownloadPackage_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	info := PackageInfo{
		URL:    server.URL,
		SHA256: "x",
	}

	destDir := t.TempDir()
	_, err := DownloadPackage(context.Background(), info, destDir)
	if err == nil {
		t.Fatal("expected http error")
	}
}

// ========== backup tests ==========

func TestRotateBackups(t *testing.T) {
	backupDir := t.TempDir()

	// 创建临时二进制
	binaryPath := filepath.Join(t.TempDir(), "cpstart")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}
	if err := os.WriteFile(binaryPath, []byte("binary v1"), 0755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	// 第一次备份
	if err := RotateBackups(backupDir, binaryPath, 2); err != nil {
		t.Fatalf("first backup: %v", err)
	}

	// 验证备份文件
	entries, _ := os.ReadDir(backupDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(entries))
	}

	// 更新二进制并再次备份
	if err := os.WriteFile(binaryPath, []byte("binary v2"), 0755); err != nil {
		t.Fatalf("write binary v2: %v", err)
	}
	if err := RotateBackups(backupDir, binaryPath, 2); err != nil {
		t.Fatalf("second backup: %v", err)
	}

	entries, _ = os.ReadDir(backupDir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(entries))
	}

	// 第三次备份，应删除最旧的
	if err := os.WriteFile(binaryPath, []byte("binary v3"), 0755); err != nil {
		t.Fatalf("write binary v3: %v", err)
	}
	if err := RotateBackups(backupDir, binaryPath, 2); err != nil {
		t.Fatalf("third backup: %v", err)
	}

	entries, _ = os.ReadDir(backupDir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 backups after rotation, got %d", len(entries))
	}
}

func TestRotateBackups_Disabled(t *testing.T) {
	backupDir := t.TempDir()
	binaryPath := filepath.Join(t.TempDir(), "test.bin")
	os.WriteFile(binaryPath, []byte("data"), 0755)

	if err := RotateBackups(backupDir, binaryPath, 0); err != nil {
		t.Fatalf("backup with 0 max: %v", err)
	}
	entries, _ := os.ReadDir(backupDir)
	if len(entries) != 0 {
		t.Fatal("expected no backups when maxBackups=0")
	}
}

// ========== swap tests ==========

func TestSwapBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("swap test uses rename semantics better tested on unix")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "new-binary")
	target := filepath.Join(dir, "current-binary")
	backupDir := filepath.Join(dir, "backups")

	// 创建旧二进制
	if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
		t.Fatalf("write old: %v", err)
	}
	// 创建新二进制
	if err := os.WriteFile(src, []byte("new"), 0755); err != nil {
		t.Fatalf("write new: %v", err)
	}

	if err := SwapBinary(src, target, 1, backupDir); err != nil {
		t.Fatalf("SwapBinary: %v", err)
	}

	// 验证目标已更新
	data, _ := os.ReadFile(target)
	if string(data) != "new" {
		t.Errorf("expected target to be 'new', got %s", string(data))
	}

	// 验证备份存在
	entries, _ := os.ReadDir(backupDir)
	if len(entries) != 1 {
		t.Errorf("expected 1 backup, got %d", len(entries))
	}

	// 验证源文件已不存在（Rename 后）
	if _, err := os.Stat(src); err == nil {
		t.Error("expected source file to be removed after rename")
	}
}

// ========== updater tests ==========

func TestUpdater_CheckNow_NoUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version": "1.0.0", "platforms": {"linux-amd64": {"core": {"url": "http://x", "sha256": "y"}}}}`)
	}))
	defer server.Close()

	u := New(Config{
		Enabled:        true,
		ManifestURL:    server.URL,
		CurrentVersion: "1.0.0",
		Platform:       "linux-amd64",
	})

	info, err := u.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if info.Available {
		t.Fatal("expected no update when version matches")
	}
}

func TestUpdater_CheckNow_UpdateAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version": "1.1.0", "platforms": {"linux-amd64": {"core": {"url": "http://x", "sha256": "y"}}}}`)
	}))
	defer server.Close()

	u := New(Config{
		Enabled:        true,
		ManifestURL:    server.URL,
		CurrentVersion: "1.0.0",
		Platform:       "linux-amd64",
	})

	info, err := u.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if !info.Available {
		t.Fatal("expected update available")
	}
	if info.ManifestVersion != "1.1.0" {
		t.Errorf("expected manifest version 1.1.0, got %s", info.ManifestVersion)
	}
}

func TestUpdater_CheckNow_DevVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version": "1.1.0", "platforms": {"linux-amd64": {"core": {"url": "http://x", "sha256": "y"}}}}`)
	}))
	defer server.Close()

	u := New(Config{
		Enabled:        true,
		ManifestURL:    server.URL,
		CurrentVersion: "dev",
		Platform:       "linux-amd64",
	})

	info, err := u.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if info.Available {
		t.Fatal("expected no update for dev version")
	}
}

func TestUpdater_Apply(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("apply test uses rename semantics")
	}

	// 创建临时二进制作为"当前"二进制
	binDir := t.TempDir()
	binaryPath := filepath.Join(binDir, "test-agent")
	os.WriteFile(binaryPath, []byte("old-version"), 0755)

	// 创建测试 HTTP server 提供更新包
	updateContent := []byte("new-version-content")
	hash := sha256.Sum256(updateContent)
	hashStr := hex.EncodeToString(hash[:])

	// 用两个 endpoint 分别提供 manifest 和包下载
	downloadPath := "/releases/core.tar.gz"
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			fmt.Fprintf(w, `{"version": "2.0.0", "platforms": {"linux-amd64": {"core": {"url": "%s%s", "sha256": "%s", "size": %d}}}}`,
				serverURL, downloadPath, hashStr, len(updateContent))
		} else if r.URL.Path == downloadPath {
			w.Write(updateContent)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	manifestURL := server.URL + "/manifest.json"

	u := New(Config{
		Enabled:        true,
		ManifestURL:    manifestURL,
		CurrentVersion: "1.0.0",
		BinaryPath:     binaryPath,
		DownloadDir:    t.TempDir(),
		Platform:       "linux-amd64",
		BackupCount:    1,
	})

	info, err := u.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if !info.Available {
		t.Fatal("expected update available")
	}

	if err := u.Apply(context.Background(), info); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// 验证二进制已更新
	data, _ := os.ReadFile(binaryPath)
	if string(data) != string(updateContent) {
		t.Errorf("binary content mismatch: got %s, expected %s", string(data), string(updateContent))
	}
}

func TestUpdater_Status(t *testing.T) {
	u := New(Config{Enabled: true})
	if s := u.Status(); s != nil {
		t.Fatal("expected nil status before first check")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version": "1.0.0", "platforms": {"linux-amd64": {"core": {"url": "x", "sha256": "y"}}}}`)
	}))
	defer server.Close()
	u.cfg.ManifestURL = server.URL

	_, err := u.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if s := u.Status(); s == nil {
		t.Fatal("expected non-nil status after check")
	} else if s.ManifestVersion != "1.0.0" {
		t.Errorf("expected manifest version 1.0.0, got %s", s.ManifestVersion)
	}
}

func TestUpdater_StartStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version": "1.0.0", "platforms": {"linux-amd64": {"core": {"url": "x", "sha256": "y"}}}}`)
	}))
	defer server.Close()

	u := New(Config{
		Enabled:        true,
		ManifestURL:    server.URL,
		CurrentVersion: "1.0.0",
		Platform:       "linux-amd64",
		CheckInterval:  100 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	u.Start(ctx)

	// 给第一次检查一些时间
	time.Sleep(200 * time.Millisecond)

	if s := u.Status(); s == nil {
		t.Fatal("expected status after start")
	}

	cancel()
	// 等待 goroutine 退出
	time.Sleep(50 * time.Millisecond)
}