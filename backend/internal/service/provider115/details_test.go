// details_test.go：验证账号资料与可选信息解析、异常容量拒绝及授权变更时在途响应失效。

package provider115

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// TestDetailsOptionalRequestBudgetPreservesProfile 验证长账号间隔使可选容量请求超时后仍保留已验证资料，不误报账号失效。
func TestDetailsOptionalRequestBudgetPreservesProfile(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		service := NewWithConfig(&detailsRepository{credential: "UID=123_A1_private"}, slog.New(slog.NewJSONHandler(io.Discard, nil)), time.Minute, true)
		calls := 0
		service.client.Transport = scanTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Host != "my.115.com" {
				t.Fatal("optional request escaped its foreground budget")
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"state":true,"data":{"user_name":"sample"}}`))}, nil
		})
		start := time.Now()
		details, err := service.Details(t.Context(), "req")
		if err != nil || details.Status != "valid" || details.Profile == nil || details.SpaceStatus != "unavailable" || calls != 1 || time.Since(start) != 45*time.Second {
			t.Fatalf("optional budget erased profile: details=%+v calls=%d elapsed=%s err=%v", details, calls, time.Since(start), err)
		}
	})
}

// detailsRepository 为账号详情和目录测试提供可替换的明文测试凭据。
type detailsRepository struct {
	testRepository
	credential string
}

// ProviderCredential 返回当前测试凭据，供服务验证请求前后的一致性。
func (r *detailsRepository) ProviderCredential(context.Context) (string, error) {
	return r.credential, nil
}

// TestDetails 验证账号状态、超大 ID、容量精度、设备映射及可选请求失败时的局部可用性。
func TestDetails(t *testing.T) {
	var nav = `{"state":true,"data":{"user_name":"Test","user_id":123456789012345678,"face":"https://example.com/face","vip":2,"expire":1700000000,"forever":0,"is_privilege":true}}`
	var space = `{"state":true,"data":{"space_info":{"all_total":{"size":155555555555555.25},"all_use":{"size":"2.5e3"}},"login_devices_info":{"list":[{"name":"iPhone","device":"iOS","ip":"127.0.0.1","city":"上海","utime":1700000000,"is_current":1}]}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "UID=private" {
			t.Error("credential missing")
		}
		if r.Header.Get("User-Agent") != "Mozilla/5.0 115Browser/36.0.0" {
			t.Errorf("unexpected provider user agent: %q", r.Header.Get("User-Agent"))
		}
		if r.URL.Path == "/nav" {
			_, _ = io.WriteString(w, nav)
		} else {
			_, _ = io.WriteString(w, space)
		}
	}))
	defer server.Close()
	logs := &strings.Builder{}
	repo := &detailsRepository{}
	service := New(repo, slog.New(slog.NewJSONHandler(logs, nil)))
	service.navURL = server.URL + "/nav"
	service.spaceURL = server.URL + "/space"
	result, err := service.Details(t.Context(), "request")
	if err != nil || result.Status != "unconfigured" {
		t.Fatalf("unconfigured: %+v %v", result, err)
	}
	repo.credential = "UID=private"
	result, err = service.Details(t.Context(), "request")
	if err != nil || result.Status != "valid" || result.Profile == nil || *result.Profile.UserID != "123456789012345678" || result.Profile.IsMember == nil || !*result.Profile.IsMember || result.Profile.MemberExpiresAt == nil || result.Profile.MemberExpiresAt.Unix() != 1700000000 || result.Profile.MemberForever == nil || *result.Profile.MemberForever || result.Space == nil || result.Space.TotalBytes != "155555555555555" || result.Space.UsedBytes != "2500" || result.DevicesStatus != "available" || len(result.Devices) != 1 || !result.Devices[0].IsCurrent || result.Devices[0].Name != "iPhone" || result.Devices[0].LoginAt == nil || result.Devices[0].LoginAt.Unix() != 1700000000 {
		t.Fatalf("valid response: %+v %v", result, err)
	}
	encoded, err := json.Marshal(result.Profile)
	if err != nil || strings.Contains(string(encoded), "vip") {
		t.Fatalf("raw VIP fields must not be exposed: %s (%v)", encoded, err)
	}
	space = `{"state":false}`
	result, err = service.Details(t.Context(), "request")
	if err != nil || result.Status != "valid" || result.SpaceStatus != "unavailable" || result.Space != nil {
		t.Fatalf("space failure: %+v %v", result, err)
	}
	nav = `{"state":false,"error":"请先登录,后操作！"}`
	result, err = service.Details(t.Context(), "request")
	if err != nil || result.Status != "invalid" {
		t.Fatalf("invalid: %+v %v", result, err)
	}
	nav = `{"state":false,"error":"系统错误"}`
	_, err = service.Details(t.Context(), "request")
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("provider failure: %v", err)
	}
	if strings.Contains(logs.String(), "UID=private") {
		t.Fatal("credential leaked in log")
	}
}

