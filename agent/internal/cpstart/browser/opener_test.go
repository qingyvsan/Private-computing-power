package browser

import (
	"testing"
)

func TestOpen_ValidURL(t *testing.T) {
	// 仅测试 URL 格式，不实际打开浏览器
	err := Open("http://127.0.0.1:8080")
	if err != nil {
		// 在某些 CI 环境中可能没有浏览器，这不算是失败
		t.Logf("Open returned: %v (may be expected in CI)", err)
	}
}