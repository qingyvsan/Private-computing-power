package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// SwapBinary 原子替换二进制文件。
// Linux/macOS: 使用 os.Rename 原子替换。
// Windows: 将新文件写入 .new 后缀，调用方需处理重启。
func SwapBinary(src, target string, backups int, backupDir string) error {
	// 确保目标目录存在
	targetDir := filepath.Dir(target)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("ensure target dir: %w", err)
	}

	// 备份当前二进制
	if backups > 0 {
		// 检查目标是否存在
		if _, err := os.Stat(target); err == nil {
			if err := RotateBackups(backupDir, target, backups); err != nil {
				return fmt.Errorf("backup: %w", err)
			}
		}
	}

	switch runtime.GOOS {
	case "windows":
		return swapWindows(src, target)
	default:
		return swapUnix(src, target)
	}
}

func swapUnix(src, target string) error {
	// Unix: os.Rename 是原子的（同文件系统）
	if err := os.Rename(src, target); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", src, target, err)
	}
	return nil
}

func swapWindows(src, target string) error {
	// Windows: 运行中的 exe 不可写，写入 .new 后缀
	newPath := target + ".new"
	if err := os.Rename(src, newPath); err != nil {
		return fmt.Errorf("rename to .new: %w", err)
	}
	// 调用方检测 .new 后缀并处理重启
	return nil
}

// HasPendingUpdate 检查是否有待应用的更新（Windows .new 文件）
func HasPendingUpdate(binaryPath string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	_, err := os.Stat(binaryPath + ".new")
	return err == nil
}

// ApplyPendingUpdate 应用待处理的更新（Windows 专用）
func ApplyPendingUpdate(binaryPath string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	newPath := binaryPath + ".new"
	oldPath := binaryPath + ".old"

	// 删除旧的 .old 文件（如果存在）
	os.Remove(oldPath)

	// 重命名当前 → .old
	if err := os.Rename(binaryPath, oldPath); err != nil {
		return fmt.Errorf("rename current -> .old: %w", err)
	}
	// 重命名 .new → 当前
	if err := os.Rename(newPath, binaryPath); err != nil {
		// 回滚
		os.Rename(oldPath, binaryPath)
		return fmt.Errorf("rename .new -> current: %w", err)
	}
	return nil
}