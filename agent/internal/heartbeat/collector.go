package heartbeat

import (
	"log"
	"net"
	"runtime"
	"time"

	pb "computing-power/proto/v1"

	"computing-power/agent/internal/container"
	"computing-power/agent/internal/nebula"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

// Collector 资源采集器
// 采集 CPU/GPU/内存/磁盘/网络指标
type Collector struct {
	reportGPU     bool
	reportNetwork bool
	nebulaMgr     *nebula.Manager
	schedulerAddr string // 用于 RTT 探测
}

// NewCollector 创建资源采集器
func NewCollector(reportGPU, reportNetwork bool, nebulaMgr *nebula.Manager, schedulerAddr string) *Collector {
	return &Collector{
		reportGPU:     reportGPU,
		reportNetwork: reportNetwork,
		nebulaMgr:     nebulaMgr,
		schedulerAddr: schedulerAddr,
	}
}

// Collect 采集当前节点资源状态
func (c *Collector) Collect() *pb.NodeResources {
	res := &pb.NodeResources{
		CPUCores: float64(runtime.NumCPU()),
	}

	// CPU 使用率
	if percent, err := cpu.Percent(0, false); err == nil && len(percent) > 0 {
		res.CPUUsage = percent[0] / 100.0
	} else if err != nil {
		log.Printf("collector: cpu percent: %v", err)
	}

	// 内存
	if vmem, err := mem.VirtualMemory(); err == nil {
		res.MemoryBytes = int64(vmem.Total)
		res.MemoryUsed = int64(vmem.Used)
	} else {
		log.Printf("collector: memory: %v", err)
		// 保底：使用硬编码值
		res.MemoryBytes = 16 * 1024 * 1024 * 1024
	}

	// 磁盘（当前工作目录所在分区）
	if usage, err := disk.Usage("."); err == nil {
		res.DiskBytes = int64(usage.Total)
		res.DiskUsed = int64(usage.Used)
	} else {
		log.Printf("collector: disk: %v", err)
		res.DiskBytes = 512 * 1024 * 1024 * 1024
	}

	// GPU
	if c.reportGPU {
		gpus, err := container.DiscoverGPUs()
		if err != nil {
			log.Printf("collector: GPU discovery error: %v", err)
		}
		res.GPUs = gpus
	}

	// 网络
	if c.reportNetwork {
		natType := "unknown"
		if c.nebulaMgr != nil {
			natType = c.nebulaMgr.GetNATType()
		}
		network := &pb.NetworkMetrics{
			RTTMs:   probeRTT(c.schedulerAddr),
			NATType: natType,
		}
		res.Network = network
	}

	return res
}

// probeRTT 向目标地址发起 TCP 连接探测，返回 RTT（毫秒）
// 地址为空或连接失败时返回 0
func probeRTT(addr string) float64 {
	if addr == "" {
		return 0
	}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return 0
	}
	conn.Close()
	return float64(time.Since(start).Microseconds()) / 1000.0
}