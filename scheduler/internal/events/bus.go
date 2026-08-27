package events

import (
	"sync"

	pb "computing-power/proto/v1"
)

// Bus 是内存事件总线，支持按 JobID 的 Pub/Sub
type Bus struct {
	mu          sync.RWMutex
	subscribers map[string]map[string]chan<- *pb.JobEvent // jobID -> subscriberID -> ch
}

// NewBus 创建事件总线
func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[string]map[string]chan<- *pb.JobEvent),
	}
}

// Subscribe 订阅指定 Job 的事件，返回接收 channel
// subscriberID 用于取消订阅时区分多个订阅者
func (b *Bus) Subscribe(jobID, subscriberID string) chan *pb.JobEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan *pb.JobEvent, 64)
	if _, ok := b.subscribers[jobID]; !ok {
		b.subscribers[jobID] = make(map[string]chan<- *pb.JobEvent)
	}
	b.subscribers[jobID][subscriberID] = ch
	return ch
}

// Unsubscribe 取消订阅
func (b *Bus) Unsubscribe(jobID, subscriberID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subs, ok := b.subscribers[jobID]; ok {
		if ch, ok := subs[subscriberID]; ok {
			close(ch)
			delete(subs, subscriberID)
		}
		if len(subs) == 0 {
			delete(b.subscribers, jobID)
		}
	}
}

// Publish 发布事件到所有订阅者
// 非阻塞：如果订阅者 channel 已满则跳过该订阅者
func (b *Bus) Publish(event *pb.JobEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	subs, ok := b.subscribers[event.JobID]
	if !ok {
		return
	}
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// 订阅者消费过慢，跳过防止阻塞
		}
	}
}