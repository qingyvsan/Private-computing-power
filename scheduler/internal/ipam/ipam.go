package ipam

import (
	"fmt"
	"net"
	"sync"

	"computing-power/scheduler/internal/store"
)

// IPAM 管理 Overlay IP 地址分配（线程安全）
type IPAM struct {
	store   *store.Store
	network *net.IPNet
	gateway net.IP
	start   net.IP
	mu      sync.Mutex
}

// NewIPAM 创建 IPAM 管理器
// cidr: 如 "10.1.0.0/16"
// gatewayIP: 网关/Lighthouse 用 IP，如 "10.1.0.1"
func NewIPAM(st *store.Store, cidr, gatewayIP string) (*IPAM, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse cidr %s: %w", cidr, err)
	}

	gateway := net.ParseIP(gatewayIP)
	if gateway == nil {
		return nil, fmt.Errorf("invalid gateway IP: %s", gatewayIP)
	}

	if !network.Contains(gateway) {
		return nil, fmt.Errorf("gateway %s is not in network %s", gatewayIP, cidr)
	}

	ipam := &IPAM{
		store:   st,
		network: network,
		gateway: gateway,
	}

	// 计算起始分配 IP（gateway + 1）
	ipam.start = nextIP(gateway)

	return ipam, nil
}

// Allocate 为节点分配 Overlay IP
// 如果节点已有分配则返回原有 IP
func (m *IPAM) Allocate(nodeID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已有分配
	existing, err := m.store.GetIPAllocation(nodeID)
	if err != nil {
		return "", fmt.Errorf("get existing allocation: %w", err)
	}
	if existing != "" {
		return existing, nil
	}

	// 获取所有已分配 IP
	allocations, err := m.store.ListIPAllocations()
	if err != nil {
		return "", fmt.Errorf("list allocations: %w", err)
	}

	allocated := make(map[string]bool)
	for _, ip := range allocations {
		allocated[ip] = true
	}
	// 保留 gateway IP
	allocated[m.gateway.String()] = true

	// 找到第一个可用 IP
	ip := m.start
	for {
		ipStr := ip.String()
		if !allocated[ipStr] {
			// 保存分配
			if err := m.store.SaveIPAllocation(nodeID, ipStr); err != nil {
				return "", fmt.Errorf("save allocation: %w", err)
			}
			return ipStr, nil
		}
		next := nextIP(ip)
		if next == nil || !m.network.Contains(next) {
			return "", fmt.Errorf("no available IP addresses in %s", m.network.String())
		}
		ip = next
	}
}

// Release 释放节点的 Overlay IP
func (m *IPAM) Release(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.DeleteIPAllocation(nodeID)
}

// Gateway 返回网关/Lighthouse 的 IP
func (m *IPAM) Gateway() string {
	return m.gateway.String()
}

// Network 返回网络 CIDR
func (m *IPAM) Network() string {
	return m.network.String()
}

// nextIP 返回 IP 的下一个地址（递增最后 8 位）
func nextIP(ip net.IP) net.IP {
	n := ip.To4()
	if n == nil {
		return nil
	}
	next := make(net.IP, 4)
	copy(next, n)
	// 递增 IPv4 地址
	for i := 3; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}
	return next
}