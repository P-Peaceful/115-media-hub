// client_test.go：以虚拟时间和受控传输验证账号间隔、公平调度、限流重试、发送预算与敏感数据边界。

package provider115

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip 让测试在实际发送边界观察请求，不接触真实网盘。
func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// testClient 使用生产端点白名单和受控传输，保留真实调度策略。
func testClient(t *testing.T, interval time.Duration, transport roundTripFunc) *Client {
	t.Helper()
	c, err := New(Config{Interval: interval, Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

// response 构造带可关闭响应体的上游测试响应。
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

// fileRequest 返回无真实凭据的目录请求。
func fileRequest() Request {
	return Request{Operation: Files, Scope: "account-1", URL: "https://webapi.115.com/files"}
}

// TestCompletionInterval 验证读取并关闭响应后才开始间隔，调大配置不会被压回一秒。
func TestCompletionInterval(t *testing.T) {
	for _, interval := range []time.Duration{time.Second, 7 * time.Second} {
		t.Run(interval.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var starts []time.Time
				c := testClient(t, interval, func(*http.Request) (*http.Response, error) {
					starts = append(starts, time.Now())
					time.Sleep(2 * time.Second)
					return response(200, "ok"), nil
				})
				for range 2 {
					if _, err := c.Do(t.Context(), fileRequest()); err != nil {
						t.Fatal(err)
					}
				}
				if got := starts[1].Sub(starts[0]); got != 2*time.Second+interval {
					t.Fatalf("start gap %s", got)
				}
			})
		})
	}
}

// TestBackgroundLongIntervalBudget 验证合法 60 秒间隔下后台后续页仍可准入，更早调用方期限不能被延长。
func TestBackgroundLongIntervalBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var starts []time.Time
		c := testClient(t, time.Minute, func(*http.Request) (*http.Response, error) {
			starts = append(starts, time.Now())
			return response(http.StatusOK, "ok"), nil
		})
		r := fileRequest()
		r.Priority = Background
		for range 2 {
			if _, err := c.Do(t.Context(), r); err != nil {
				t.Fatal(err)
			}
		}
		if len(starts) != 2 || starts[1].Sub(starts[0]) != time.Minute {
			t.Fatalf("background pages cannot follow long interval: %v", starts)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		start := time.Now()
		if _, err := c.Do(ctx, r); !errors.Is(err, context.DeadlineExceeded) || len(starts) != 2 || time.Since(start) != 15*time.Second {
			t.Fatalf("background budget bypassed caller deadline: calls=%d elapsed=%s err=%v", len(starts), time.Since(start), err)
		}
	})
}

// TestFairForegroundScheduling 验证前台插入分页间隙，且四次前台后必须放行已等待的后台。
func TestFairForegroundScheduling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hold := make(chan struct{})
		var order []string
		c := testClient(t, time.Second, func(r *http.Request) (*http.Response, error) {
			id := r.Header.Get("X-Test-ID")
			order = append(order, id)
			if id == "hold" {
				<-hold
			}
			return response(200, "ok"), nil
		})
		results := make(chan error, 8)
		launch := func(id string, priority Priority) {
			r := fileRequest()
			r.Priority = priority
			r.Header = http.Header{"X-Test-Id": {id}}
			go func() { _, err := c.Do(t.Context(), r); results <- err }()
			synctest.Wait()
		}
		launch("hold", Background)
		launch("background", Background)
		for _, id := range []string{"front1", "front2", "front3", "front4", "front5", "front6"} {
			launch(id, Foreground)
		}
		close(hold)
		for range 8 {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		expected := "hold,front1,front2,front3,front4,background,front5,front6"
		if strings.Join(order, ",") != expected {
			t.Fatalf("order %v", order)
		}
	})
}

