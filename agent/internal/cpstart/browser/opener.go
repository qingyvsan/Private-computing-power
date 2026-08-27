package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open 在系统默认浏览器中打开 URL
func Open(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux
		cmd = "xdg-open"
		args = []string{url}
	}

	if err := exec.Command(cmd, args...).Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}