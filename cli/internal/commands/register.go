package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	pb "computing-power/proto/v1"

	"computing-power/cli/internal/client"
)

// registerCmd 注册节点
var registerCmd = &cobra.Command{
	Use:   "register [name]",
	Short: "注册本节点到调度器",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := ""
		if len(args) > 0 {
			name = args[0]
		}

		inviteCode, _ := cmd.Flags().GetString("invite")
		fingerprint := getHostFingerprint()

		req := &pb.RegisterNodeRequest{
			Name:    name,
			InviteCode: inviteCode,
			HardwareFingerprint: fingerprint,
		}

		c, err := client.New(client.Config{Address: schedulerAddr})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.RegisterNode(context.Background(), req)
		if err != nil {
			return fmt.Errorf("register: %w", err)
		}

		fmt.Printf("节点注册成功!\n")
		fmt.Printf("  节点 ID:   %s\n", resp.NodeID)
		fmt.Printf("  Overlay IP: %s\n", resp.OverlayIP)

		// TODO(P6): 保存 Nebula 证书和配置到本地
		return nil
	},
}

// getHostFingerprint 生成主机指纹
func getHostFingerprint() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

func init() {
	registerCmd.Flags().String("invite", "", "邀请码（已有网络时必填）")
}