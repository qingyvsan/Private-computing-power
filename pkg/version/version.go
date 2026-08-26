package version

import "fmt"

var (
	// Version 版本号，编译时通过 -ldflags 注入
	Version = "dev"
	// GitCommit Git 提交哈希，编译时通过 -ldflags 注入
	GitCommit = "unknown"
	// BuildTime 构建时间，编译时通过 -ldflags 注入
	BuildTime = "unknown"
)

// Info 返回完整的版本信息字符串
func Info() string {
	return fmt.Sprintf("computing-power v%s (%s) built at %s", Version, GitCommit, BuildTime)
}

// Short 返回简短的版本号
func Short() string {
	return Version
}