// TestCancellationAndClose 验证取消清除排队项、不消耗预算，关闭可唤醒所有等待者。
func TestCancellationAndClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hold := make(chan struct{})
		calls := 0
		c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				<-hold
			}
			return response(200, "ok"), nil
		})
		initial := make(chan error, 1)
		go func() { _, err := c.Do(t.Context(), fileRequest()); initial <- err }()
		synctest.Wait()
		ctx, cancel := context.WithCancel(t.Context())
		budget := NewBudget(5)
		r := fileRequest()
		r.Budget = budget
		result := make(chan error, 1)
		go func() { _, err := c.Do(ctx, r); result <- err }()
		synctest.Wait()
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel %v", err)
		}
		if budget.Remaining() != 5 {
			t.Fatal("queue wait consumed budget")
		}
		c.mu.Lock()
		queued := len(c.accounts["account-1"].queue)
		c.mu.Unlock()
		if queued != 0 {
			t.Fatal("canceled waiter retained")
		}
		go func() { _, err := c.Do(t.Context(), fileRequest()); result <- err }()
		synctest.Wait()
		c.Close()
		if err := <-result; !errors.Is(err, ErrClosed) {
			t.Fatalf("close %v", err)
		}
		close(hold)
		if err := <-initial; err != nil {
			t.Fatal(err)
		}
	})
}

// TestSharedRetryAfter 验证秒值、HTTP 日期和默认冷却覆盖其他操作，且不跨账号阻塞。
func TestSharedRetryAfter(t *testing.T) {
	for _, header := range []string{"10", "date", "invalid"} {
		t.Run(header, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				delay := 10 * time.Second
				if header == "invalid" {
					delay = 30 * time.Second
				}
				calls := 0
				var starts []time.Time
				c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) {
					calls++
					starts = append(starts, time.Now())
					if calls == 1 {
						r := response(429, "private body")
						v := header
						if v == "date" {
							v = start.Add(delay).UTC().Format(http.TimeFormat)
						}
						r.Header.Set("Retry-After", v)
						return r, nil
					}
					return response(200, "ok"), nil
				})
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				_, err := c.Do(ctx, fileRequest())
				var upstream *Error
				if !errors.As(err, &upstream) || upstream.Kind != Throttled || !upstream.RetryAt.Equal(start.Add(delay)) {
					t.Fatalf("throttle %#v %v", upstream, err)
				}
				other := fileRequest()
				other.Scope = "account-2"
				if _, err := c.Do(t.Context(), other); err != nil {
					t.Fatal(err)
				}
				if !starts[1].Equal(start) {
					t.Fatal("other account delayed")
				}
				r := fileRequest()
				r.Operation = Metadata
				r.URL = "https://webapi.115.com/files/index_info"
				if _, err := c.Do(t.Context(), r); err != nil {
					t.Fatal(err)
				}
				if starts[2].Sub(start) != delay {
					t.Fatalf("shared cooldown %s", starts[2].Sub(start))
				}
			})
		})
	}
}

// TestCooldownNeverShortened 验证新短冷却不覆盖旧长冷却，溢出秒值不会立刻重发。
func TestCooldownNeverShortened(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) { calls++; return response(200, "ok"), nil })
		start := time.Now()
		c.ObserveThrottle("account-1", "20")
		c.ObserveThrottle("account-1", "1")
		if _, err := c.Do(t.Context(), fileRequest()); err != nil {
			t.Fatal(err)
		}
		if time.Since(start) != 20*time.Second {
			t.Fatalf("shortened cooldown %s", time.Since(start))
		}
		c.ObserveThrottle("account-1", "99999999999999999999999999999999999999")
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		_, err := c.Do(ctx, fileRequest())
		if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
			t.Fatalf("overflow cooldown: calls=%d err=%v", calls, err)
		}
	})
}

