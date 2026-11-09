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
)

type detailsRepository struct {
	testRepository
	credential string
}

func (r *detailsRepository) ProviderCredential(context.Context) (string, error) {
	return r.credential, nil
}
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
func TestParseBytesRejectsMalformed(t *testing.T) {
	for _, input := range []string{"null", `"Infinity"`, `"-1"`, `""`, `{}`, `"1e1000000000"`} {
		if _, ok := parseBytes([]byte(input)); ok {
			t.Fatalf("accepted %s", input)
		}
	}
}

type changingCredentialRepository struct {
	testRepository
	credential atomic.Value
}

func (r *changingCredentialRepository) ProviderCredential(context.Context) (string, error) {
	return r.credential.Load().(string), nil
}

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
