// download_test.go：验证附件传输的字节保真、限量重试、可信目标、凭据回执、共享并发和下载/API 配额分离。
// 成功内容用包内取链边界替身，协议错误与后台预算另走真实 m115 请求和 SDK 调度。

package provider115

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	adapter "github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/provider115"
)

// downloadRepository 支持并发读取和替换测试 Cookie，避免凭据监视器测试自身的数据竞争。
type downloadRepository struct {
	testRepository
	mu     sync.Mutex
	cookie string
}

// ProviderCredential 返回锁保护的测试凭据，尊重已取消的上下文。
func (r *downloadRepository) ProviderCredential(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cookie, nil
}

// replace 模拟同 UID 重新扫码或切换账号，测试完整快照失效。
func (r *downloadRepository) replace(cookie string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cookie = cookie
}

// newDownloadFixture 构造真实服务、可并发凭据仓储和受控 HTTPS CDN 传输，所有资源由测试回收。
func newDownloadFixture(t *testing.T, transport http.RoundTripper) (*Service, *downloadRepository, *os.File, DownloadRequest) {
	t.Helper()
	repo := &downloadRepository{cookie: "UID=123_A1_private; CID=secret; SEID=private"}
	s := New(repo, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	s.client.Transport = transport
	s.downloads.resolve = func(context.Context, string, string, string) (string, error) {
		return "https://media.115cdn.net/original?signature=private", nil
	}
	file, err := os.Create(filepath.Join(t.TempDir(), "staging"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	return s, repo, file, DownloadRequest{AccountID: "123", SourceFileID: "456", PlaybackCode: "privateLocator", Kind: "nfo", RequestID: "download-request"}
}

// downloadResponse 构造独立关闭的内容响应，长度与正文可分别控制以覆盖畸形上游。
func downloadResponse(status int, body string, length int64) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), ContentLength: length, Body: io.NopCloser(strings.NewReader(body))}
}

// TestDownloadSidecarRawBytesAndSnapshot 验证 NFO 原始字节、固定无 Cookie 请求头、成功回执及同 UID 凭据变更后拒绝发布。
func TestDownloadSidecarRawBytesAndSnapshot(t *testing.T) {
	body := "\xff\xfe<movie>原始 NFO</movie>\r\n"
	s, repo, sink, request := newDownloadFixture(t, scanTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "media.115cdn.net" || r.Method != http.MethodGet || r.Header.Get("Cookie") != "" || r.UserAgent() != downloadUserAgent || r.Header.Get("Accept-Encoding") != "identity" || r.Header.Get("Referer") != "https://115.com/" {
			t.Error("unsafe content request")
		}
		return downloadResponse(200, body, int64(len(body))), nil
	}))
	expected := int64(len(body))
	request.ExpectedBytes = &expected
	receipt, err := s.DownloadSidecar(t.Context(), request, sink)
	if err != nil || receipt.Bytes != expected || receipt.Check(t.Context()) != nil {
		t.Fatalf("download receipt: bytes=%d err=%v", receipt.Bytes, err)
	}
	if _, err := sink.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(sink)
	if err != nil || !bytes.Equal(got, []byte(body)) {
		t.Fatal("download rewrote raw bytes")
	}
	repo.replace("UID=123_A1_private; CID=new; SEID=new")
	if !errors.Is(receipt.Check(t.Context()), ErrCredentialChanged) {
		t.Fatal("same UID credential replacement accepted old receipt")
	}
	if (DownloadReceipt{}).Check(t.Context()) == nil {
		t.Fatal("empty receipt accepted")
	}
}

