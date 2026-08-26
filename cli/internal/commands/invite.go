package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	pb "computing-power/proto/v1"

	"computing-power/cli/internal/client"
)

// inviteCmd 邀请码管理命令
var inviteCmd = &cobra.Command{
	Use:   "invite",
	Short: "邀请码管理",
}

// inviteCreateCmd 创建邀请码
var inviteCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建邀请码",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeID, _ := cmd.Flags().GetString("node")

		c, err := client.New(client.Config{Address: schedulerAddr})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.CreateInviteCode(context.Background(), &pb.CreateInviteCodeRequest{
			NodeID: nodeID,
		})
		if err != nil {
			return fmt.Errorf("create invite code: %w", err)
		}

		fmt.Printf("邀请码: %s\n", resp.Code)
		fmt.Printf("过期时间: %d\n", resp.ExpiresAt)
		return nil
	},
}

// inviteRedeemCmd 兑换邀请码
var inviteRedeemCmd = &cobra.Command{
	Use:   "redeem <code>",
	Short: "兑换邀请码",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeID, _ := cmd.Flags().GetString("node")

		c, err := client.New(client.Config{Address: schedulerAddr})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.RedeemInviteCode(context.Background(), &pb.RedeemInviteCodeRequest{
			Code:   args[0],
			NodeID: nodeID,
		})
		if err != nil {
			return fmt.Errorf("redeem invite code: %w", err)
		}
		if resp.Valid {
			fmt.Printf("邀请码有效: %s\n", resp.Message)
		} else {
			fmt.Printf("邀请码无效: %s\n", resp.Message)
		}
		return nil
	},
}

func init() {
	inviteCmd.AddCommand(inviteCreateCmd, inviteRedeemCmd)
	inviteCreateCmd.Flags().String("node", "", "节点 ID")
	inviteRedeemCmd.Flags().String("node", "", "节点 ID")
}