// TestReadRetryLimits 验证暂时故障最多三次、退避一秒两秒，非只读登录只发一次。
func TestReadRetryLimits(t *testing.T) {
	for _, status := range []int{0, 502, 503, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var starts []time.Time
				c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) {
					starts = append(starts, time.Now())
					if status == 0 {
						return nil, errors.New("secret https://example.test/?cookie=private")
					}
					return response(status, "private"), nil
				})
				_, err := c.Do(t.Context(), fileRequest())
				var e *Error
				if !errors.As(err, &e) || e.Kind != Temporary || len(starts) != 3 {
					t.Fatalf("retry calls %d: %v", len(starts), err)
				}
				if starts[1].Sub(starts[0]) != time.Second || starts[2].Sub(starts[1]) != 2*time.Second {
					t.Fatalf("backoff %v", starts)
				}
				if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "https") {
					t.Fatal("network data leaked")
				}
			})
		})
	}
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) { calls++; return response(503, "bad"), nil })
		r := Request{Operation: Login, Method: http.MethodPost, URL: "https://qrcodeapi.115.com/app/1.0/alipaymini/1.0/login/qrcode/"}
		if _, err := c.Do(t.Context(), r); err == nil || calls != 1 {
			t.Fatalf("login repeated %d %v", calls, err)
		}
	})
}

// TestRequestBudgetCountsAttempts 验证目录重试和跨调用共享五次实际发送，排队和校验失败不扣次数。
func TestRequestBudgetCountsAttempts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) { calls++; return response(503, "bad"), nil })
		budget := NewBudget(5)
		ctx := WithBudget(t.Context(), budget)
		if _, err := c.Do(ctx, fileRequest()); err == nil {
			t.Fatal("expected temporary failure")
		}
		if _, err := c.Do(ctx, fileRequest()); !errors.Is(err, ErrBudgetExhausted) {
			t.Fatalf("budget %v", err)
		}
		if calls != 5 || budget.Remaining() != 0 {
			t.Fatalf("sent %d remaining %d", calls, budget.Remaining())
		}
		if _, err := c.Do(ctx, fileRequest()); !errors.Is(err, ErrBudgetExhausted) || calls != 5 {
			t.Fatal("sent over budget")
		}
		budget = NewBudget(1)
		r := fileRequest()
		r.Budget = budget
		sentinel := errors.New("changed credentials")
		r.Check = func(context.Context) error { return sentinel }
		if _, err := c.Do(t.Context(), r); !errors.Is(err, sentinel) || budget.Remaining() != 1 {
			t.Fatalf("check error %v budget %d", err, budget.Remaining())
		}
	})
}

// TestCredentialRecheckDiscardResponse 验证采用响应前重新校验，旧凭据结果不被返回。
func TestCredentialRecheckDiscardResponse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) { return response(200, "old account result"), nil })
		r := fileRequest()
		sentinel := errors.New("credential changed")
		r.Check = func(context.Context) error {
			calls++
			if calls == 2 {
				return sentinel
			}
			return nil
		}
		data, err := c.Do(t.Context(), r)
		if !errors.Is(err, sentinel) || data != nil || calls != 2 {
			t.Fatalf("adopted stale response %q %v", data, err)
		}
	})
}

// TestOperationDeadlines 验证单次十五秒及总预算，播放不因重试延长短路径。
func TestOperationDeadlines(t *testing.T) {
	for _, op := range []Operation{Files, Playback} {
		t.Run(string(op), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				c := testClient(t, time.Second, func(r *http.Request) (*http.Response, error) {
					calls++
					<-r.Context().Done()
					return nil, r.Context().Err()
				})
				r := fileRequest()
				r.Operation = op
				if op == Playback {
					r.URL = "https://proapi.115.com/app/chrome/downurl"
					r.Method = http.MethodPost
				}
				start := time.Now()
				_, err := c.Do(t.Context(), r)
				want := 45 * time.Second
				if op == Playback {
					want = 15 * time.Second
				}
				if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != want {
					t.Fatalf("deadline %s %v", time.Since(start), err)
				}
				if op == Files && calls != 3 || op == Playback && calls != 1 {
					t.Fatalf("attempts %d", calls)
				}
			})
		})
	}
}

