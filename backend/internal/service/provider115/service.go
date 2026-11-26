// service.go：负责单账号扫码授权的创建、轮询、取消及会话失效处理，管理临时二维码状态与凭据保存边界。

// Package provider115 管理单账号 115 授权、网盘目录和播放链接。
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

	adapter "github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/provider115"
)

// ErrNotFound 表示二维码不存在、已结束或不属于当前会话。
var ErrNotFound = errors.New("QR session not found")

// ErrUpstream 表示上游响应或请求不可用；不携带上游敏感正文。
var ErrUpstream = errors.New("115 service unavailable")

// Account 描述本地保存的账号连接状态，不包含 Cookie。
type Account struct {
	Connected   bool       `json:"connected"`
	AccountName *string    `json:"account_name"`
	ConnectedAt *time.Time `json:"connected_at"`
}

// QR 描述返回客户端的二维码状态和过期时间，不暴露上游签名。
type QR struct {
	ID        string    `json:"id"`
	QRURL     string    `json:"qr_url"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Repository 提供账号状态及带审计信息的授权保存、删除边界。
type Repository interface {
	ProviderAccount(context.Context) (Account, error)
	SaveProviderAccount(context.Context, string, string, time.Time, string, string, string) error
	DeleteProviderAccount(context.Context, string, string, time.Time) error
}

// pending 保存单个会话的临时扫码材料，访问时由服务互斥锁保护。
type pending struct {
	QR
	owner, uid, sign string
	timestamp        int64
	busy             bool
}

// Service 管理单账号授权及上游客户端；互斥锁保护临时二维码和授权代次。
type Service struct {
	repo            Repository
	client          *http.Client
	logger          *slog.Logger
	mu              sync.Mutex
	active          *pending
	generation      uint64
	startingOwner   string
	expiryTimer     *time.Timer
	baseURL         string
	navURL          string
	spaceURL        string
	filesURL        string
	downloadLinks   []downloadEndpoint
	adapterOnce     sync.Once
	adapterClient   *adapter.Client
	adapterErr      error
	requestInterval time.Duration
	fastScan        bool
	categoryURL     string
	directoriesURL  string
	downloads       downloadState
	now             func() time.Time
}

// New 构造使用默认账号节流和快速扫描策略的单账号服务。
func New(repo Repository, logger *slog.Logger) *Service {
	return NewWithConfig(repo, logger, time.Second, true)
}

// NewWithConfig 注入账号读取间隔和正式扫描模式；客户端在首次调用时固定，整个服务共享冷却。
func NewWithConfig(repo Repository, logger *slog.Logger, interval time.Duration, fastScan bool) *Service {
	if interval == 0 {
		interval = time.Second
	}
	return &Service{
		requestInterval: interval, fastScan: fastScan, categoryURL: "https://webapi.115.com/category/get", directoriesURL: "https://proapi.115.com/app/chrome/downfolders",
		repo: repo, logger: logger.With("module", "provider_115"), client: &http.Client{Timeout: 65 * time.Second},
		baseURL: "https://qrcodeapi.115.com", navURL: "https://my.115.com/?ct=ajax&ac=nav",
		spaceURL: "https://webapi.115.com/files/index_info", filesURL: "https://webapi.115.com/files",
		downloadLinks: downloadEndpoints(), now: time.Now,
	}
}

// log 记录操作的真实结果与稳定错误分类；目录成功分页及客户端取消只记录 DEBUG，避免扫描日志洪泛。
func (s *Service) log(ctx context.Context, operation, requestID string, start time.Time, err error) {
	level := slog.LevelInfo
	outcome := "succeeded"
	attrs := []any{"operation", operation, "request_id", requestID, "duration_ms", time.Since(start).Milliseconds()}
	if err != nil {
		level, outcome = slog.LevelWarn, "failed"
		code := "provider_115_failed"
		switch {
		case errors.Is(err, context.Canceled):
			level, outcome, code = slog.LevelDebug, "canceled", "request_canceled"
		case errors.Is(err, context.DeadlineExceeded):
			code = "provider_timeout"
		case errors.Is(err, ErrCredentialChanged):
			code = "provider_credential_changed"
		case errors.Is(err, ErrInvalidAuthorization):
			code = "provider_auth_invalid"
		case errors.Is(err, errPlaybackThrottled):
			code = "provider_throttled"
		}
		attrs = append(attrs, "error_code", code)
	} else if operation == "list_files" {
		// 批量扫描由 STRM 汇总进度，逐页成功记录只用于调试。
		level = slog.LevelDebug
	}
	attrs = append(attrs, "outcome", outcome)
	s.logger.Log(ctx, level, "115 operation completed", attrs...)
}

// Account 读取本地已保存的账号连接状态，不向上游请求新资料。
func (s *Service) Account(ctx context.Context) (Account, error) { return s.repo.ProviderAccount(ctx) }

// Start 替换当前二维码并获取扫码材料；以代次检查阻止过期创建请求覆盖新会话。
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
		return QR{}, errors.Join(ErrUpstream, err)
	}
	image, err := s.request(ctx, http.MethodGet, "/api/1.0/mac/1.0/qrcode?"+url.Values{"uid": {token.Data.UID}}.Encode(), nil)
	if err != nil || len(image) == 0 {
		return QR{}, errors.Join(ErrUpstream, err)
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

// Poll 校验会话归属并轮询扫码状态；网络等待时释放锁，确认状态仍有效后才交换并保存授权。
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
		return QR{}, errors.Join(ErrUpstream, err)
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
			return QR{}, errors.Join(ErrUpstream, err)
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

// Cancel 取消属于当前会话的二维码并递增代次，使在途轮询不能继续授权。
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

// Remove 清除临时扫码状态并删除已保存账号，由仓储记录删除审计。
func (s *Service) Remove(ctx context.Context, owner, actor, requestID string) (resultErr error) {
	start := time.Now()
	defer func() { s.log(ctx, "remove_account", requestID, start, resultErr) }()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearLocked()
	s.generation++
	return s.repo.DeleteProviderAccount(ctx, actor, requestID, s.now().UTC())
}

// clearLocked 释放临时二维码及过期计时器；调用者必须持有 s.mu。
func (s *Service) clearLocked() {
	if s.expiryTimer != nil {
		s.expiryTimer.Stop()
		s.expiryTimer = nil
	}
	s.active = nil
}

// InvalidateSession 丢弃已退出会话的扫码状态，并使该会话尚未完成的创建请求失效。
func (s *Service) InvalidateSession(owner string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.startingOwner == owner || (s.active != nil && s.active.owner == owner) {
		s.generation++
		s.startingOwner = ""
		s.clearLocked()
	}
}

// request 将二维码协议交给受控传输，保持扫码不占账号配额、登录交换单次发送及取消身份。
func (s *Service) request(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	var data []byte
	if body != nil {
		var err error
		data, err = io.ReadAll(io.LimitReader(body, (1<<20)+1))
		if err != nil || len(data) > 1<<20 {
			return nil, ErrUpstream
		}
	}
	operation := adapter.QR
	if method == http.MethodPost {
		operation = adapter.Login
	}
	return s.doRequest(ctx, adapter.Request{Operation: operation, Method: method, URL: s.baseURL + path, Body: data, Header: requestHeaders("", "Mozilla/5.0 115Browser/36.0.0"), MaxBytes: 1 << 20})
}

// upstreamState 兼容上游布尔值和 0、1 数字状态编码。
type upstreamState bool

// UnmarshalJSON 只接受 true、false、0、1，拒绝未知状态编码。
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
