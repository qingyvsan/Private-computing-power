package updater

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RotateBackups 备份当前二进制并轮转，保留最多 maxBackups 份
func RotateBackups(backupDir, currentBinary string, maxBackups int) error {
	if maxBackups <= 0 {
		return nil
	}

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	// 备份当前文件
	timestamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	ext := filepath.Ext(currentBinary)
	base := strings.TrimSuffix(filepath.Base(currentBinary), ext)
	backupName := fmt.Sprintf("%s.%s%s", base, timestamp, ext)
	backupPath := filepath.Join(backupDir, backupName)

	src, err := os.Open(currentBinary)
	if err != nil {
		return fmt.Errorf("open current binary: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(backupPath)
		return fmt.Errorf("copy backup: %w", err)
	}

	// 删除超出 maxBackups 的旧备份
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("read backup dir: %w", err)
	}

	var backups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), base+".") {
			backups = append(backups, e.Name())
		}
	}

	// 按名称（时间戳）排序，删除最旧的
	sort.Strings(backups)
	for len(backups) > maxBackups {
		old := filepath.Join(backupDir, backups[0])
		os.Remove(old)
		backups = backups[1:]
	}

	return nil
}