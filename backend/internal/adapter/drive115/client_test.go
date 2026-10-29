package drive115

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPathMixedIDsRepeatedAndRootFallback(t *testing.T) {
	calls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/files" {
			t.Errorf("unexpected protocol %s", r.URL.Path)
		}
		if calls == 3 {
			w.Write([]byte(`{"state":true,"path":[{"cid":0,"name":"根目录"}]}`))
			return
		}
		w.Write([]byte(`{"state":true,"path":[{"cid":0,"name":"根目录"},{"cid":"3549698981099472286","pid":0,"aid":"1","name":"电影"}],"count":0,"data":[]}`))
	})
	previous := http.DefaultTransport
	http.DefaultTransport = handlerTransport{handler}
	defer func() { http.DefaultTransport = previous }()
	d := New(time.Second)
	for i := 0; i < 2; i++ {
		got, err := d.StatDirectory(context.Background(), Credential{}, "3549698981099472286")
		if err != nil || len(got.Breadcrumbs) != 2 || got.Name != "电影" {
			t.Fatalf("got %+v, %v", got, err)
		}
	}
	if _, err := d.StatDirectory(context.Background(), Credential{}, "3549698981099472286"); !errors.Is(err, ErrDirectoryNotFound) {
		t.Fatalf("fallback error %v", err)
	}
}

type handlerTransport struct{ handler http.Handler }

func (t handlerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	w := httptest.NewRecorder()
	t.handler.ServeHTTP(w, r)
	return w.Result(), nil
}
func testDriver(handler http.HandlerFunc) *Driver {
	d := New(time.Second)
	d.http.Transport = handlerTransport{handler}
	return d
}

func TestListingProtocolAndEntries(t *testing.T) {
	d := testDriver(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for key, want := range map[string]string{"cid": "17", "offset": "20", "limit": "10", "cur": "1", "type": "4", "search_value": "movie", "o": "user_ptime"} {
			if q.Get(key) != want {
				t.Errorf("%s=%q", key, q.Get(key))
			}
		}
		if r.Header.Get("User-Agent") != userAgent || r.Header.Get("Referer") != "https://115.com/" {
			t.Error("missing headers")
		}
		w.Write([]byte(`{"state":true,"cid":17,"count":"22","data":[{"f":"0","cid":3549698981099472286,"pid":"17","n":"folder","t":"1700000000"},{"f":1,"fid":"3549698981099472287","cid":17,"n":"film.mkv","s":"1024","t":"2026-10-30 10:30"}]}`))
	})
	got, err := d.ListDirectory(context.Background(), Credential{}, "17", 20, 10, "video", "movie")
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 22 || len(got.Entries) != 2 || got.Entries[0].ID != "3549698981099472286" || !got.Entries[0].IsDirectory || got.Entries[1].Size != 1024 || got.Entries[1].ModifiedAt.Hour() != 2 {
		t.Fatalf("page %+v", got)
	}
}

func TestFailuresAndMalformedSuccess(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"invalid credential", `{"state":false,"errno":99,"error":"登录失效"}`, 200, ErrCredentialInvalid},
		{"rate limit", `{"error":"slow down"}`, 429, ErrLimited},
		{"http error", `{"state":true}`, 502, ErrUnreachable},
		{"malformed", `not json`, 200, ErrUnreachable},
		{"missing state", `{"cid":1,"count":0,"data":[]}`, 200, ErrUnreachable},
		{"missing list", `{"state":true,"cid":1,"count":0}`, 200, ErrUnreachable},
		{"missing count", `{"state":true,"cid":1,"data":[]}`, 200, ErrUnreachable},
		{"wrong identity", `{"state":true,"cid":0,"count":0,"data":[]}`, 200, ErrDirectoryNotFound},
		{"wrong area", `{"state":true,"cid":1,"aid":7,"count":0,"data":[]}`, 200, ErrDirectoryNotFound},
		{"bad id", `{"state":true,"cid":1.2,"count":0,"data":[]}`, 200, ErrUnreachable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDriver(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); w.Write([]byte(tc.body)) })
			_, err := d.ListDirectory(context.Background(), Credential{}, "1", 0, 10, "all", "")
			if !errors.Is(err, tc.want) {
				t.Fatalf("error %v want %v", err, tc.want)
			}
		})
	}
}

