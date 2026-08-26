package commands

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	pb "computing-power/proto/v1"

	"computing-power/cli/internal/client"
)

// jobCmd 作业管理命令
var jobCmd = &cobra.Command{
	Use:   "job",
	Short: "作业管理",
}

// jobSpec 用于解析 job.yaml 的中间结构
type jobSpec struct {
	Name          string           `yaml:"name"`
	Type          string           `yaml:"type"`
	Image         string           `yaml:"image"`
	Resources     resourceSpecYAML `yaml:"resources"`
	Stages        []stageSpecYAML  `yaml:"stages"`
	FailurePolicy string           `yaml:"failure_policy"`
	MaxRetries    int              `yaml:"max_retries"`
	MaxDuration   string           `yaml:"max_duration"`
	OwnerID       string           `yaml:"owner_id"`
}

type resourceSpecYAML struct {
	CPU     float64 `yaml:"cpu"`
	Memory  string  `yaml:"memory"`
	GPU     string  `yaml:"gpu"`
	Disk    string  `yaml:"disk"`
}

type stageSpecYAML struct {
	Name       string          `yaml:"name"`
	DependsOn  []string        `yaml:"depends_on"`
	Image      string          `yaml:"image"`
	Resources  resourceSpecYAML `yaml:"resources"`
	Split      *splitYAML      `yaml:"split"`
	Inputs     []string        `yaml:"inputs"`
	Outputs    []string        `yaml:"outputs"`
	MaxConcurrency int         `yaml:"max_concurrency"`
}

type splitYAML struct {
	Strategy string   `yaml:"strategy"`
	Input    string   `yaml:"input"`
	FileList []string `yaml:"file_list"`
	Start    int64    `yaml:"start"`
	End      int64    `yaml:"end"`
	NumParts int32    `yaml:"num_parts"`
	Script   string   `yaml:"script"`
	Args     []string `yaml:"args"`
}

// parseMemory 解析内存字符串 "16GB" -> 字节
func parseMemory(s string) int64 {
	var value int64
	var unit string
	fmt.Sscanf(s, "%d%s", &value, &unit)
	switch unit {
	case "KB", "kb":
		return value * 1024
	case "MB", "mb":
		return value * 1024 * 1024
	case "GB", "gb":
		return value * 1024 * 1024 * 1024
	case "TB", "tb":
		return value * 1024 * 1024 * 1024 * 1024
	default:
		return value
	}
}

// parseDurationMS 解析时长字符串到毫秒
func parseDurationMS(s string) int64 {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d.Milliseconds()
}

// parseJobSpec 将 job.yaml 转换为 proto Job
func parseJobSpec(data []byte, ownerID string) (*pb.Job, error) {
	var spec jobSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse job.yaml: %w", err)
	}

	if ownerID == "" {
		ownerID = spec.OwnerID
	}

	job := &pb.Job{
		Name:          spec.Name,
		OwnerID:       ownerID,
		Image:         spec.Image,
		FailurePolicy: spec.FailurePolicy,
		MaxRetries:    int32(spec.MaxRetries),
		MaxDurationMs: parseDurationMS(spec.MaxDuration),
	}

	// 设置类型
	switch spec.Type {
	case "single", "":
		job.Type = pb.JobTypeSingle
	case "aggregate":
		job.Type = pb.JobTypeAggregate
	case "workflow":
		job.Type = pb.JobTypeWorkflow
	default:
		return nil, fmt.Errorf("unknown job type: %s", spec.Type)
	}

	// 顶层资源
	if spec.Resources.CPU > 0 || spec.Resources.Memory != "" || spec.Resources.GPU != "" {
		job.Resources = toProtoResource(spec.Resources)
	}

	// Stage（workflow 或 aggregate）
	for i, stageSpec := range spec.Stages {
		stage := &pb.Stage{
			ID:             fmt.Sprintf("stage-%s-%d", spec.Name, i),
			Name:           stageSpec.Name,
			DependsOn:      stageSpec.DependsOn,
			Inputs:         stageSpec.Inputs,
			Outputs:        stageSpec.Outputs,
			MaxConcurrency: int32(stageSpec.MaxConcurrency),
			Resources:      toProtoResource(stageSpec.Resources),
		}
		if stageSpec.Split != nil {
			stage.Split = toProtoSplit(stageSpec.Split)
		}
		job.Stages = append(job.Stages, stage)
	}

	return job, nil
}

// toProtoResource 转换资源规格
func toProtoResource(r resourceSpecYAML) *pb.ResourceSpec {
	rs := &pb.ResourceSpec{
		CPUCores: r.CPU,
	}
	if r.Memory != "" {
		rs.MemoryBytes = parseMemory(r.Memory)
	}
	if r.GPU != "" {
		var memMB int64
		fmt.Sscanf(r.GPU, "%dGB", &memMB)
		rs.GPUs = []*pb.GPURequest{
			{MemoryMB: memMB * 1024, Cores: 100, Count: 1},
		}
	}
	return rs
}

