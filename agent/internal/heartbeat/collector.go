package heartbeat

import (
	"log"
	"runtime"

	pb "computing-power/proto/v1"

	"computing-power/agent/internal/container"
	"computing-power/agent/internal/nebula"
)

// Collector 资源采集器
// 采集 CPU/GPU/内存/磁盘/网络指标
type Collector struct {
	reportGPU     bool
	reportNetwork bool
	nebulaMgr     *nebula.Manager
}

// NewCollector 创建资源采集器
func NewCollector(reportGPU, reportNetwork bool, nebulaMgr *nebula.Manager) *Collector {
	return &Collector{
		reportGPU:     reportGPU,
		reportNetwork: reportNetwork,
		nebulaMgr:     nebulaMgr,
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
		gpus, err := container.DiscoverGPUs()
		if err != nil {
			log.Printf("collector: GPU discovery error: %v", err)
		}
		res.GPUs = gpus
	}

	if c.reportNetwork {
		natType := "unknown"
		if c.nebulaMgr != nil {
			natType = c.nebulaMgr.GetNATType()
		}
		res.Network = &pb.NetworkMetrics{
			RTTMs:   5,
			NATType: natType,
		}
	}

	return res
}