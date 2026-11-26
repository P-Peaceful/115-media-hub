// stream.go：为运行日志的服务端事件流管理有界连接与投递，断开无法跟上数据的订阅者。

package logs

import (
	"sync"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/logs"
)

const (
	maxSubscribers = 16
	streamBuffer   = 128
)

// Event 从已提交的记录或清理事务生成，只设置对应的一个字段。
type Event struct {
	Entry   *domain.Entry
	Cleared *ClearResult
}

type subscriber struct {
	filter domain.Filter
	events chan Event
}

type broker struct {
	mu          sync.Mutex
	stopping    bool
	subscribers map[*subscriber]struct{}
}

// Subscribe 创建有界的实时订阅；调用者结束读取时应释放订阅。
func (s *Service) Subscribe(filter domain.Filter) (<-chan Event, func(), error) {
	b := &s.streams
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopping || len(b.subscribers) >= maxSubscribers {
		return nil, nil, ErrUnavailable
	}
	sub := &subscriber{filter: filter, events: make(chan Event, streamBuffer)}
	b.subscribers[sub] = struct{}{}
	return sub.events, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, exists := b.subscribers[sub]; exists {
			b.disconnect(sub)
		}
	}, nil
}

// StopStreams 关闭实时流并阻止新订阅，采集和刷盘仍可继续处理服务尾部日志。
func (s *Service) StopStreams() {
	b := &s.streams
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopping = true
	for sub := range b.subscribers {
		b.disconnect(sub)
	}
}

// publish 向匹配订阅者非阻塞投递；缓冲溢出时断开订阅，浏览器可重连并重新获取历史。
func (b *broker) publish(event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for sub := range b.subscribers {
		if event.Entry != nil && !sub.filter.Matches(*event.Entry) {
			continue
		}
		select {
		case sub.events <- event:
		default:
			b.disconnect(sub)
		}
	}
}

// disconnect 在持有订阅锁时关闭连接，丢弃积压事件让连接立即重新同步。
func (b *broker) disconnect(sub *subscriber) {
	delete(b.subscribers, sub)
	for {
		select {
		case <-sub.events:
		default:
			close(sub.events)
			return
		}
	}
}