// TestDetailsKeepsUnknownMembershipAndUnavailableDevices 验证未知会员值保持未知，设备请求失败不使有效账号资料失效。
func TestDetailsKeepsUnknownMembershipAndUnavailableDevices(t *testing.T) {
	repo := &detailsRepository{credential: "UID=private"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/nav" {
			_, _ = io.WriteString(w, `{"state":true,"data":{"user_name":"Test","vip":9,"expire":0,"forever":0}}`)
			return
		}
		_, _ = io.WriteString(w, `{"state":false}`)
	}))
	defer server.Close()
	service := New(repo, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	service.navURL, service.spaceURL = server.URL+"/nav", server.URL+"/space"
	result, err := service.Details(t.Context(), "request")
	if err != nil || result.Status != "valid" || result.Profile == nil || result.Profile.IsMember != nil || result.Profile.MemberExpiresAt != nil || result.Profile.MemberForever == nil || *result.Profile.MemberForever || result.DevicesStatus != "unavailable" {
		t.Fatalf("unknown values should stay unknown: %+v, %v", result, err)
	}
}

// TestDetailsKeepsProfileAndSpaceWhenOptionalFieldsAreMalformed 验证可选字段类型异常不遮蔽有效资料，数字字符串按合同解析。
func TestDetailsKeepsProfileAndSpaceWhenOptionalFieldsAreMalformed(t *testing.T) {
	repo := &detailsRepository{credential: "UID=private"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/nav" {
			_, _ = io.WriteString(w, `{"state":true,"data":{"user_name":"Test","user_id":"not-a-number","face":123,"vip":"1","expire":"1700000000","forever":"1"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"state":true,"data":{"space_info":{"all_total":{"size":"2048"},"all_use":{"size":1024}},"login_devices_info":{"list":[{"name":false,"device":"iOS","ip":null,"city":"上海","utime":"1700000000","is_current":"1"}]}}}`)
	}))
	defer server.Close()
	service := New(repo, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	service.navURL, service.spaceURL = server.URL+"/nav", server.URL+"/space"
	result, err := service.Details(t.Context(), "request")
	if err != nil || result.Status != "valid" || result.Profile == nil {
		t.Fatalf("malformed optional fields must not invalidate profile: %+v, %v", result, err)
	}
	if result.Profile.Avatar != nil || result.Profile.UserID != nil || result.Profile.IsMember == nil || !*result.Profile.IsMember || result.Profile.MemberExpiresAt == nil || result.Profile.MemberForever == nil || !*result.Profile.MemberForever {
		t.Fatalf("numeric/string profile fields were not handled: %+v", result.Profile)
	}
	if result.SpaceStatus != "available" || result.DevicesStatus != "available" || result.Space == nil || len(result.Devices) != 1 || result.Devices[0].Name != "" || result.Devices[0].LoginAt == nil || !result.Devices[0].IsCurrent {
		t.Fatalf("malformed optional device fields hid valid data: %+v", result)
	}
}

// TestParseMemberKeepsUnknownValues 验证仅已知会员编码可映射，其他编码和类型保持未知。
func TestParseMemberKeepsUnknownValues(t *testing.T) {
	for _, raw := range []string{`null`, `"unknown"`, `3`, `-1`, `1.5`, `true`} {
		if got := parseMember([]byte(raw)); got != nil {
			t.Errorf("parseMember(%s) = %v, want unknown", raw, *got)
		}
	}
	for _, tc := range []struct {
		raw  string
		want bool
	}{{`0`, false}, {`"1"`, true}, {`2`, true}} {
		got := parseMember([]byte(tc.raw))
		if got == nil || *got != tc.want {
			t.Errorf("parseMember(%s) = %v, want %t", tc.raw, got, tc.want)
		}
	}
}