// TestDownloadEndpointMatrix 验证取链白名单按 HTTPS 主机、精确路径和 HTTP 方法约束发送。
func TestDownloadEndpointMatrix(t *testing.T) {
	for _, tc := range []struct {
		address, method string
		allowed         bool
	}{
		{"https://proapi.115.com/android/2.0/ufile/download", http.MethodPost, true},
		{"https://pro.api.115.com/android/2.0/ufile/download", http.MethodPost, true},
		{"https://proapi.115.com/app/chrome/downurl?t=1", http.MethodPost, true},
		{"https://webapi.115.com/files/download", http.MethodPost, true},
		{"https://webapi.115.com/files/download?pickcode=example", http.MethodGet, true},
		{"http://pro.api.115.com/android/2.0/ufile/download", http.MethodPost, false},
		{"https://proapi.115.com/android/2.0/ufile/download", http.MethodGet, false},
		{"https://pro.api.115.com/app/chrome/downurl", http.MethodPost, false},
		{"https://proapi.115.com/files/download", http.MethodGet, false},
		{"https://webapi.115.com/files/%64ownload", http.MethodGet, false},
		{"https://webapi.115.com.evil.test/files/download", http.MethodGet, false},
	} {
		t.Run(tc.method+" "+tc.address, func(t *testing.T) {
			calls := 0
			c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) {
				calls++
				return response(200, "ok"), nil
			})
			for _, op := range []Operation{DownloadLink, Playback} {
				_, err := c.Do(t.Context(), Request{Operation: op, Scope: string(op), Method: tc.method, URL: tc.address})
				if (err == nil) != tc.allowed {
					t.Fatalf("wrong endpoint classification for %s: %v", op, err)
				}
			}
			if tc.allowed && calls != 2 || !tc.allowed && calls != 0 {
				t.Fatalf("unexpected send count: %d", calls)
			}
		})
	}
}

// TestEndpointAndResponseSafety 验证信任端点、大小边界、截断响应、状态分类及重定向不跟随。
func TestEndpointAndResponseSafety(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		readError bool
		kind      Kind
	}{
		{"exact", 200, "abcd", false, ""}, {"oversize", 200, "abcde", false, Protocol}, {"unauthorized", 401, "", false, InvalidAuthorization}, {"forbidden", 403, "", false, InvalidAuthorization}, {"missing", 404, "", false, NotFound}, {"redirect", 302, "", false, Protocol},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) {
				calls++
				r := response(tt.status, tt.body)
				r.Header.Set("Location", "https://untrusted.test/?secret=private")
				if tt.readError {
					r.Body = &brokenReader{}
				}
				return r, nil
			})
			r := fileRequest()
			r.MaxBytes = 4
			data, err := c.Do(t.Context(), r)
			if tt.kind == "" {
				if err != nil || string(data) != tt.body {
					t.Fatalf("response %q %v", data, err)
				}
			} else {
				var e *Error
				if !errors.As(err, &e) || e.Kind != tt.kind || strings.Contains(err.Error(), "private") {
					t.Fatalf("safe error %v", err)
				}
			}
			if calls != 1 {
				t.Fatalf("unexpected repeat %d", calls)
			}
		})
	}
	for _, address := range []string{"http://webapi.115.com/files", "https://webapi.115.com.evil.test/files", "https://user:private@webapi.115.com/files", "https://webapi.115.com/files#private", "https://webapi.115.com/delete"} {
		t.Run(address, func(t *testing.T) {
			c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) { t.Fatal("untrusted endpoint sent"); return nil, nil })
			r := fileRequest()
			r.URL = address
			if _, err := c.Do(t.Context(), r); err == nil {
				t.Fatal("accepted endpoint")
			}
		})
	}
}

type brokenReader struct{}

