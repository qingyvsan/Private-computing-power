package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	pb "computing-power/proto/v1"

	"computing-power/cli/internal/client"
)

// nodeCmd 节点管理命令
var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "节点管理",
}

// nodeListCmd 列出节点
var nodeListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有节点",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.New(client.Config{Address: schedulerAddr, CACert: caCert, TLSCert: tlsCert, TLSKey: tlsKey})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.ListNodes(context.Background(), &pb.ListNodesRequest{})
		if err != nil {
			return fmt.Errorf("list nodes: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(resp.Nodes)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSTATUS\tCPU\tMEM\tGPU\tTASKS\tPHI\tSAMPLES")
		for _, n := range resp.Nodes {
			cpu := "N/A"
			mem := "N/A"
			gpu := "0"
			if n.Resources != nil {
				cpu = fmt.Sprintf("%.0f/%.0f", n.Resources.CPUUsage*100, n.Resources.CPUCores)
				mem = fmt.Sprintf("%.0f%%", float64(n.Resources.MemoryUsed)/float64(n.Resources.MemoryBytes)*100)
				gpu = fmt.Sprintf("%d", len(n.Resources.GPUs))
			}
			phi := fmt.Sprintf("%.2f", n.PhiValue)
			samples := fmt.Sprintf("%d", n.HeartbeatSampleCount)
			if n.HeartbeatSampleCount == 0 {
				samples = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
				n.ID, n.Name, n.Status, cpu, mem, gpu, n.CurrentTasks, phi, samples)
		}
		w.Flush()
		return nil
	},
}

// nodeStatusCmd 查看节点详情
var nodeStatusCmd = &cobra.Command{
	Use:   "status <node-id>",
	Short: "查看节点详情",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.New(client.Config{Address: schedulerAddr, CACert: caCert, TLSCert: tlsCert, TLSKey: tlsKey})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.GetNode(context.Background(), &pb.GetNodeRequest{NodeID: args[0]})
		if err != nil {
			return fmt.Errorf("get node: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(resp.Node)
		}

		n := resp.Node
		fmt.Printf("ID:           %s\n", n.ID)
		fmt.Printf("Name:         %s\n", n.Name)
		fmt.Printf("Status:       %s\n", n.Status)
		fmt.Printf("Phi Value:    %.2f (threshold=4.0)\n", n.PhiValue)
		fmt.Printf("Samples:      %d\n", n.HeartbeatSampleCount)
		fmt.Printf("Overlay IP:   %s\n", n.OverlayIP)
		fmt.Printf("Version:      %s\n", n.Version)
		fmt.Printf("Reputation:   %.2f\n", n.Reputation)
		fmt.Printf("Tasks:        %d/%d\n", n.CurrentTasks, n.MaxTasks)
		if n.Resources != nil {
			fmt.Printf("CPU:          %.0f cores (%.0f%% used)\n", n.Resources.CPUCores, n.Resources.CPUUsage*100)
			fmt.Printf("Memory:       %d / %d bytes\n", n.Resources.MemoryUsed, n.Resources.MemoryBytes)
			fmt.Printf("GPUs:         %d\n", len(n.Resources.GPUs))
		}
		return nil
	},
}

// printJSON 输出 JSON 格式
func printJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// nodeUnregisterCmd 注销节点
var nodeUnregisterCmd = &cobra.Command{
	Use:   "unregister <node-id>",
	Short: "注销并删除节点",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.New(client.Config{Address: schedulerAddr, CACert: caCert, TLSCert: tlsCert, TLSKey: tlsKey})
		if err != nil {
			return err
		}
		defer c.Close()

		reason, _ := cmd.Flags().GetString("reason")

		resp, err := c.UnregisterNode(context.Background(), &pb.UnregisterNodeRequest{
			NodeID: args[0],
			Reason: reason,
		})
		if err != nil {
			return fmt.Errorf("unregister node: %w", err)
		}

		fmt.Printf("Node %s unregistered (success=%v)\n", args[0], resp.Success)
		return nil
	},
}

func init() {
	nodeCmd.AddCommand(nodeListCmd, nodeStatusCmd, nodeUnregisterCmd)
	nodeUnregisterCmd.Flags().String("reason", "manual", "注销原因")
}