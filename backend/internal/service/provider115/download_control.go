// download_control.go：管理附属内容传输的账号级双并发、有界等待、共享 CDN 冷却及凭据变更取消。
// 状态隶属服务实例，不使用包级可变限流器或常驻后台协程。

package provider115

import (
	"context"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// downloadState 保护所有账号下载配额；resolve 仅供包内协议边界测试注入，不接受用户配置。
type downloadState struct {
	mu       sync.Mutex
	accounts map[string]*downloadGate
	resolve  func(context.Context, string, string, string) (string, error)
}

// downloadGate 在 downloadState.mu 下访问；一个活跃项覆盖取链与有限内容重试，但不持有 API 锁。
type downloadGate struct {
	active, waiting int
	cooldown        time.Time
	blocked         bool
	changed         chan struct{}
}

// notifyDownloadLocked 唤醒因配额或冷却变化而等待的调用者，调用者必须持有下载互斥锁。
func notifyDownloadLocked(g *downloadGate) { close(g.changed); g.changed = make(chan struct{}) }

// acquireDownload 将同账号处理中项限制为二、等待项限制为十六；所有取消路径都移除等待计数。
func (s *Service) acquireDownload(ctx context.Context, scope string, check func(context.Context) error) (*downloadGate, error) {
	s.downloads.mu.Lock()
	if s.downloads.accounts == nil {
		s.downloads.accounts = make(map[string]*downloadGate)
	}
	g := s.downloads.accounts[scope]
	if g == nil {
		g = &downloadGate{changed: make(chan struct{})}
		s.downloads.accounts[scope] = g
	}
	if g.waiting >= 16 {
		s.downloads.mu.Unlock()
		return nil, ErrDownloadQueueFull
	}
	g.waiting++
	s.downloads.mu.Unlock()
	defer func() { s.downloads.mu.Lock(); g.waiting--; notifyDownloadLocked(g); s.downloads.mu.Unlock() }()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := check(ctx); err != nil {
			return nil, err
		}
		s.downloads.mu.Lock()
		if g.active < 2 {
			g.active++
			s.downloads.mu.Unlock()
			return g, nil
		}
		changed := g.changed
		s.downloads.mu.Unlock()
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
		case <-changed:
		case <-timer.C:
		}
		timer.Stop()
	}
}

// releaseDownload 释放本调用的处理中资格，不影响独立账号 API 配额或其他下载的冷却。
func (s *Service) releaseDownload(g *downloadGate) {
	s.downloads.mu.Lock()
	defer s.downloads.mu.Unlock()
	g.active--
	notifyDownloadLocked(g)
}

// observeDownloadThrottle 按 Retry-After 延长同账号全部下载源的保守冷却；溢出值不可降为较短默认值。
func (s *Service) observeDownloadThrottle(g *downloadGate, value string) {
	until, blocked := downloadRetryAfter(value, time.Now())
	s.downloads.mu.Lock()
	defer s.downloads.mu.Unlock()
	if until.After(g.cooldown) {
		g.cooldown = until
	}
	g.blocked = g.blocked || blocked
	notifyDownloadLocked(g)
}

// downloadRetryAfter 接受非负整数秒或未来 HTTP 日期，未提供或无效值冷却三十秒。
func downloadRetryAfter(value string, now time.Time) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value != "" && strings.Trim(value, "0123456789") == "" {
		seconds, err := strconv.ParseUint(value, 10, 64)
		if err != nil || seconds > uint64(math.MaxInt64/int64(time.Second)) {
			return time.Time{}, true
		}
		return now.Add(time.Duration(seconds) * time.Second), false
	}
	if parsed, err := http.ParseTime(value); err == nil && parsed.After(now) {
		return parsed, false
	}
	return now.Add(30 * time.Second), false
}

// waitDownloadCooldown 在每次内容尝试前检查共享冷却，不截短上游要求，任务取消和凭据变化可中断。
func (s *Service) waitDownloadCooldown(ctx context.Context, g *downloadGate, check func(context.Context) error) error {
	for {
		if err := ctx.Err(); err != nil {
			return downloadContextError(ctx, err)
		}
		if err := check(ctx); err != nil {
			return err
		}
		s.downloads.mu.Lock()
		delay, blocked, changed := time.Until(g.cooldown), g.blocked, g.changed
		s.downloads.mu.Unlock()
		if !blocked && delay <= 0 {
			return nil
		}
		if blocked {
			delay = time.Hour
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
		case <-changed:
		case <-timer.C:
		}
		timer.Stop()
	}
}

// waitDownload 拥有可取消的局部退避计时器，不新增协程，也不延长文件总预算。
func waitDownload(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// monitorDownloadCredential 为已准入的至多两个下载监视完整凭据快照；调用者 stop 取消并等待协程退出。
func monitorDownloadCredential(parent context.Context, check func(context.Context) error) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTicker(250 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if err := check(ctx); err != nil {
					cancel(err)
					return
				}
			}
		}
	}()
	return ctx, func() { cancel(context.Canceled); <-done }
}

// expiredDownloadURL 仅根据签名 URL 显式的 Unix 过期秒数判定过期；不猜测 403 正文或账号状态。
func expiredDownloadURL(raw string, now time.Time) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	for _, name := range []string{"t"} {
		value := u.Query().Get(name)
		if value == "" || strings.Trim(value, "0123456789") != "" {
			continue
		}
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err == nil && seconds > 0 && seconds <= now.Unix() {
			return true
		}
	}
	return false
}