// TestDownloadSidecarRetryAndFailureBounds 验证瞬时失败清空旧暂存后有限重试、长度/超限/403 分类和日志脱敏。
func TestDownloadSidecarRetryAndFailureBounds(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		length   int64
		expected *int64
		want     error
		calls    int
	}{
		{"retry 503", 503, "", 0, nil, ErrUpstream, 3},
		{"forbidden", 403, "private upstream body", 21, nil, ErrUpstream, 1},
		{"header length mismatch", 200, "abc", 3, new(int64(4)), ErrDownloadLength, 1},
		{"body length mismatch", 200, "abc", -1, new(int64(4)), ErrDownloadLength, 3},
		{"header oversized", 200, "", (8 << 20) + 1, nil, ErrDownloadTooLarge, 1},
		{"stream oversized", 200, strings.Repeat("x", (8<<20)+1), -1, nil, ErrDownloadTooLarge, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				s, _, sink, request := newDownloadFixture(t, scanTransport(func(*http.Request) (*http.Response, error) {
					calls++
					return downloadResponse(tc.status, tc.body, tc.length), nil
				}))
				var logs bytes.Buffer
				s.logger = slog.New(slog.NewJSONHandler(&logs, nil))
				request.ExpectedBytes = tc.expected
				_, err := s.DownloadSidecar(t.Context(), request, sink)
				if !errors.Is(err, tc.want) || calls != tc.calls {
					t.Fatalf("calls=%d err=%v", calls, err)
				}
				if errors.Is(err, ErrInvalidAuthorization) {
					t.Fatal("CDN response invalidated account")
				}
				for _, private := range []string{"signature=private", "privateLocator", "CID=secret", "private upstream body", "staging"} {
					if strings.Contains(logs.String(), private) || strings.Contains(err.Error(), private) {
						t.Fatal("sensitive content leaked")
					}
				}
			})
		})
	}
	t.Run("retry truncates old bytes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			calls := 0
			s, _, sink, request := newDownloadFixture(t, scanTransport(func(*http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: 8, Body: &brokenDownloadBody{}}, nil
				}
				return downloadResponse(200, "ok", 2), nil
			}))
			receipt, err := s.DownloadSidecar(t.Context(), request, sink)
			if err != nil || receipt.Bytes != 2 || calls != 2 {
				t.Fatalf("retry calls=%d bytes=%d err=%v", calls, receipt.Bytes, err)
			}
			if _, err := sink.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(sink)
			if err != nil || string(got) != "ok" {
				t.Fatal("failed attempt bytes survived")
			}
		})
	})
}

// TestDownloadSidecarStableErrorCodes 验证取链、目标和 CDN 阶段均输出稳定脱敏错误码。
func TestDownloadSidecarStableErrorCodes(t *testing.T) {
	cases := []struct {
		name, want   string
		resolve      func(context.Context, string, string, string) (string, error)
		status       int
		transportErr error
	}{
		{name: "source not found", want: "sidecar_source_not_found", resolve: func(context.Context, string, string, string) (string, error) { return "", ErrSourceNotFound }},
		{name: "take link failed", want: "sidecar_take_link_failed", resolve: func(context.Context, string, string, string) (string, error) {
			return "", errors.New("take link unavailable")
		}},
		{name: "take link response", want: "sidecar_take_link_response_invalid", resolve: func(context.Context, string, string, string) (string, error) {
			return "", errors.Join(ErrDownloadLinkResponse, ErrUpstream)
		}},
		{name: "untrusted target", want: "sidecar_untrusted_target", resolve: func(context.Context, string, string, string) (string, error) { return "https://example.test/file", nil }},
		{name: "content status", want: "sidecar_content_status_failed", status: http.StatusNotFound},
		{name: "content request", want: "sidecar_content_request_failed", transportErr: errors.New("network unavailable")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var logs bytes.Buffer
				s, _, sink, request := newDownloadFixture(t, scanTransport(func(*http.Request) (*http.Response, error) {
					if tc.transportErr != nil {
						return nil, tc.transportErr
					}
					return downloadResponse(tc.status, "", 0), nil
				}))
				s.logger = slog.New(slog.NewJSONHandler(&logs, nil))
				if tc.resolve != nil {
					s.downloads.resolve = tc.resolve
				}
				_, _ = s.DownloadSidecar(t.Context(), request, sink)
				if !strings.Contains(logs.String(), `"error_code":"`+tc.want+`"`) {
					t.Fatalf("missing error code %s in %s", tc.want, logs.String())
				}
			})
		})
	}
}

