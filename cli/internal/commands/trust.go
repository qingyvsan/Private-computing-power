package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	pb "computing-power/proto/v1"

	"computing-power/cli/internal/client"
	"computing-power/pkg/trustgraph"
)

// trustCmd 信任管理命令
var trustCmd = &cobra.Command{
	Use:   "trust",
	Short: "信任关系管理",
}

// trustListCmd 列出信任图
var trustListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看信任图",
	RunE: func(cmd *cobra.Command, args []string) error {
		fromNodeID, _ := cmd.Flags().GetString("from")

		c, err := client.New(client.Config{Address: schedulerAddr})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.GetTrustGraph(context.Background(), &pb.GetTrustGraphRequest{
			NodeID: fromNodeID,
		})
		if err != nil {
			return fmt.Errorf("get trust graph: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(resp.Edges)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "FROM\tTO\tEXPIRES")
		for _, e := range resp.Edges {
			fmt.Fprintf(w, "%s\t%s\t%d\n", e.FromNode, e.ToNode, e.ExpiresAt)
		}
		w.Flush()
		return nil
	},
}

// trustAddCmd 添加信任关系
var trustAddCmd = &cobra.Command{
	Use:   "add <node-id>",
	Short: "添加信任关系",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fromNodeID, _ := cmd.Flags().GetString("from")
		if fromNodeID == "" {
			return fmt.Errorf("--from is required (the node declaring trust)")
		}
		expiresIn, _ := cmd.Flags().GetDuration("expires")
		targetNodeID := args[0]

		// 加载本地私钥
		keyPath := filepath.Join(getCpDir(), "keys", "ecdsa.pem")
		key, err := trustgraph.LoadPrivateKey(keyPath)
		if err != nil {
			return fmt.Errorf("load private key (run 'cpcli register' first): %w", err)
		}

		// 生成签名
		sig, err := trustgraph.SignTrust(key, fromNodeID, targetNodeID)
		if err != nil {
			return fmt.Errorf("sign trust declaration: %w", err)
		}

		var expiresAt int64
		if expiresIn > 0 {
			expiresAt = time.Now().Add(expiresIn).UnixMilli()
		}

		c, err := client.New(client.Config{Address: schedulerAddr})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
			FromNodeID:   fromNodeID,
			TargetNodeID: targetNodeID,
			Signature:    sig,
			ExpiresAt:    expiresAt,
		})
		if err != nil {
			return fmt.Errorf("declare trust: %w", err)
		}
		if resp.Success {
			fmt.Printf("已信任节点 %s (from %s)\n", targetNodeID, fromNodeID)
		}
		return nil
	},
}

// trustRemoveCmd 移除信任关系
var trustRemoveCmd = &cobra.Command{
	Use:   "remove <node-id>",
	Short: "移除信任关系",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fromNodeID, _ := cmd.Flags().GetString("from")
		if fromNodeID == "" {
			return fmt.Errorf("--from is required (the node revoking trust)")
		}
		targetNodeID := args[0]

		// 加载本地私钥
		keyPath := filepath.Join(getCpDir(), "keys", "ecdsa.pem")
		key, err := trustgraph.LoadPrivateKey(keyPath)
		if err != nil {
			return fmt.Errorf("load private key: %w", err)
		}

		// 生成签名
		sig, err := trustgraph.SignTrust(key, fromNodeID, targetNodeID)
		if err != nil {
			return fmt.Errorf("sign trust revocation: %w", err)
		}

		c, err := client.New(client.Config{Address: schedulerAddr})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.RevokeTrust(context.Background(), &pb.RevokeTrustRequest{
			FromNodeID:   fromNodeID,
			TargetNodeID: targetNodeID,
			Signature:    sig,
		})
		if err != nil {
			return fmt.Errorf("revoke trust: %w", err)
		}
		if resp.Success {
			fmt.Printf("已解除对节点 %s 的信任\n", targetNodeID)
		}
		return nil
	},
}

func init() {
	trustCmd.AddCommand(trustListCmd, trustAddCmd, trustRemoveCmd)
	trustListCmd.Flags().String("from", "", "按声明节点过滤")
	trustAddCmd.Flags().String("from", "", "声明信任的节点 ID")
	trustAddCmd.Flags().Duration("expires", 0, "信任到期时间（如 72h）")
	trustRemoveCmd.Flags().String("from", "", "撤销信任的节点 ID")
}