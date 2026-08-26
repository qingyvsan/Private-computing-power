package heartbeat

import (
	"runtime"

	pb "computing-power/proto/v1"
)

// Collector 资源采集器
// 采集 CPU/GPU/内存/磁盘/网络指标
type Collector struct {
	reportGPU     bool
	reportNetwork bool
}

// NewCollector 创建资源采集器
func NewCollector(reportGPU, reportNetwork bool) *Collector {
	return &Collector{
		reportGPU:     reportGPU,
		reportNetwork: reportNetwork,
	}
}

// Collect 采集当前节点资源状态
func (c *Collector) Collect() *pb.NodeResources {
	// TODO(P4/P5): 接入系统指标采集
	// 当前返回基础占位数据
	res := &pb.NodeResources{
		CPUCores:    float64(runtime.NumCPU()),
		CPUUsage:    0,
		MemoryBytes: 16 * 1024 * 1024 * 1024, // 占位 16GB
		MemoryUsed:  0,
		DiskBytes:   512 * 1024 * 1024 * 1024, // 占位 512GB
		DiskUsed:    0,
	}

	if c.reportGPU {
		// TODO(P5): 通过 nvidia-smi 或 HAMi 采集 GPU 指标
		res.GPUs = []*pb.GPUDevice{}
	}

	if c.reportNetwork {
		// TODO(P6): 通过 Nebula 隧道和探测采集网络指标
		res.Network = &pb.NetworkMetrics{
			RTTMs: 5,
			NATType: "unknown",
		}
	}

	return res
}