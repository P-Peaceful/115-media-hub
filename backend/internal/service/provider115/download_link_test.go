// download_link_test.go：验证各取链接口的响应格式、回退次序、账号配额以及下载凭据的内存边界。

package provider115

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	adapter "github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/provider115"
)

// TestDownloadEndpointLocatorPayload 验证加密前的字段拼写遵守 Android、Chrome 和 Web POST 的各自协议。
func TestDownloadEndpointLocatorPayload(t *testing.T) {
	for _, tc := range []struct {
		protocol downloadProtocol
		want     string
	}{
		{androidPrimary, `{"pick_code":"locator"}`},
		{androidMirror, `{"pick_code":"locator"}`},
		{chromeLink, `{"pickcode":"locator"}`},
		{webLinkPost, `{"pickcode":"locator"}`},
	} {
		payload, err := tc.protocol.locatorPayload("locator")
		if err != nil || string(payload) != tc.want {
			t.Errorf("wrong locator field on %s: %q %v", tc.protocol, payload, err)
		}
	}
}

// TestDownloadEndpointParse 验证各协议解密后的结构及身份回显，密文解码边界通过替身喂入已知 JSON。
func TestDownloadEndpointParse(t *testing.T) {
	const target = "https://media.115cdn.net/file?signature=temporary"
	identity := downloadIdentity{"900719925474099313", "locator"}
	for _, tc := range []struct {
		name     string
		protocol downloadProtocol
		payload  string
		want     string
		wantErr  error
	}{
		{"android direct", androidPrimary, `{"url":"` + target + `"}`, target, nil},
		{"android object", androidMirror, `{"fid":900719925474099313,"pick_code":"locator","url":{"url":"` + target + `"}}`, target, nil},
		{"chrome map", chromeLink, `{"900719925474099313":{"pick_code":"locator","url":{"url":"` + target + `"}}}`, target, nil},
		{"web post", webLinkPost, `{"file_url":"` + target + `","file_id":"900719925474099313"}`, target, nil},
		{"web get direct", webLinkGet, `{"state":true,"url":{"url":"` + target + `"}}`, target, nil},
		{"web get nested", webLinkGet, `{"state":1,"data":{"url":"` + target + `","pickcode":"locator"}}`, target, nil},
		{"identity mismatch", androidPrimary, `{"file_id":"123","url":"` + target + `"}`, "", ErrSourceNotFound},
		{"pickcode mismatch", webLinkPost, `{"file_url":"` + target + `","pickcode":"different"}`, "", ErrSourceNotFound},
		{"nested identity mismatch", webLinkGet, `{"state":true,"url":"` + target + `","data":{"fid":"123"}}`, "", ErrSourceNotFound},
		{"bad host", androidPrimary, `{"url":"https://115cdn.com.evil.test/sample"}`, "", ErrDownloadUntrustedTarget},
		{"unrecognized field", webLinkPost, `{"unrelated":{"url":"` + target + `"}}`, "", ErrDownloadLinkResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"state":true,"data":"fixture"}`
			if tc.protocol == webLinkGet {
				body = tc.payload
			}
			endpoint := downloadEndpoint{protocol: tc.protocol}
			got, err := endpoint.parse([]byte(body), identity, func(cipher string) ([]byte, error) {
				if cipher != "fixture" || tc.protocol == webLinkGet {
					t.Fatal("unexpected encrypted payload")
				}
				return []byte(tc.payload), nil
			})
			if !errors.Is(err, tc.wantErr) || got != tc.want {
				t.Fatalf("unexpected target presence=%t error=%v", got != "", err)
			}
		})
	}
}

// TestSidecarFallbackDownloadsWithSignedCookie 验证真实适配器按序取链，每次耗用账号发送资格，并仅转交受控下载头。
func TestSidecarFallbackDownloadsWithSignedCookie(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			locator   = "privateLocator"
			signedURL = "https://media.115cdn.net/asset?f=3&signature=private"
		)
		var requests []string
		var sendTimes []time.Time
		s, _, sink, request := newDownloadFixture(t, scanTransport(func(r *http.Request) (*http.Response, error) {
			requests = append(requests, r.Method+" "+r.URL.Host+r.URL.Path)
			if r.UserAgent() != downloadSigningAgent {
				t.Error("take-link and download used different UA")
			}
			if r.URL.Host == "media.115cdn.net" {
				if r.Header.Get("Cookie") != "dl_session=signed" {
					t.Error("CDN lacked dedicated signed cookie or received login credentials")
				}
				return downloadResponse(200, "<movie/>", 8), nil
			}
			sendTimes = append(sendTimes, time.Now())
			if !strings.Contains(r.Header.Get("Cookie"), "UID=123_") {
				t.Error("missing upstream account authorization")
			}
			if r.Method == http.MethodPost {
				verifyLinkForm(t, r, locator)
			}
			switch len(sendTimes) {
			case 1:
				return downloadResponse(http.StatusGone, "endpoint unavailable", -1), nil
			case 2:
				return downloadResponse(http.StatusNotFound, "endpoint unavailable", -1), nil
			case 3:
				return downloadResponse(http.StatusServiceUnavailable, "temporary", -1), nil
			case 4:
				return downloadResponse(200, `{"state":true,"data":"AA=="}`, -1), nil
			case 5:
				if r.URL.Query().Get("pickcode") != locator {
					t.Error("wrong GET pickcode")
				}
				resp := downloadResponse(200, `{"state":true,"url":{"url":"`+signedURL+`"}}`, -1)
				resp.Header.Add("Set-Cookie", "dl_session=signed; Secure; Domain=115cdn.net")
				resp.Header.Add("Set-Cookie", "discarded=other; Domain=external.test")
				resp.Header.Add("Set-Cookie", "UID=refreshed-account-token")
				return resp, nil
			default:
				t.Fatal("unexpected take-link request")
				return nil, ErrUpstream
			}
		}))
		s.downloads.resolve = nil
		var logs bytes.Buffer
		s.logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		budget := NewRequestBudget(5)
		receipt, err := s.DownloadSidecar(WithRequestBudget(t.Context(), budget), request, sink)
		if err != nil || receipt.Bytes != 8 || budget.Remaining() != 0 {
			t.Fatalf("sidecar download before budget exhaustion bytes=%d err=%v remaining=%d", receipt.Bytes, err, budget.Remaining())
		}
		want := []string{
			"POST proapi.115.com/android/2.0/ufile/download",
			"POST pro.api.115.com/android/2.0/ufile/download",
			"POST proapi.115.com/app/chrome/downurl",
			"POST webapi.115.com/files/download",
			"GET webapi.115.com/files/download",
			"GET media.115cdn.net/asset",
		}
		if !slices.Equal(requests, want) {
			t.Fatalf("wrong take-link sequence: %v", requests)
		}
		for i := 1; i < len(sendTimes); i++ {
			if sendTimes[i].Sub(sendTimes[i-1]) < time.Second {
				t.Fatal("take-link fallback bypassed account interval")
			}
		}
		for _, private := range []string{locator, signedURL, "dl_session=signed", "UID=123_", downloadSigningAgent} {
			if strings.Contains(logs.String(), private) {
				t.Fatal("take-link or download log exposed confidential material")
			}
		}
	})
}

// verifyLinkForm 检查固定 POST 端点只发送表单密文，Chrome 兼容通道额外包含时间参数。
func verifyLinkForm(t *testing.T, r *http.Request, locator string) {
	t.Helper()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	form, err := url.ParseQuery(string(data))
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := base64.StdEncoding.DecodeString(form.Get("data"))
	if err != nil || len(cipher) != 128 || strings.Contains(string(data), locator) || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatal("encrypted take-link form missing or invalid")
	}
	if (r.URL.Path == "/app/chrome/downurl") != (r.URL.Query().Get("t") != "") {
		t.Fatal("wrong Chrome timestamp behavior")
	}
}

// TestDownloadLinkStopsOnHardFailures 验证账号授权、源文件缺失、冷却和请求额度耗尽均停止回退。
func TestDownloadLinkStopsOnHardFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		budget int
		want   error
	}{
		{"authentication", 200, `{"state":false,"errNo":99}`, 5, ErrInvalidAuthorization},
		{"missing source", 200, `{"state":0,"errcode":50003}`, 5, ErrSourceNotFound},
		{"HTTP rate limit", 429, "", 5, ErrDownloadThrottled},
		{"business rate limit", 200, `{"state":false,"errNo":590075}`, 5, ErrDownloadThrottled},
		{"budget exhausted", 503, "", 1, ErrRequestBudgetExhausted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				s, repo, _, request := newDownloadFixture(t, scanTransport(func(*http.Request) (*http.Response, error) {
					calls++
					return downloadResponse(tc.status, tc.body, -1), nil
				}))
				ctx := WithRequestBudget(t.Context(), NewRequestBudget(tc.budget))
				_, err := s.resolveDownloadTarget(ctx, repo.cookie, request.SourceFileID, request.PlaybackCode, downloadSigningAgent, adapter.DownloadLink, adapter.Background)
				if calls != 1 || !errors.Is(err, tc.want) || errors.Is(err, ErrDownloadTakeLink) {
					t.Fatalf("wrong fallback after hard failure calls=%d err=%v", calls, err)
				}
			})
		})
	}
}

// TestPlaybackRedirectRejectsCookieOnlyTarget 验证 302 不使用需 Cookie 的链接，播放器无须接收签发凭据。
func TestPlaybackRedirectRejectsCookieOnlyTarget(t *testing.T) {
	for _, tc := range []struct {
		suffix string
		cookie bool
		want   bool
	}{
		{"", false, true},
		{"", true, false},
		{"?f=2", true, true},
		{"?f=3", false, false},
		{"?f=3", true, false},
	} {
		target := downloadTarget{URL: "https://media.115cdn.net/file" + tc.suffix}
		if tc.cookie {
			target.Cookies = []*http.Cookie{{Name: "dl_session", Value: "signed"}}
		}
		if target.permitsPlaybackRedirect() != tc.want {
			t.Fatalf("wrong redirect eligibility for f=%q cookie=%t", tc.suffix, tc.cookie)
		}
	}
}