// Read 模拟已建立响应中的网络截断，仍属于只读操作可有限重试的传输故障。
func (*brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// Close 结束截断响应的资源所有权。
func (*brokenReader) Close() error { return nil }

// failingCloser 模拟已完整读取响应但关闭传输资源失败，不把敏感底层错误直接返回。
type failingCloser struct{ io.Reader }

// Close 返回测试用传输故障，供有限重试与脱敏断言使用。
func (*failingCloser) Close() error { return errors.New("private-network-detail") }

// TestResponseIOFailuresRetryBoundedly 验证截断和关闭故障按临时传输错误最多尝试三次，明确认证及缺失状态不重试。
func TestResponseIOFailuresRetryBoundedly(t *testing.T) {
	for _, scenario := range []string{"truncated", "close_failure", "unauthorized", "forbidden", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) {
					calls++
					status := http.StatusOK
					switch scenario {
					case "unauthorized":
						status = http.StatusUnauthorized
					case "forbidden":
						status = http.StatusForbidden
					case "missing":
						status = http.StatusNotFound
					}
					r := response(status, "body")
					r.Body = &brokenReader{}
					if scenario == "close_failure" {
						r.Body = &failingCloser{Reader: strings.NewReader("body")}
					}
					return r, nil
				})
				r := fileRequest()
				r.Operation = FastFiles
				_, err := c.Do(t.Context(), r)
				var failure *Error
				if !errors.As(err, &failure) || strings.Contains(err.Error(), "private-network-detail") {
					t.Fatalf("unsafe error: %v", err)
				}
				wantCalls, wantKind := 3, Temporary
				switch scenario {
				case "unauthorized", "forbidden":
					wantCalls, wantKind = 1, InvalidAuthorization
				case "missing":
					wantCalls, wantKind = 1, NotFound
				}
				if calls != wantCalls || failure.Kind != wantKind {
					t.Fatalf("calls=%d kind=%s want=%d/%s", calls, failure.Kind, wantCalls, wantKind)
				}
			})
		})
	}
}

// TestConcurrentBudget 验证多个发送者不能突破共享的实际请求配额。
func TestConcurrentBudget(t *testing.T) {
	budget := NewBudget(5)
	var wg sync.WaitGroup
	var mu sync.Mutex
	sent := 0
	for range 20 {
		wg.Go(func() {
			if budget.Take() {
				mu.Lock()
				sent++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if sent != 5 || budget.Remaining() != 0 {
		t.Fatalf("sent %d", sent)
	}
}

// TestQRSeparateQuotaAndReplayBody 验证扫码不等待账号队列，播放的 POST 重试重建相同请求体且保留空 UA。
func TestQRSeparateQuotaAndReplayBody(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		blocked := make(chan struct{})
		bodies := []string{}
		playbackCalls := 0
		c := testClient(t, time.Second, func(r *http.Request) (*http.Response, error) {
			switch r.URL.Host {
			case "webapi.115.com":
				<-blocked
			case "proapi.115.com":
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				bodies = append(bodies, string(body))
				if r.Header.Get("User-Agent") != "" {
					t.Fatal("empty player UA replaced")
				}
				playbackCalls++
				if playbackCalls == 1 {
					return response(503, "bad"), nil
				}
			}
			return response(200, "ok"), nil
		})
		done := make(chan error, 1)
		go func() { _, err := c.Do(t.Context(), fileRequest()); done <- err }()
		synctest.Wait()
		start := time.Now()
		qr := Request{Operation: QR, URL: "https://qrcodeapi.115.com/api/1.0/mac/1.0/qrcode?uid=private"}
		if _, err := c.Do(t.Context(), qr); err != nil {
			t.Fatal(err)
		}
		if !time.Now().Equal(start) {
			t.Fatal("QR waited for account")
		}
		close(blocked)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		r := Request{Operation: Playback, Scope: "account-1", Method: http.MethodPost, URL: "https://proapi.115.com/app/chrome/downurl", Body: []byte("opaque-codec-body"), Header: http.Header{"User-Agent": {""}}}
		if _, err := c.Do(t.Context(), r); err != nil {
			t.Fatal(err)
		}
		if len(bodies) != 2 || bodies[0] != "opaque-codec-body" || bodies[1] != bodies[0] {
			t.Fatalf("replayed bodies %v", bodies)
		}
	})
}

// TestCloseDuringRetry 验证关闭能终止退避等待，不留到下一次尝试才退出。
func TestCloseDuringRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) { calls++; return response(503, "bad"), nil })
		done := make(chan error, 1)
		go func() { _, err := c.Do(t.Context(), fileRequest()); done <- err }()
		synctest.Wait()
		c.Close()
		if err := <-done; !errors.Is(err, ErrClosed) || calls != 1 {
			t.Fatalf("close during retry %d %v", calls, err)
		}
	})
}