// brokenDownloadBody 模拟成功返回一部分内容后网络截断，首次落盘字节必须由下一次尝试清空。
type brokenDownloadBody struct{ sent bool }

// Read 先返回部分正文，再返回可分类为长度不完整的错误。
func (b *brokenDownloadBody) Read(p []byte) (int, error) {
	if b.sent {
		return 0, io.ErrUnexpectedEOF
	}
	b.sent = true
	return copy(p, "partial"), nil
}

// Close 模拟不额外失败的内容响应关闭。
func (*brokenDownloadBody) Close() error { return nil }

// TestDownloadSidecarExpectedSizeRejectsBeforeLink 验证已知超限源和非法参数在取链前拒绝。
func TestDownloadSidecarExpectedSizeRejectsBeforeLink(t *testing.T) {
	s, _, sink, request := newDownloadFixture(t, scanTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("unexpected content request")
		return nil, ErrUpstream
	}))
	s.downloads.resolve = func(context.Context, string, string, string) (string, error) {
		t.Fatal("unexpected take-link")
		return "", ErrUpstream
	}
	for _, kind := range []string{"nfo", "image"} {
		request.Kind = kind
		limit := int64(8 << 20)
		if kind == "image" {
			limit = 64 << 20
		}
		size := limit + 1
		request.ExpectedBytes = &size
		if _, err := s.DownloadSidecar(t.Context(), request, sink); !errors.Is(err, ErrDownloadTooLarge) {
			t.Fatalf("kind %s err=%v", kind, err)
		}
	}
	request.ExpectedBytes = nil
	request.Kind = "subtitle"
	if _, err := s.DownloadSidecar(t.Context(), request, sink); !errors.Is(err, ErrInvalidQuery) {
		t.Fatal(err)
	}
}

// TestDownloadSidecarTrustedRedirects 验证每跳 HTTPS 可信域、受控头和五跳上限，安全拒绝不重试。
func TestDownloadSidecarTrustedRedirects(t *testing.T) {
	for _, target := range []string{"https://other.115cdn.net/final", "http://media.115cdn.net/final", "https://115cdn.net.evil.test/final", "https://user@115cdn.net/final", "https://115cdn.net:443/final", "https://127.0.0.1/final", "loop"} {
		t.Run(target, func(t *testing.T) {
			calls := 0
			s, _, sink, request := newDownloadFixture(t, scanTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Header.Get("Cookie") != "" {
					t.Error("redirect leaked cookie")
				}
				if r.URL.Path == "/final" {
					return downloadResponse(200, "ok", 2), nil
				}
				response := downloadResponse(302, "", 0)
				location := target
				if target == "loop" {
					location = "https://media.115cdn.net/loop"
				}
				response.Header.Set("Location", location)
				return response, nil
			}))
			_, err := s.DownloadSidecar(t.Context(), request, sink)
			if target == "https://other.115cdn.net/final" {
				if err != nil || calls != 2 {
					t.Fatalf("trusted redirect calls=%d err=%v", calls, err)
				}
				return
			}
			if err == nil || target != "loop" && calls != 1 || target == "loop" && calls != 6 {
				t.Fatalf("redirect escaped bound calls=%d err=%v", calls, err)
			}
		})
	}
}

