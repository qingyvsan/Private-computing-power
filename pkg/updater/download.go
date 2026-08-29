package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// DownloadPackage 下载包并校验 SHA256，返回临时文件路径
func DownloadPackage(ctx context.Context, info PackageInfo, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("create download dir: %w", err)
	}

	// 创建临时文件（与目标同目录，确保后续 rename 在同一文件系统）
	tmpFile := filepath.Join(destDir, fmt.Sprintf(".download-%d.tmp", os.Getpid()))
	f, err := os.Create(tmpFile)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer f.Close()

	// HTTP GET
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.URL, nil)
	if err != nil {
		os.Remove(tmpFile)
		return "", fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		os.Remove(tmpFile)
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		os.Remove(tmpFile)
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	// 流式下载 + 实时计算 SHA256
	hasher := sha256.New()
	writer := io.MultiWriter(f, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		os.Remove(tmpFile)
		return "", fmt.Errorf("download: %w", err)
	}

	// 关闭文件以确保全部写入
	if err := f.Close(); err != nil {
		os.Remove(tmpFile)
		return "", fmt.Errorf("close temp file: %w", err)
	}

	// 校验 SHA256
	gotHash := hex.EncodeToString(hasher.Sum(nil))
	if gotHash != info.SHA256 {
		os.Remove(tmpFile)
		return "", fmt.Errorf("sha256 mismatch: expected %s, got %s", info.SHA256, gotHash)
	}

	return tmpFile, nil
}