// TestParseBytesRejectsMalformed 验证容量解析拒绝缺失、负数、无穷和巨大指数等异常输入。
func TestParseBytesRejectsMalformed(t *testing.T) {
	for _, input := range []string{"null", `"Infinity"`, `"-1"`, `""`, `{}`, `"1e1000000000"`} {
		if _, ok := parseBytes([]byte(input)); ok {
			t.Fatalf("accepted %s", input)
		}
	}
}

// changingCredentialRepository 使用原子值模拟请求进行中的授权删除或替换。
type changingCredentialRepository struct {
	testRepository
	credential atomic.Value
}

// ProviderCredential 返回当前测试凭据，供服务验证请求前后的一致性。
func (r *changingCredentialRepository) ProviderCredential(context.Context) (string, error) {
	return r.credential.Load().(string), nil
}

// TestDetailsRejectsReplacedOrDeletedAccount 在资料请求等待期间替换或清空凭据，验证旧资料不会返回。
func TestDetailsRejectsReplacedOrDeletedAccount(t *testing.T) {
	for _, replacement := range []string{"", "UID=new"} {
		t.Run(replacement, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/nav" {
					close(entered)
					<-release
					_, _ = io.WriteString(w, `{"state":true,"data":{"user_name":"Old"}}`)
					return
				}
				_, _ = io.WriteString(w, `{"state":false}`)
			}))
			defer server.Close()
			repo := &changingCredentialRepository{}
			repo.credential.Store("UID=old")
			service := New(repo, slog.New(slog.NewJSONHandler(io.Discard, nil)))
			service.navURL, service.spaceURL = server.URL+"/nav", server.URL+"/space"
			done := make(chan error, 1)
			go func() { _, err := service.Details(t.Context(), "req"); done <- err }()
			<-entered
			repo.credential.Store(replacement)
			close(release)
			if err := <-done; !errors.Is(err, ErrUpstream) {
				t.Fatalf("stale account details returned: %v", err)
			}
		})
	}
}

// TestDetailsHTTPAuthorization 验证 HTTP 401 与已知业务登录错误一样报告 invalid，不能误当临时上游故障。
func TestDetailsHTTPAuthorization(t *testing.T) {
	service := New(&detailsRepository{credential: "UID=123_A1_private"}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	service.client.Transport = scanTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	details, err := service.Details(t.Context(), "req")
	if err != nil || details.Status != "invalid" || details.Profile != nil {
		t.Fatalf("http authorization %+v %v", details, err)
	}
}

// TestDetailsSpaceCancellation 验证账号资料成功后容量请求取消不能被可选字段降级吞掉。
func TestDetailsSpaceCancellation(t *testing.T) {
	service := New(&detailsRepository{credential: "UID=123_A1_private"}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	service.client.Transport = scanTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "my.115.com" {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"state":true,"data":{"user_name":"sample"}}`))}, nil
		}
		cancel()
		return nil, context.Canceled
	})
	_, err := service.Details(ctx, "req")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled details returned success: %v", err)
	}
}

// TestDetailsFinalCredentialCheck 验证容量读取结束后的最终检查仍保留凭据变更身份，并清除旧账号资料。
func TestDetailsFinalCredentialCheck(t *testing.T) {
	repo := &finalReadCredentialRepository{changeAt: 6}
	service := New(repo, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	service.client.Transport = scanTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"state":false}`
		if r.URL.Host == "my.115.com" {
			body = `{"state":true,"data":{"user_name":"sample"}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	details, err := service.Details(t.Context(), "req")
	if repo.reads != 6 || !errors.Is(err, ErrCredentialChanged) || details.Profile != nil || details.Status != "" {
		t.Fatalf("final details reads=%d details=%+v err=%v", repo.reads, details, err)
	}
}