// TestDownloadSidecarExpiredRefreshBound 验证仅有明确签名过期秒数的 403 刷新一次，且仍计入三次内容预算。
func TestDownloadSidecarExpiredRefreshBound(t *testing.T) {
	for _, proven := range []bool{false, true} {
		t.Run(fmt.Sprint(proven), func(t *testing.T) {
			calls, links := 0, 0
			s, _, sink, request := newDownloadFixture(t, scanTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Query().Get("t") == "1" {
					return downloadResponse(403, "", 0), nil
				}
				return downloadResponse(200, "ok", 2), nil
			}))
			s.downloads.resolve = func(context.Context, string, string, string) (string, error) {
				links++
				if !proven || links == 1 {
					return "https://media.115cdn.net/file?t=1", nil
				}
				return "https://media.115cdn.net/file?t=9999999999", nil
			}
			if !proven {
				s.downloads.resolve = func(context.Context, string, string, string) (string, error) {
					links++
					return "https://media.115cdn.net/file?signature=private", nil
				}
				s.client.Transport = scanTransport(func(*http.Request) (*http.Response, error) { calls++; return downloadResponse(403, "", 0), nil })
			}
			_, err := s.DownloadSidecar(t.Context(), request, sink)
			if proven {
				if err != nil || links != 2 || calls != 2 {
					t.Fatalf("refresh links=%d calls=%d err=%v", links, calls, err)
				}
			} else if err == nil || links != 1 || calls != 1 {
				t.Fatalf("unproven refresh links=%d calls=%d err=%v", links, calls, err)
			}
		})
	}
}

// TestDownloadSidecarConcurrencyQueueAndCancellation 验证跨调用共享双并发、十六等待上限、取消移除及 API 锁不被内容占用。
func TestDownloadSidecarConcurrencyQueueAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var active, maxActive atomic.Int32
		started := make(chan struct{}, 32)
		s, _, _, request := newDownloadFixture(t, scanTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "webapi.115.com" {
				return downloadResponse(200, `{"state":true}`, 14), nil
			}
			n := active.Add(1)
			defer active.Add(-1)
			for old := maxActive.Load(); n > old && !maxActive.CompareAndSwap(old, n); old = maxActive.Load() {
			}
			started <- struct{}{}
			<-r.Context().Done()
			return nil, r.Context().Err()
		}))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		results := make(chan error, 18)
		for range 18 {
			go func() {
				file, err := os.CreateTemp(t.TempDir(), "sink")
				if err != nil {
					results <- err
					return
				}
				defer func() {
					if err := file.Close(); err != nil {
						t.Error(err)
					}
				}()
				_, err = s.DownloadSidecar(ctx, request, file)
				results <- err
			}()
		}
		synctest.Wait()
		if len(started) != 2 || maxActive.Load() != 2 {
			t.Fatalf("active=%d max=%d", len(started), maxActive.Load())
		}
		file, err := os.CreateTemp(t.TempDir(), "rejected")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := file.Close(); err != nil {
				t.Error(err)
			}
		}()
		if _, err = s.DownloadSidecar(t.Context(), request, file); !errors.Is(err, ErrDownloadQueueFull) {
			t.Fatalf("unbounded queue err=%v", err)
		}
		// 内容仍阻塞时直接读取真实 SDK，证明下载没有占住账号 API gate。
		if _, err := s.doRequest(t.Context(), adapter.Request{Scope: "account:123", Operation: adapter.Metadata, URL: "https://webapi.115.com/category/get"}); err != nil {
			t.Fatalf("content held API gate: %v", err)
		}
		cancel()
		for range 18 {
			if err := <-results; !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel err=%v", err)
			}
		}
		s.downloads.mu.Lock()
		g := s.downloads.accounts["account:123"]
		a, w := g.active, g.waiting
		s.downloads.mu.Unlock()
		if a != 0 || w != 0 {
			t.Fatalf("quota leaked active=%d wait=%d", a, w)
		}
	})
}

// TestDownloadSidecarCredentialChangeCancelsBody 验证内容读取阻塞期间同 UID 换凭据会主动取消传输并释放配额。
func TestDownloadSidecarCredentialChangeCancelsBody(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		s, repo, sink, request := newDownloadFixture(t, scanTransport(func(r *http.Request) (*http.Response, error) {
			close(started)
			<-r.Context().Done()
			return nil, r.Context().Err()
		}))
		result := make(chan error, 1)
		go func() { _, err := s.DownloadSidecar(t.Context(), request, sink); result <- err }()
		<-started
		repo.replace("UID=123_A1_private; CID=replaced")
		if err := <-result; !errors.Is(err, ErrCredentialChanged) {
			t.Fatalf("old stream accepted: %v", err)
		}
	})
}

