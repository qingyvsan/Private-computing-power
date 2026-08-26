package commands

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	pb "computing-power/proto/v1"

	"computing-power/cli/internal/client"
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
		c, err := client.New(client.Config{Address: schedulerAddr})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.GetTrustGraph(context.Background(), &pb.GetTrustGraphRequest{})
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
		// TODO(P7): 生成 ECDSA 签名后提交
		c, err := client.New(client.Config{Address: schedulerAddr})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.DeclareTrust(context.Background(), &pb.DeclareTrustRequest{
			TargetNodeID: args[0],
		})
		if err != nil {
			return fmt.Errorf("declare trust: %w", err)
		}
		if resp.Success {
			fmt.Printf("已信任节点 %s\n", args[0])
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
		c, err := client.New(client.Config{Address: schedulerAddr})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.RevokeTrust(context.Background(), &pb.RevokeTrustRequest{
			TargetNodeID: args[0],
		})
		if err != nil {
			return fmt.Errorf("revoke trust: %w", err)
		}
		if resp.Success {
			fmt.Printf("已解除对节点 %s 的信任\n", args[0])
		}
		return nil
	},
}

func init() {
	trustCmd.AddCommand(trustListCmd, trustAddCmd, trustRemoveCmd)
}