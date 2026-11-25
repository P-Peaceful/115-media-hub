// limiter.go：按账号串行调度交互和后台读取，维护完成后间隔与共享冷却；等待由调用者承担，无常驻协程。

package provider115

import (
	"context"
	"sync"
	"time"
)

// accountGate 的队列和通知均由 Client.mu 保护；busy 覆盖响应体读取和关闭。
type accountGate struct {
	busy       bool
	foreground int
	next       time.Time
	cooldown   time.Time
	blocked    bool
	queue      []*waiter
	changed    chan struct{}
}
type waiter struct{ priority Priority }

// notifyLocked 唤醒账号所有等待者，只有选中的队首可以获得在途资格。
func (g *accountGate) notifyLocked() { close(g.changed); g.changed = make(chan struct{}) }

// selectedLocked 在等待后台请求存在时限制前台连续占用四次。
func (g *accountGate) selectedLocked() *waiter {
	var fg, bg *waiter
	for _, w := range g.queue {
		if w.priority == Background {
			if bg == nil {
				bg = w
			}
		} else if fg == nil {
			fg = w
		}
	}
	if bg != nil && (fg == nil || g.foreground >= 4) {
		return bg
	}
	return fg
}

// removeLocked 移除获准或已取消的队列项，避免取消残留阻塞后续调用。
func (g *accountGate) removeLocked(w *waiter) {
	for i, v := range g.queue {
		if v == w {
			g.queue = append(g.queue[:i], g.queue[i+1:]...)
			return
		}
	}
}

// acquire 等待账号单在途资格、完成后间隔及冷却；关闭和 context 取消均会清理队列。
func (c *Client) acquire(ctx context.Context, scope string, priority Priority) (*accountGate, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	g := c.accounts[scope]
	if g == nil {
		g = &accountGate{changed: make(chan struct{})}
		c.accounts[scope] = g
	}
	if len(g.queue) >= 256 {
		c.mu.Unlock()
		return nil, &Error{Kind: Temporary}
	}
	w := &waiter{priority: priority}
	g.queue = append(g.queue, w)
	g.notifyLocked()
	for {
		if err := ctx.Err(); err != nil {
			g.removeLocked(w)
			g.notifyLocked()
			c.mu.Unlock()
			return nil, err
		}
		if c.closed {
			g.removeLocked(w)
			g.notifyLocked()
			c.mu.Unlock()
			return nil, ErrClosed
		}
		ready := g.next
		if g.cooldown.After(ready) {
			ready = g.cooldown
		}
		chosen := !g.busy && !g.blocked && g.selectedLocked() == w
		delay := time.Until(ready)
		if chosen && delay <= 0 {
			g.removeLocked(w)
			g.busy = true
			if priority == Background {
				g.foreground = 0
			} else if g.foreground < 4 {
				g.foreground++
			}
			c.mu.Unlock()
			return g, nil
		}
		changed := g.changed
		var timer *time.Timer
		var tick <-chan time.Time
		if chosen {
			timer = time.NewTimer(delay)
			tick = timer.C
		}
		c.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-c.done:
		case <-changed:
		case <-tick:
		}
		if timer != nil {
			timer.Stop()
		}
		c.mu.Lock()
	}
}

// release 在响应处理结束后开始最小间隔，保证慢响应不会与下一请求重叠。
func (c *Client) release(g *accountGate) {
	c.mu.Lock()
	defer c.mu.Unlock()
	g.busy = false
	g.next = time.Now().Add(c.interval)
	g.notifyLocked()
}

// cool 只延长共享冷却；无法表示的 Retry-After 永久阻止本客户端账号发送，避免绕过限流。
func (c *Client) cool(g *accountGate, until time.Time, blocked bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if until.After(g.cooldown) {
		g.cooldown = until
	}
	g.blocked = g.blocked || blocked
	g.notifyLocked()
}

// waitContext 的计时器属于当前调用，取消或客户端关闭时立即回收。
func (c *Client) waitContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return ErrClosed
	case <-timer.C:
		return nil
	}
}

// clientState 保存所有账号的准入状态；禁止包级可变 limiter。
type clientState struct {
	mu       sync.Mutex
	accounts map[string]*accountGate
	closed   bool
	done     chan struct{}
}

// ObserveThrottle 把服务已确认的业务限流并入同账号冷却；返回受控的最早重试时间。
// retryAfter 为空时使用 30 秒，无法表示的长值会阻止本客户端账号后续发送。
func (c *Client) ObserveThrottle(scope, retryAfterValue string) time.Time {
	until, blocked := retryAfter(retryAfterValue, time.Now())
	c.mu.Lock()
	g := c.accounts[scope]
	if g == nil {
		g = &accountGate{changed: make(chan struct{})}
		c.accounts[scope] = g
	}
	c.mu.Unlock()
	c.cool(g, until, blocked)
	return until
}
