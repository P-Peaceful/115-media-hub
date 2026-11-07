package auth

import (
	"sync"
	"time"
)

type attemptWindow struct {
	count   int
	expires time.Time
}

// attemptLimiter bounds expensive work, aggregate attempts and retained peer state.
// It deliberately uses the transport's trusted peer address, never usernames or forwarded headers.
type attemptLimiter struct {
	mu     sync.Mutex
	peers  map[string]attemptWindow
	global attemptWindow
	active int
}

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