// TestDownloadRetryAfterSharedAndDeadline 验证秒值/日期/默认/溢出冷却，后续同账号调用不能缩短已有冷却。
func TestDownloadRetryAfterSharedAndDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		for _, tc := range []struct {
			value   string
			delay   time.Duration
			blocked bool
		}{{"", 30 * time.Second, false}, {"2", 2 * time.Second, false}, {now.Add(time.Minute).UTC().Format(http.TimeFormat), time.Minute, false}, {"invalid", 30 * time.Second, false}, {"999999999999999999999999", 0, true}} {
			until, blocked := downloadRetryAfter(tc.value, now)
			if blocked != tc.blocked || !blocked && until.Sub(now) != tc.delay {
				t.Fatalf("retry-after %q delay=%v blocked=%t", tc.value, until.Sub(now), blocked)
			}
		}
		calls := 0
		s, _, sink, request := newDownloadFixture(t, scanTransport(func(*http.Request) (*http.Response, error) {
			calls++
			response := downloadResponse(429, "", 0)
			response.Header.Set("Retry-After", "60")
			return response, nil
		}))
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		if _, err := s.DownloadSidecar(ctx, request, sink); !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
			t.Fatalf("shortened cooldown calls=%d err=%v", calls, err)
		}
		s.downloads.mu.Lock()
		g := s.downloads.accounts["account:123"]
		until := g.cooldown
		s.downloads.mu.Unlock()
		s.observeDownloadThrottle(g, "1")
		s.downloads.mu.Lock()
		same := g.cooldown.Equal(until)
		s.downloads.mu.Unlock()
		if !same {
			t.Fatal("shorter cooldown replaced shared value")
		}
		ctx2, cancel2 := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel2()
		if _, err := s.DownloadSidecar(ctx2, request, sink); !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
			t.Fatalf("later caller bypassed shared cooldown calls=%d err=%v", calls, err)
		}
	})
}

// TestDownloadLinkRealAdapterBackgroundBudget 验证取链 POST 的真实协议头、只读限流及 60 秒间隔后台预算，不借用播放 15 秒期限。
func TestDownloadLinkRealAdapterBackgroundBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, _, _, _ := newDownloadFixture(t, nil)
		s.requestInterval = time.Minute
		calls := 0
		s.client.Transport = scanTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Host != "proapi.115.com" || r.URL.Path != "/app/chrome/downurl" || r.Method != http.MethodPost || r.Header.Get("Cookie") == "" || r.UserAgent() != downloadUserAgent {
				t.Error("wrong take-link request")
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			values, err := url.ParseQuery(string(body))
			if err != nil || values.Get("data") == "" || strings.Contains(string(body), "privateLocator") {
				t.Error("unencoded m115 request")
			}
			return downloadResponse(200, `{"state":false,"errno":50003}`, 31), nil
		})
		for range 2 {
			if _, err := s.requestDownloadLink(t.Context(), "UID=123_A1_private; CID=secret; SEID=private", "456", "privateLocator", downloadUserAgent, adapter.DownloadLink, adapter.Background); !errors.Is(err, ErrSourceNotFound) {
				t.Fatal(err)
			}
		}
		if calls != 2 {
			t.Fatal("long interval prevented following download link")
		}
		start := time.Now()
		if _, err := s.requestPlayback(t.Context(), "UID=123_A1_private; CID=secret; SEID=private", "456", "privateLocator", downloadUserAgent); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != 15*time.Second || calls != 2 {
			t.Fatalf("playback budget changed elapsed=%s calls=%d err=%v", time.Since(start), calls, err)
		}
	})
}
