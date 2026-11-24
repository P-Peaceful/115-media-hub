package provider115

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolvePlaybackUpstreamFailuresAndRedaction(t *testing.T) {
	const (
		cookie = "UID=1234_A1_private; CID=secret"
		code   = "samplePrivateLocator"
		agent  = "SamplePrivatePlayer/1"
	)
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"invalid authorization", http.StatusOK, `{"state":false,"errno":"99","data":[]}`, ErrInvalidAuthorization},
		{"missing file", http.StatusOK, `{"state":0,"errno":50003}`, ErrSourceNotFound},
		{"throttled", http.StatusTooManyRequests, "", errPlaybackThrottled},
		{"redirect blocked", http.StatusFound, "", ErrUpstream},
		{"unexpected failure", http.StatusOK, `{"state":false,"error":"private remote error"}`, ErrUpstream},
		{"oversized response", http.StatusOK, strings.Repeat(" ", 64<<10) + `{"state":true}`, ErrUpstream},
		{"invalid ciphertext", http.StatusOK, `{"state":true,"data":"AA=="}`, ErrUpstream},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Cookie") != cookie || r.UserAgent() != agent || r.URL.Query().Get("t") == "" {
					t.Error("unexpected upstream request")
				}
				if err := r.ParseForm(); err != nil {
					t.Errorf("decode request form: %v", err)
				}
				ciphertext, err := base64.StdEncoding.DecodeString(r.PostForm.Get("data"))
				if err != nil || len(ciphertext) != 128 || strings.Contains(r.PostForm.Get("data"), code) {
					t.Error("expected encoded playback payload")
				}
				if tc.status == http.StatusFound {
					w.Header().Set("Location", "/never-follow")
				}
				if r.URL.Path == "/never-follow" {
					t.Error("followed a redirect with an authenticated upstream request")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.WriteHeader(tc.status)
				if _, err := io.WriteString(w, tc.body); err != nil {
					t.Errorf("write fixture: %v", err)
				}
			}))
			defer upstream.Close()
			var logs strings.Builder
			service := New(&detailsRepository{credential: cookie}, slog.New(slog.NewJSONHandler(&logs, nil)))
			service.playbackURL = upstream.URL
			got, err := service.ResolvePlayback(t.Context(), "1234", "235", code, agent, "playback-req")
			if got != "" || !errors.Is(err, tc.want) {
				t.Fatalf("resolve returned a link: %t, error: %v", got != "", err)
			}
			for _, expected := range []string{"resolve_playback", "playback-req", "source_file_id", "duration_ms", "outcome", "error_code"} {
				if !strings.Contains(logs.String(), expected) {
					t.Errorf("missing log attribute %s", expected)
				}
			}
			for _, private := range []string{cookie, code, agent, "private remote error"} {
				if strings.Contains(logs.String(), private) {
					t.Fatal("playback log exposed upstream input or response")
				}
			}
		})
	}
}

func TestResolvePlaybackRequiresAccountOwnership(t *testing.T) {
	cases := []struct {
		cookie string
		want   error
	}{
		{"", ErrUnconfigured},
		{"UID=12345_A1_private", ErrInvalidAuthorization},
		{"UID=invalid", ErrInvalidAuthorization},
	}
	for _, tc := range cases {
		service := New(&detailsRepository{credential: tc.cookie}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
		service.playbackURL = "://otherwise fails with upstream error"
		_, err := service.ResolvePlayback(t.Context(), "1234", "5", "loc", "", "req")
		if !errors.Is(err, tc.want) {
			t.Fatalf("expected %v, got %v", tc.want, err)
		}
	}
}

func TestPlaybackTargetVerifiesSourceAndRedirect(t *testing.T) {
	const valid = "https://media.115cdn.net/sample?signature=temporary"
	fixture := func(id, pickCode, target string) []byte {
		return fmt.Appendf(nil, `{%q:{"pick_code":%q,"url":{"url":%q}}}`, id, pickCode, target)
	}
	if got, err := playbackTarget(fixture("42", "expected", valid), "42", "expected"); err != nil || got != valid {
		t.Fatal("rejected an expected 115 CDN target")
	}
	for _, payload := range [][]byte{fixture("24", "expected", valid), fixture("42", "different", valid)} {
		if _, err := playbackTarget(payload, "42", "expected"); !errors.Is(err, ErrSourceNotFound) {
			t.Fatalf("identity mismatch: %v", err)
		}
	}
	for _, target := range []string{
		"//115.com/sample", "http://115.com:80/sample", "https://115.com@external.test/sample",
		"https://login@115cdn.net/sample", "https://115cdn.net.external.test/sample",
		"https://fake115.com/sample", "https://115.com/sample#external", "https://127.0.0.1/sample",
	} {
		if _, err := playbackTarget(fixture("42", "expected", target), "42", "expected"); !errors.Is(err, ErrUpstream) {
			t.Errorf("accepted invalid target %q", target)
		}
	}
}
