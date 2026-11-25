// limiter.go：控制认证昂贵计算的并发数、每分钟全局和来源尝试数，并限制保留的来源状态。

package auth

import (
	"sync"
	"time"
)

// attemptWindow 记录单个固定时间窗内的尝试次数和到期时刻。
type attemptWindow struct {
	count   int
	expires time.Time
}

// attemptLimiter 通过互斥锁限制并发、全局及来源次数和内存状态；来源仅使用传输层可信对端地址。
type attemptLimiter struct {
	mu     sync.Mutex
	peers  map[string]attemptWindow
	global attemptWindow
	active int
}

// admit 清理过期窗口并申请认证计算额度；成功返回的释放函数必须且仅能调用一次。
func (l *attemptLimiter) admit(peer string, now time.Time) (func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.peers == nil {
		l.peers = make(map[string]attemptWindow)
	}
	for key, window := range l.peers {
		if !now.Before(window.expires) {
			delete(l.peers, key)
		}
	}
	if !now.Before(l.global.expires) {
		l.global = attemptWindow{expires: now.Add(time.Minute)}
	}
	window, exists := l.peers[peer]
	if !exists {
		window.expires = now.Add(time.Minute)
	}
	if l.active >= 4 {
		return nil, &ThrottledError{RetryAfter: time.Second}
	}
	if l.global.count >= 30 {
		return nil, &ThrottledError{RetryAfter: l.global.expires.Sub(now)}
	}
	if window.count >= 5 {
		return nil, &ThrottledError{RetryAfter: window.expires.Sub(now)}
	}
	if !exists && len(l.peers) >= 1024 {
		return nil, &ThrottledError{RetryAfter: time.Minute}
	}
	window.count++
	l.peers[peer] = window
	l.global.count++
	l.active++
	return func() { l.mu.Lock(); l.active--; l.mu.Unlock() }, nil
}