// toProtoSplit 转换拆分策略
func toProtoSplit(s *splitYAML) *pb.SplitStrategy {
	switch s.Strategy {
	case "by_file":
		return &pb.SplitStrategy{
			Type: pb.SplitTypeByFile,
			ByFile: &pb.ByFileSplit{
				InputPattern: s.Input,
				FileList:     s.FileList,
			},
		}
	case "by_range":
		return &pb.SplitStrategy{
			Type: pb.SplitTypeByRange,
			ByRange: &pb.ByRangeSplit{
				Start:    s.Start,
				End:      s.End,
				NumParts: s.NumParts,
			},
		}
	case "by_n":
		return &pb.SplitStrategy{
			Type: pb.SplitTypeByN,
			ByN:  &pb.ByNSplit{NumParts: s.NumParts},
		}
	case "by_custom":
		return &pb.SplitStrategy{
			Type: pb.SplitTypeByCustom,
			ByCustom: &pb.ByCustomSplit{
				Script: s.Script,
				Args:   s.Args,
			},
		}
	default:
		return &pb.SplitStrategy{Type: pb.SplitTypeByN, ByN: &pb.ByNSplit{NumParts: 1}}
	}
}

// jobSubmitCmd 提交作业
var jobSubmitCmd = &cobra.Command{
	Use:   "submit <job.yaml>",
	Short: "提交作业",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("read job file: %w", err)
		}

		ownerID, _ := cmd.Flags().GetString("owner")
		job, err := parseJobSpec(data, ownerID)
		if err != nil {
			return err
		}

		c, err := client.New(client.Config{Address: schedulerAddr})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.SubmitJob(context.Background(), &pb.SubmitJobRequest{Job: job})
		if err != nil {
			return fmt.Errorf("submit job: %w", err)
		}

		fmt.Printf("作业已提交: %s (状态: %s)\n", resp.JobID, resp.Status)
		fmt.Printf("  消息: %s\n", resp.Message)
		return nil
	},
}

// jobListCmd 列出作业
var jobListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出我的作业",
	RunE: func(cmd *cobra.Command, args []string) error {
		ownerID, _ := cmd.Flags().GetString("owner")

		c, err := client.New(client.Config{Address: schedulerAddr})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.ListJobs(context.Background(), &pb.ListJobsRequest{NodeID: ownerID})
		if err != nil {
			return fmt.Errorf("list jobs: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(resp.Jobs)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tTYPE\tSTATUS\tOWNER")
		for _, j := range resp.Jobs {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				j.ID, j.Name, j.Type, j.Status, j.OwnerID)
		}
		w.Flush()
		return nil
	},
}

// jobStatusCmd 查看作业状态
var jobStatusCmd = &cobra.Command{
	Use:   "status <job-id>",
	Short: "查看作业状态",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.New(client.Config{Address: schedulerAddr})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.GetJob(context.Background(), &pb.GetJobRequest{JobID: args[0]})
		if err != nil {
			return fmt.Errorf("get job: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(resp.Job)
		}

		j := resp.Job
		fmt.Printf("ID:        %s\n", j.ID)
		fmt.Printf("Name:      %s\n", j.Name)
		fmt.Printf("Type:      %s\n", j.Type)
		fmt.Printf("Status:    %s\n", j.Status)
		fmt.Printf("Owner:     %s\n", j.OwnerID)
		if len(j.Stages) > 0 {
			fmt.Printf("Stages:\n")
			for _, s := range j.Stages {
				fmt.Printf("  - %s (status: %s)\n", s.Name, s.Status)
			}
		}
		return nil
	},
}

// jobCancelCmd 取消作业
var jobCancelCmd = &cobra.Command{
	Use:   "cancel <job-id>",
	Short: "取消作业",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ownerID, _ := cmd.Flags().GetString("owner")

		c, err := client.New(client.Config{Address: schedulerAddr})
		if err != nil {
			return err
		}
		defer c.Close()

		resp, err := c.CancelJob(context.Background(), &pb.CancelJobRequest{
			JobID:  args[0],
			NodeID: ownerID,
		})
		if err != nil {
			return fmt.Errorf("cancel job: %w", err)
		}
		if resp.Success {
			fmt.Printf("作业 %s 已取消\n", args[0])
		}
		return nil
	},
}

func init() {
	jobCmd.AddCommand(jobSubmitCmd, jobListCmd, jobStatusCmd, jobCancelCmd)
	for _, cmd := range []*cobra.Command{jobSubmitCmd, jobListCmd, jobCancelCmd} {
		cmd.Flags().String("owner", "", "作业所有者节点 ID")
	}
}