// TestConfigurationAndExplicitTestEndpoint 验证非法配置不能启动，可信测试注入端点不需要修改生产白名单。
func TestConfigurationAndExplicitTestEndpoint(t *testing.T) {
	for _, interval := range []time.Duration{-1, time.Millisecond, 61 * time.Second} {
		if _, err := New(Config{Interval: interval}); err == nil {
			t.Fatalf("invalid interval accepted %s", interval)
		}
	}
	if _, err := New(Config{TestEndpoints: []string{"http://user:private@127.0.0.1/"}}); err == nil {
		t.Fatal("userinfo endpoint accepted")
	}
	c, err := New(Config{TestEndpoints: []string{"http://127.0.0.1:1234/path"}, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return response(200, "ok"), nil })})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r := fileRequest()
	r.URL = "http://127.0.0.1:1234/fixture"
	if _, err := c.Do(t.Context(), r); err != nil {
		t.Fatal(err)
	}
}

// TestThrottleSurvivesCredentialChange 验证旧凭据响应被丢弃时，同账号已确认的限流仍被保留。
func TestThrottleSurvivesCredentialChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		calls := 0
		checks := 0
		c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				r := response(429, "private")
				r.Header.Set("Retry-After", "20")
				return r, nil
			}
			return response(200, "ok"), nil
		})
		r := fileRequest()
		changed := errors.New("credential changed")
		r.Check = func(context.Context) error {
			checks++
			if checks == 2 {
				return changed
			}
			return nil
		}
		if _, err := c.Do(t.Context(), r); !errors.Is(err, changed) {
			t.Fatalf("response adoption %v", err)
		}
		if _, err := c.Do(t.Context(), fileRequest()); err != nil {
			t.Fatal(err)
		}
		if time.Since(start) != 20*time.Second || calls != 2 {
			t.Fatalf("lost cooldown %s %d", time.Since(start), calls)
		}
	})
}

// TestRetryDiagnosticsRedacted 验证适配器仅记录受控重试属性，Cookie、pickcode、URL 和响应原文均不进入日志。
func TestRetryDiagnosticsRedacted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})).With("module", "provider_115")
		c, err := New(Config{Logger: logger, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return response(503, "private-response-body"), nil })})
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		r := fileRequest()
		r.URL += "?pickcode=private-pickcode"
		r.Header = http.Header{"Cookie": {"private-cookie"}}
		if _, err := c.Do(t.Context(), r); err == nil {
			t.Fatal("expected upstream failure")
		}
		output := logs.String()
		for _, secret := range []string{"private-pickcode", "private-cookie", "private-response-body", "https://"} {
			if strings.Contains(output, secret) {
				t.Fatalf("sensitive log %s", output)
			}
		}
		if strings.Count(output, "115 read retry") != 2 || !strings.Contains(output, "attempt_count") || !strings.Contains(output, "error_code") {
			t.Fatalf("missing bounded diagnostic %s", output)
		}
		for record := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
			if strings.Count(record, `"module"`) != 1 || !strings.Contains(record, `"component":"adapter"`) {
				t.Fatalf("duplicate module or missing adapter component: %s", record)
			}
		}
	})
}

// TestThrottleRetryWithinBudget 验证冷却可容纳在总期限时才重试，并保持实际发送计数。
func TestThrottleRetryWithinBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		calls := 0
		c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return response(429, "private"), nil
			}
			return response(200, "ok"), nil
		})
		budget := NewBudget(5)
		r := fileRequest()
		r.Budget = budget
		if _, err := c.Do(t.Context(), r); err != nil {
			t.Fatal(err)
		}
		if calls != 2 || time.Since(start) != 30*time.Second || budget.Remaining() != 3 {
			t.Fatalf("throttle retry calls=%d duration=%s remaining=%d", calls, time.Since(start), budget.Remaining())
		}
	})
}