func TestContextAndConcurrentCredentials(t *testing.T) {
	d := testDriver(func(w http.ResponseWriter, r *http.Request) {
		c, _ := r.Cookie("UID")
		if c == nil || c.Value != r.URL.Query().Get("search_value") {
			t.Error("credentials crossed requests")
		}
		w.Write([]byte(`{"state":true,"cid":0,"count":0,"data":[]}`))
	})
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			uid := strconv.Itoa(i)
			_, err := d.ListDirectory(context.Background(), Credential{UID: uid}, "0", 0, 10, "all", uid)
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := d.ListDirectory(ctx, Credential{}, "0", 0, 1, "all", "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation %v", err)
	}
	d.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err = d.ListDirectory(ctx, Credential{}, "0", 0, 1, "all", "")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestQRCodeAllDevicesAndStates(t *testing.T) {
	for _, device := range []Device{DeviceWeb, DeviceAlipayMini, DeviceWechatMini} {
		t.Run(string(device), func(t *testing.T) {
			status := "0"
			missingKID := false
			d := testDriver(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/1.0/web/1.0/token/":
					w.Write([]byte(`{"state":1,"data":{"uid":"session","time":"1700000000","sign":"qr-sign"}}`))
				case "/api/1.0/mac/1.0/qrcode":
					w.Write([]byte("\x89PNG\r\n\x1a\nimage"))
				case "/get/status/":
					if r.URL.Query().Get("sign") != "qr-sign" {
						t.Error("missing signature")
					}
					fmt.Fprintf(w, `{"state":1,"data":{"status":%s}}`, status)
				case "/app/1.0/" + string(device) + "/1.0/login/qrcode/":
					if r.Method != "POST" {
						t.Error("wrong method")
					}
					r.ParseForm()
					if r.Form.Get("account") != "session" || r.Form.Get("app") != "" {
						t.Error("wrong form")
					}
					kid := `"KID":"kid",`
					if missingKID {
						kid = ""
					}
					fmt.Fprintf(w, `{"state":1,"data":{"cookie":{%s"UID":"uid","CID":"cid","SEID":"seid"}}}`, kid)
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
				}
			})
			ch, err := d.StartQRCode(context.Background(), device)
			if err != nil {
				t.Fatal(err)
			}
			for value, want := range map[string]Status{"0": Waiting, "1": Scanned, "2": Allowed, "-1": Expired, "-2": Canceled} {
				status = value
				got, err := d.PollQRCode(context.Background(), ch)
				if err != nil || got != want {
					t.Fatalf("status %s: %s %v", value, got, err)
				}
			}
			status = "3"
			if _, err := d.PollQRCode(context.Background(), ch); !errors.Is(err, ErrUnknownStatus) {
				t.Fatalf("unknown state %v", err)
			}
			cr, err := d.CompleteQRCode(context.Background(), ch, device)
			if err != nil || cr.KID != "kid" {
				t.Fatalf("credential %+v %v", cr, err)
			}
			missingKID = true
			if _, err := d.CompleteQRCode(context.Background(), ch, device); err == nil {
				t.Fatal("accepted incomplete credential")
			}
		})
	}
	d := testDriver(func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected network") })
	if _, err := d.StartQRCode(context.Background(), "bad"); !errors.Is(err, ErrInvalidDevice) {
		t.Fatal(err)
	}
	if _, err := d.PollQRCode(context.Background(), Challenge{}); !errors.Is(err, ErrInvalidChallenge) {
		t.Fatal(err)
	}
}

func TestVerifyIdentityProfileAndOptionalFailure(t *testing.T) {
	for _, optionalFailure := range []bool{false, true} {
		d := testDriver(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Host + r.URL.Path {
			case "my.115.com/":
				w.Write([]byte(`{"state":true,"data":{"user_id":"3549698981099472286","user_name":"user","vip":"2","expire":"4102444800"}}`))
			case "webapi.115.com/files":
				w.Write([]byte(`{"state":true,"cid":0,"count":0,"data":[]}`))
			case "webapi.115.com/user/vip_limit":
				w.Write([]byte(`{"state":true,"data":{"user_limit":{"level_name":"upstream name"}}}`))
			case "webapi.115.com/files/index_info":
				if optionalFailure {
					w.WriteHeader(502)
					return
				}
				w.Write([]byte(`{"state":true,"data":{"space_info":{"all_total":{"size":"155555555555555.25"},"all_use":{"size":1.5e6},"all_remain":{"size":0}},"login_devices_info":{"list":[]}}}`))
			default:
				t.Error("unexpected endpoint")
			}
		})
		evidence, err := d.VerifyCredential(context.Background(), Credential{})
		if err != nil || !evidence.RootReadable || evidence.AccountID != "3549698981099472286" || evidence.Profile.MembershipStatus != "unknown" {
			t.Fatalf("evidence %+v %v", evidence, err)
		}
		if optionalFailure {
			if evidence.Profile.StorageTotalBytes != nil {
				t.Fatal("fabricated storage")
			}
		} else if evidence.Profile.StorageTotalBytes == nil || *evidence.Profile.StorageTotalBytes != 155555555555555 || *evidence.Profile.StorageUsedBytes != 1500000 || *evidence.Profile.StorageFreeBytes != 0 || *evidence.Profile.DeviceCount != 0 {
			t.Fatalf("profile %+v", evidence.Profile)
		}
	}
}

