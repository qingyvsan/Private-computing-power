package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"computing-power/pkg/version"
)

// RootCmd 根命令
var RootCmd = &cobra.Command{
	Use:   "cpcli",
	Short: "Computing Power 命令行工具",
	Long: `cpcli 是去中心化个人算力共享平台的命令行工具。

支持节点注册、作业提交、信任管理、邀请码等功能。`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// 全局变量
var (
	// schedulerAddr 调度器地址
	schedulerAddr string
	// outputFormat 输出格式
	outputFormat string
)

func init() {
	RootCmd.PersistentFlags().StringVarP(&schedulerAddr, "scheduler", "s", "localhost:9090", "调度器 gRPC 地址")
	RootCmd.PersistentFlags().StringVarP(&outputFormat, "format", "f", "table", "输出格式: table | json")

	// 注册子命令
	RootCmd.AddCommand(
		registerCmd,
		nodeCmd,
		jobCmd,
		trustCmd,
		inviteCmd,
		versionCmd,
	)
}

// versionCmd 版本命令
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version.Info())
	},
}