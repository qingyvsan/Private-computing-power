package phidetector

import (
	"testing"
	"time"
)

func TestDetectorHealthyNode(t *testing.T) {
	d := NewDetector(1000, 100, 4.0)

	now := time.Now()
	// 模拟 200 个正常心跳，间隔 3 秒
	for i := 0; i < 200; i++ {
		now = now.Add(3 * time.Second)
		d.ReportHeartbeat(now)
	}

	// 3 秒后检查，应处于健康状态
	phi := d.Phi(now.Add(3 * time.Second))
	if phi >= 4.0 {
		t.Fatalf("expected healthy node phi < 4, got %v", phi)
	}
}

func TestDetectorCrashedNode(t *testing.T) {
	d := NewDetector(1000, 100, 4.0)

	now := time.Now()
	// 模拟 200 个正常心跳
	for i := 0; i < 200; i++ {
		now = now.Add(3 * time.Second)
		d.ReportHeartbeat(now)
	}

	// 30 秒没有心跳，应判定为故障
	phi := d.Phi(now.Add(30 * time.Second))
	if phi < 4.0 {
		t.Fatalf("expected crashed node phi >= 4, got %v", phi)
	}
}

func TestDetectorJitteryNode(t *testing.T) {
	d := NewDetector(1000, 50, 4.0)

	now := time.Now()
	// 模拟心跳抖动：大部分 3 秒，偶发 10 秒
	for i := 0; i < 200; i++ {
		now = now.Add(3 * time.Second)
		if i%20 == 0 {
			now = now.Add(7 * time.Second)
		}
		d.ReportHeartbeat(now)
	}

	// 5 秒后检查（略高于正常间隔），应仍健康
	phi := d.Phi(now.Add(5 * time.Second))
	if phi >= 4.0 {
		t.Fatalf("jittery node should still be healthy at 5s, got phi %v", phi)
	}
}

func TestDetectorNotEnoughSamples(t *testing.T) {
	d := NewDetector(1000, 100, 4.0)

	now := time.Now()
	// 只有 5 个心跳，样本不足
	for i := 0; i < 5; i++ {
		now = now.Add(3 * time.Second)
		d.ReportHeartbeat(now)
	}

	// 即使很久没有心跳，也返回 0（样本不足时信任）
	phi := d.Phi(now.Add(1 * time.Minute))
	if phi != 0 {
		t.Fatalf("expected phi 0 with insufficient samples, got %v", phi)
	}
}

func TestManager(t *testing.T) {
	m := NewManager(1000, 100, 4.0)

	nodeA := time.Now()
	// 节点 A 正常心跳（带轻微抖动，保证方差 > 0）
	for i := 0; i < 200; i++ {
		nodeA = nodeA.Add(3 * time.Second)
		if i%10 == 0 {
			nodeA = nodeA.Add(100 * time.Millisecond) // 轻微抖动
		}
		m.ReportHeartbeat("node-a", nodeA)
	}

	// 节点 A 在 3 秒后检查，应可用
	if !m.IsAvailable("node-a", nodeA.Add(3*time.Second)) {
		t.Fatal("node-a should be available")
	}
	// node-a 30 秒无心跳应不可用
	if m.IsAvailable("node-a", nodeA.Add(30*time.Second)) {
		t.Fatal("node-a should be unavailable after 30s silence")
	}

	// 节点 B 独立心跳流（样本不足）
	nodeB := time.Now().Add(10 * time.Minute) // 与 node-a 时间线隔离
	for i := 0; i < 5; i++ {
		nodeB = nodeB.Add(3 * time.Second)
		m.ReportHeartbeat("node-b", nodeB)
	}
	// 节点 B 样本不足，应视为可用
	if !m.IsAvailable("node-b", nodeB.Add(1*time.Minute)) {
		t.Fatal("node-b with insufficient samples should be available")
	}
}