func TestSensitiveErrorsAndQRCodeFailures(t *testing.T) {
	const secret = "known-secret-value"
	for _, body := range []string{
		`{"state":false,"errno":"0","code":99,"message":"refused known-secret-value sign=qr-secret token=token-secret"}`,
		`{"state":false,"errno":99,"error":"Cookie=UID=secret; SEID=other-secret"}`,
	} {
		d := testDriver(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
		_, err := d.ListDirectory(context.Background(), Credential{UID: secret}, "0", 0, 1, "all", "")
		if !errors.Is(err, ErrCredentialInvalid) {
			t.Fatalf("category %v", err)
		}
		for _, value := range []string{secret, "qr-secret", "token-secret", "other-secret", "UID=secret"} {
			if strings.Contains(err.Error(), value) {
				t.Fatalf("leaked error %s", err)
			}
		}
	}
	d := testDriver(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"state":false,"code":40101004,"error":"denied uid=session-secret"}`))
	})
	_, err := d.PollQRCode(context.Background(), Challenge{opaque: &qrSession{UID: "session-secret", Time: "1", Sign: "signed-secret"}})
	if !errors.Is(err, ErrLimited) || strings.Contains(err.Error(), "session-secret") {
		t.Fatalf("poll failure %v", err)
	}
	d.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, fmt.Errorf("network url sign=secret-token") })
	_, err = d.ListDirectory(context.Background(), Credential{}, "0", 0, 1, "all", "")
	if !errors.Is(err, ErrUnreachable) || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("transport error %v", err)
	}
}

func TestQRMalformedResponsesAndMissingState(t *testing.T) {
	for _, body := range []string{`{"state":1,"data":{}}`, `{"state":1,"data":{"status":null}}`, `{"data":{"status":0}}`} {
		d := testDriver(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
		if _, err := d.PollQRCode(context.Background(), Challenge{opaque: &qrSession{}}); err == nil {
			t.Fatalf("accepted malformed poll %s", body)
		}
	}
	for _, body := range []string{`{"state":1,"data":{}}`, `{"state":1,"data":{"uid":"u","time":1}}`, `{"state":false,"code":99}`} {
		d := testDriver(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
		if _, err := d.StartQRCode(context.Background(), DeviceWeb); err == nil {
			t.Fatalf("accepted malformed token %s", body)
		}
	}
}

func TestRootFailurePreservesIdentityAndOptionalProfile(t *testing.T) {
	for _, tc := range []struct {
		body string
		want error
	}{
		{`{"state":false,"errno":1234,"error":"permission denied"}`, ErrRootUnreadable},
		{`{"state":false,"errno":99,"error":"login expired"}`, ErrCredentialInvalid},
	} {
		d := testDriver(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Host == "my.115.com" {
				w.Write([]byte(`{"state":true,"data":{"user_id":123,"user_name":"user","vip":null,"expire":"invalid"}}`))
			} else {
				w.Write([]byte(tc.body))
			}
		})
		evidence, err := d.VerifyCredential(context.Background(), Credential{})
		if !errors.Is(err, tc.want) {
			t.Fatalf("error %v", err)
		}
		if tc.want == ErrRootUnreadable && (evidence.AccountID != "123" || evidence.Profile.MembershipStatus != "unknown") {
			t.Fatalf("lost evidence %+v", evidence)
		}
	}
}

func TestMalformedOptionalPrivilegeDoesNotProvideMembershipEvidence(t *testing.T) {
	for _, privilege := range []string{
		`{"state":true,"expire":"invalid"}`,
		`{"expire":"invalid","state":true}`,
	} {
		d := testDriver(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Host + r.URL.Path {
			case "my.115.com/":
				fmt.Fprintf(w, `{"state":true,"data":{"user_id":123,"user_name":"user","privilege":%s}}`, privilege)
			case "webapi.115.com/files":
				w.Write([]byte(`{"state":true,"cid":0,"count":0,"data":[]}`))
			case "webapi.115.com/files/index_info", "webapi.115.com/user/vip_limit":
				w.WriteHeader(http.StatusBadGateway)
			default:
				t.Errorf("unexpected endpoint %s", r.URL.Path)
				w.WriteHeader(http.StatusBadGateway)
			}
		})
		evidence, err := d.VerifyCredential(context.Background(), Credential{})
		if err != nil || !evidence.RootReadable || evidence.AccountID != "123" {
			t.Fatalf("lost account evidence: %+v, %v", evidence, err)
		}
		if evidence.Profile.MembershipStatus != "unknown" || evidence.Profile.MembershipExpiresAt != nil {
			t.Fatalf("malformed optional privilege became membership evidence: %+v", evidence.Profile)
		}
	}
}