// TestResponseInspectionHoldsAdmission 验证业务响应检查仍持有账号资格，延迟登记的冷却不能被排队请求绕过。
func TestResponseInspectionHoldsAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		entered, release := make(chan struct{}), make(chan struct{})
		var starts []time.Time
		c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) {
			starts = append(starts, time.Now())
			return response(http.StatusOK, "opaque response"), nil
		})
		first := fileRequest()
		first.Inspect = func(context.Context, []byte) error {
			close(entered)
			<-release
			until := c.ObserveThrottle(first.Scope, "")
			return &Error{Kind: Throttled, RetryAt: until, StatusCode: http.StatusOK}
		}
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		firstDone, secondDone := make(chan error, 1), make(chan error, 1)
		go func() { _, err := c.Do(ctx, first); firstDone <- err }()
		<-entered
		go func() { _, err := c.Do(t.Context(), fileRequest()); secondDone <- err }()
		synctest.Wait()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if len(starts) != 1 {
			t.Fatal("queued request bypassed response inspection")
		}
		close(release)
		var throttled *Error
		if err := <-firstDone; !errors.As(err, &throttled) || throttled.Kind != Throttled || throttled.Operation != Files {
			t.Fatalf("inspection error classification: %v", err)
		}
		if err := <-secondDone; err != nil {
			t.Fatal(err)
		}
		if len(starts) != 2 || starts[1].Sub(start) != 32*time.Second {
			t.Fatalf("business cooldown bypassed: %v", starts)
		}
	})
}

// TestResponseInspectionPreservesErrorIdentity 验证业务检查的重试与实际预算共用，旧凭据丢弃响应仍保留业务冷却。
func TestResponseInspectionPreservesErrorIdentity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		calls, inspections, checks := 0, 0, 0
		c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) {
			calls++
			return response(http.StatusOK, "opaque response"), nil
		})
		changed := errors.New("credential replaced")
		r := fileRequest()
		r.Check = func(context.Context) error {
			checks++
			if checks == 2 {
				return changed
			}
			return nil
		}
		r.Inspect = func(context.Context, []byte) error {
			inspections++
			until := c.ObserveThrottle(r.Scope, "20")
			return &Error{Kind: Throttled, RetryAt: until}
		}
		if _, err := c.Do(t.Context(), r); !errors.Is(err, changed) {
			t.Fatalf("credential check identity lost: %v", err)
		}
		if _, err := c.Do(t.Context(), fileRequest()); err != nil {
			t.Fatal(err)
		}
		if inspections != 1 || calls != 2 || time.Since(start) != 20*time.Second {
			t.Fatalf("lost stale-response cooldown: inspections=%d calls=%d elapsed=%s", inspections, calls, time.Since(start))
		}
	})
	t.Run("retry_and_budget", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			calls := 0
			c := testClient(t, time.Second, func(*http.Request) (*http.Response, error) {
				calls++
				return response(http.StatusOK, "opaque response"), nil
			})
			r := fileRequest()
			r.Budget = NewBudget(2)
			business := errors.New("safe business failure")
			r.Inspect = func(context.Context, []byte) error {
				until := c.ObserveThrottle(r.Scope, "1")
				return errors.Join(business, &Error{Kind: Throttled, RetryAt: until})
			}
			if _, err := c.Do(t.Context(), r); !errors.Is(err, ErrBudgetExhausted) || calls != 2 {
				t.Fatalf("business retries escaped budget: calls=%d err=%v", calls, err)
			}
			r.Budget = nil
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			r.Inspect = func(context.Context, []byte) error {
				until := c.ObserveThrottle(r.Scope, "30")
				return errors.Join(business, &Error{Kind: Throttled, RetryAt: until})
			}
			if _, err := c.Do(ctx, r); !errors.Is(err, business) {
				t.Fatalf("business cause lost: %v", err)
			}
		})
	})
}
