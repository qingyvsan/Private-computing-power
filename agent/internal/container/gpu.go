package container

import (
	"encoding/csv"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"

	pb "computing-power/proto/v1"
)

// DiscoverGPUs 通过 nvidia-smi 查询 GPU 设备列表
// nvidia-smi 不可用或未安装驱动时返回空切片（非 nil），不返回错误
// 错误被记录到日志，调用者无需处理
func DiscoverGPUs() ([]*pb.GPUDevice, error) {
	binary := findNvidiaSmi()
	if binary == "" {
		return []*pb.GPUDevice{}, nil
	}

	cmd := exec.Command(binary,
		"--query-gpu=index,uuid,name,memory.total,memory.free,utilization.gpu",
		"--format=csv,noheader,nounits",
	)
	stdout, err := cmd.Output()
	if err != nil {
		// nvidia-smi 存在但执行失败（驱动未加载等）
		log.Printf("gpu: nvidia-smi failed: %v", err)
		return []*pb.GPUDevice{}, nil
	}

	gpus, err := parseNvidiaSmiCSV(strings.NewReader(string(stdout)))
	if err != nil {
		log.Printf("gpu: parse nvidia-smi output: %v", err)
		return []*pb.GPUDevice{}, nil
	}
	return gpus, nil
}

// parseNvidiaSmiCSV 解析 nvidia-smi CSV 输出
// 输入格式: index,uuid,name,memory.total,memory.free,utilization.gpu
// 处理 BOM、[N/A] 值、部分行失败等异常
func parseNvidiaSmiCSV(reader io.Reader) ([]*pb.GPUDevice, error) {
	r := csv.NewReader(reader)
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1 // 允许行长度可变，手动校验
	r.LazyQuotes = true

	var gpus []*pb.GPUDevice
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("gpu: skip malformed csv line: %v", err)
			continue
		}
		if len(row) < 5 {
			continue
		}

		// 跳过 BOM 行（部分驱动版本 CSV 含 BOM）
		indexStr := strings.TrimLeft(row[0], "\ufeff")
		uuid := strings.TrimSpace(row[1])
		model := strings.TrimSpace(row[2])
		totalStr := strings.TrimSpace(row[3])
		freeStr := strings.TrimSpace(row[4])

		// 解析 memory.total
		totalMB := parseInt64(totalStr)
		// 解析 memory.free（[N/A] 时回退到 total）
		freeMB := parseInt64(freeStr)
		if freeMB == 0 {
			freeMB = totalMB
		}

		// 解析 utilization.gpu（第 6 列，可选）
		var computeUtil float64
		if len(row) >= 6 {
			utilStr := strings.TrimSpace(row[5])
			computeUtil = parseFloat64(utilStr)
		}

		_ = indexStr // index 仅用于调试，不存储
		gpus = append(gpus, &pb.GPUDevice{
			UUID:             uuid,
			Model:            model,
			MemoryTotalMB:    totalMB,
			MemoryUsedMB:     totalMB - freeMB,
			MemoryAvailMB:    freeMB,
			ComputeUtil:      computeUtil,
		})
	}

	if gpus == nil {
		gpus = []*pb.GPUDevice{}
	}
	return gpus, nil
}

// parseInt64 安全解析 int64，解析失败返回 0
func parseInt64(s string) int64 {
	// 移除可能的单位后缀和空格
	s = strings.TrimSpace(s)
	if s == "" || s == "[N/A]" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseFloat64 安全解析 float64，解析失败返回 0
func parseFloat64(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "[N/A]" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// findNvidiaSmi 搜索 nvidia-smi 二进制文件
// 优先搜索 PATH，再到常见安装路径查找
func findNvidiaSmi() string {
	// 先查 PATH
	path, err := exec.LookPath("nvidia-smi")
	if err == nil {
		return path
	}

	// 常见安装路径
	commonPaths := []string{
		"/usr/bin/nvidia-smi",
		"/usr/local/bin/nvidia-smi",
		"/usr/lib/nvidia-smi",
		"/opt/nvidia/bin/nvidia-smi",
		"/run/current-system/sw/bin/nvidia-smi",   // NixOS
		"/usr/local/nvidia/bin/nvidia-smi",         // macOS CUDA Toolkit
		"/Library/CUDA/bin/nvidia-smi",             // macOS CUDA
		"/usr/local/cuda/bin/nvidia-smi",           // macOS CUDA
	}
	for _, p := range commonPaths {
		if fileExists(p) {
			return p
		}
	}
	return ""
}