package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
			Name:               name,
			InviteCode:         inviteCode,
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

		// 保存 Nebula 证书和配置到本地
		if len(resp.NebulaCertificate) > 0 {
			nebulaDir := filepath.Join(os.Getenv("HOME"), ".cp", "nebula")
			if home := os.Getenv("HOME"); home == "" {
				nebulaDir = filepath.Join(os.Getenv("USERPROFILE"), ".cp", "nebula")
			}
			if err := os.MkdirAll(nebulaDir, 0700); err != nil {
				return fmt.Errorf("create nebula dir: %w", err)
			}

			files := map[string][]byte{
				"ca.crt":       resp.CACertificate,
				"node.crt":     resp.NebulaCertificate,
				"node.key":     resp.NebulaPrivateKey,
				"config.yaml":  []byte(resp.NebulaConfig),
			}
			for name, data := range files {
				perm := os.FileMode(0644)
				if name == "node.key" {
					perm = 0600
				}
				if err := os.WriteFile(filepath.Join(nebulaDir, name), data, perm); err != nil {
					return fmt.Errorf("write %s: %w", name, err)
				}
			}
			fmt.Printf("  Nebula 证书: %s\n", nebulaDir)
			fmt.Printf("  Nebula 状态: 已配置\n")
		} else {
			fmt.Printf("  Nebula 状态: 未配置（调度器未启用 Nebula）\n")
		}

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