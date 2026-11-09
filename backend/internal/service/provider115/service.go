// Package provider115 implements the single-account 115 QR authorization flow.
package provider115

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("QR session not found")
var ErrUpstream = errors.New("115 service unavailable")

type Account struct {
	Connected   bool       `json:"connected"`
	AccountName *string    `json:"account_name"`
	ConnectedAt *time.Time `json:"connected_at"`
}
type QR struct {
	ID        string    `json:"id"`
	QRURL     string    `json:"qr_url"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Repository interface {
	ProviderAccount(context.Context) (Account, error)
	SaveProviderAccount(context.Context, string, string, time.Time, string, string, string) error
	DeleteProviderAccount(context.Context, string, string, time.Time) error
}
type pending struct {
	QR
	owner, uid, sign string
	timestamp        int64
	busy             bool
}
type Service struct {
	repo          Repository
	client        *http.Client
	logger        *slog.Logger
	mu            sync.Mutex
	active        *pending
	generation    uint64
	startingOwner string
	expiryTimer   *time.Timer
	baseURL       string
	navURL        string
	spaceURL      string
	filesURL      string
	now           func() time.Time
}

func New(repo Repository, logger *slog.Logger) *Service {
	return &Service{repo: repo, logger: logger.With("module", "provider_115"), client: &http.Client{Timeout: 65 * time.Second}, baseURL: "https://qrcodeapi.115.com", navURL: "https://my.115.com/?ct=ajax&ac=nav", spaceURL: "https://webapi.115.com/files/index_info", filesURL: "https://webapi.115.com/files", now: time.Now}
}
func (s *Service) log(ctx context.Context, operation, requestID string, start time.Time, err error) {
	outcome := "succeeded"
	attrs := []any{"operation", operation, "request_id", requestID, "duration_ms", time.Since(start).Milliseconds()}
	if err != nil {
		outcome = "failed"
		attrs = append(attrs, "error_code", "provider_115_failed")
	}
	attrs = append(attrs, "outcome", outcome)
	s.logger.InfoContext(ctx, "115 operation completed", attrs...)
}
func (s *Service) Account(ctx context.Context) (Account, error) { return s.repo.ProviderAccount(ctx) }
func (s *Service) Start(ctx context.Context, owner, requestID string) (result QR, resultErr error) {
	start := time.Now()
	defer func() { s.log(ctx, "start_qr", requestID, start, resultErr) }()
	s.mu.Lock()
	s.generation++
	generation := s.generation
	s.clearLocked()
	s.startingOwner = owner
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.generation == generation {
			s.startingOwner = ""
		}
	}()
	var token struct {
		State upstreamState `json:"state"`
		Data  struct {
			UID  string `json:"uid"`
			Time int64  `json:"time"`
			Sign string `json:"sign"`
		} `json:"data"`
	}
	data, err := s.request(ctx, http.MethodGet, "/api/1.0/web/1.0/token/", nil)
	if err != nil || json.Unmarshal(data, &token) != nil || !bool(token.State) || token.Data.UID == "" || token.Data.Sign == "" {
		return QR{}, ErrUpstream
	}
	image, err := s.request(ctx, http.MethodGet, "/api/1.0/mac/1.0/qrcode?"+url.Values{"uid": {token.Data.UID}}.Encode(), nil)
	if err != nil || len(image) == 0 {
		return QR{}, ErrUpstream
	}
	id := rand.Text()
	result = QR{ID: id, QRURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(image), Status: "waiting", ExpiresAt: s.now().UTC().Add(2 * time.Minute)}
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generation {
		return QR{}, ErrNotFound
	}
	s.startingOwner = ""
	s.active = &pending{QR: result, owner: owner, uid: token.Data.UID, sign: token.Data.Sign, timestamp: token.Data.Time}
	p := s.active
	s.expiryTimer = time.AfterFunc(2*time.Minute, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.active == p {
			s.clearLocked()
			s.log(context.Background(), "expire_qr", requestID, result.ExpiresAt.Add(-2*time.Minute), nil)
		}
	})
	return result, nil
}
func (s *Service) Poll(ctx context.Context, owner, id, actor, requestID string) (result QR, resultErr error) {
	start := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.active
	if p == nil || p.owner != owner || p.ID != id {
		return QR{}, ErrNotFound
	}
	if p.busy {
		return QR{}, ErrUpstream
	}
	p.busy = true
	defer func() { p.busy = false }()
	if !s.now().Before(p.ExpiresAt) {
		p.Status = "expired"
		s.log(ctx, "expire_qr", requestID, start, nil)
		result = p.QR
		s.clearLocked()
		return result, nil
	}
	query := url.Values{"uid": {p.uid}, "time": {strconv.FormatInt(p.timestamp, 10)}, "sign": {p.sign}}
	s.mu.Unlock()
	data, err := s.request(ctx, http.MethodGet, "/get/status/?"+query.Encode(), nil)
	s.mu.Lock()
	if s.active != p {
		return QR{}, ErrNotFound
	}
	if err != nil {
		s.log(ctx, "poll_qr", requestID, start, err)
		return QR{}, ErrUpstream
	}
	var status struct {
		State upstreamState `json:"state"`
		Data  struct {
			Status *int `json:"status"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &status) != nil || !bool(status.State) || status.Data.Status == nil {
		s.log(ctx, "poll_qr", requestID, start, ErrUpstream)
		return QR{}, ErrUpstream
	}
	switch *status.Data.Status {
	case 0:
		p.Status = "waiting"
	case 1:
		p.Status = "scanned"
	case -1:
		p.Status = "expired"
		s.log(ctx, "expire_qr", requestID, start, nil)
	case -2:
		p.Status = "canceled"
		s.log(ctx, "cancel_qr", requestID, start, nil)
	case 2:
		form := url.Values{"account": {p.uid}}
		s.mu.Unlock()
		data, err = s.request(ctx, http.MethodPost, "/app/1.0/alipaymini/1.0/login/qrcode/", strings.NewReader(form.Encode()))
		s.mu.Lock()
		if s.active != p {
			return QR{}, ErrNotFound
		}
		if !s.now().Before(p.ExpiresAt) {
			s.clearLocked()
			p.Status = "expired"
			s.log(ctx, "expire_qr", requestID, start, nil)
			return p.QR, nil
		}
		if err != nil {
			s.log(ctx, "authorize", requestID, start, err)
			return QR{}, ErrUpstream
		}
		var login struct {
			State upstreamState `json:"state"`
			Data  struct {
				UserName string         `json:"user_name"`
				Cookie   map[string]any `json:"cookie"`
			} `json:"data"`
		}
		if json.Unmarshal(data, &login) != nil || !bool(login.State) || len(login.Data.Cookie) == 0 {
			s.log(ctx, "authorize", requestID, start, ErrUpstream)
			return QR{}, ErrUpstream
		}
		parts := []string{}
		for _, key := range []string{"UID", "CID", "SEID", "KID"} {
			if value, ok := login.Data.Cookie[key]; ok && value != nil && fmt.Sprint(value) != "" {
				parts = append(parts, key+"="+fmt.Sprint(value))
			}
		}
		if login.Data.Cookie["UID"] == nil || login.Data.Cookie["CID"] == nil || login.Data.Cookie["SEID"] == nil || fmt.Sprint(login.Data.Cookie["UID"]) == "" || fmt.Sprint(login.Data.Cookie["CID"]) == "" || fmt.Sprint(login.Data.Cookie["SEID"]) == "" || len(parts) < 3 {
			s.log(ctx, "authorize", requestID, start, ErrUpstream)
			return QR{}, ErrUpstream
		}
		if err = s.repo.SaveProviderAccount(ctx, login.Data.UserName, strings.Join(parts, "; "), s.now().UTC(), actor, requestID, owner); err != nil {
			s.log(ctx, "authorize", requestID, start, err)
			return QR{}, err
		}
		p.Status = "succeeded"
		s.log(ctx, "authorize", requestID, start, nil)
	default:
		s.log(ctx, "poll_qr", requestID, start, ErrUpstream)
		return QR{}, ErrUpstream
	}
	result = p.QR
	if p.Status == "expired" || p.Status == "canceled" || p.Status == "succeeded" {
		s.clearLocked()
	}
	return result, nil
}
func (s *Service) Cancel(ctx context.Context, owner, id, requestID string) error {
	start := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil || s.active.owner != owner || s.active.ID != id {
		return ErrNotFound
	}
	s.clearLocked()
	s.generation++
	s.log(ctx, "cancel_qr", requestID, start, nil)
	return nil
}
func (s *Service) Remove(ctx context.Context, owner, actor, requestID string) (resultErr error) {
	start := time.Now()
	defer func() { s.log(ctx, "remove_account", requestID, start, resultErr) }()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearLocked()
	s.generation++
	return s.repo.DeleteProviderAccount(ctx, actor, requestID, s.now().UTC())
}

// clearLocked releases transient QR data. The caller must hold s.mu.
func (s *Service) clearLocked() {
	if s.expiryTimer != nil {
		s.expiryTimer.Stop()
		s.expiryTimer = nil
	}
	s.active = nil
}

// InvalidateSession discards QR state belonging to a logged-out session.
func (s *Service) InvalidateSession(owner string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.startingOwner == owner || (s.active != nil && s.active.owner == owner) {
		s.generation++
		s.startingOwner = ""
		s.clearLocked()
	}
}

func (s *Service) request(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 115Browser/36.0.0")
	req.Header.Set("Referer", "https://115.com/")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	closeErr := resp.Body.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, ErrUpstream
	}
	return data, nil
}

// upstreamState accepts the provider's numeric and boolean state encodings.
type upstreamState bool

func (s *upstreamState) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case "1", "true":
		*s = true
		return nil
	case "0", "false":
		*s = false
		return nil
	default:
		return errors.New("invalid provider state")
	}
}
