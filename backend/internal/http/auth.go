package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

const sessionCookie = "mediahub_session"
const anonymousCookie = "mediahub_preauth"

// AuthOptions injects authentication policy and the explicit reverse-proxy HTTPS cookie setting.
type AuthOptions struct {
	Service      *auth.Service
	CookieSecure bool
	Provider     *provider115.Service
}

type authenticationHandler struct {
	service  *auth.Service
	provider *provider115.Service
	secure   bool
	logger   *slog.Logger
}

func (h *authenticationHandler) register(router *gin.Engine) {
	router.GET("/api/v1/auth/bootstrap", h.bootstrap)
	router.POST("/api/v1/auth/initialize", h.initialize)
	router.POST("/api/v1/auth/login", h.login)
	router.POST("/api/v1/auth/logout", h.logout)
	router.GET("/api/v1/settings/security", h.security)
	router.PUT("/api/v1/settings/security/credentials", h.credentials)
	router.PUT("/api/v1/settings/security/session", h.sessionSettings)
}

// Registered management routes are protected by default; unknown namespaces retain their 404 contract.
func (h *authenticationHandler) managementBoundary() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.FullPath() == "" || !strings.HasPrefix(c.FullPath(), "/api/v1/") {
			c.Next()
			return
		}
		c.Header("Cache-Control", "no-store")
		public := c.Request.Method == http.MethodGet && c.FullPath() == "/api/v1/auth/bootstrap" || c.Request.Method == http.MethodPost && (c.FullPath() == "/api/v1/auth/initialize" || c.FullPath() == "/api/v1/auth/login")
		if h.service == nil {
			writeError(c, http.StatusUnauthorized, generated.AuthenticationRequired, "请先登录")
			return
		}
		token, _ := c.Cookie(sessionCookie)
		started := time.Now()
		identity, err := h.service.Authenticate(c.Request.Context(), token)
		if err != nil && !errors.Is(err, auth.ErrUnauthenticated) {
			h.handleStateFailure(c, "authenticate", started, err)
			return
		}
		if !public && err != nil {
			h.expireCookie(c, sessionCookie)
			h.writeFailure(c, auth.ErrUnauthenticated)
			return
		}
		if err == nil {
			c.Set("auth_identity", identity)
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
			proof := c.GetHeader("X-CSRF-Token")
			valid := err == nil && h.service.ValidateSessionCSRF(identity, token, proof)
			if public && !valid {
				cookie, _ := c.Cookie(anonymousCookie)
				valid = h.service.ValidateAnonymous(cookie, proof)
			}
			if !valid || !h.sameOrigin(c.Request) {
				h.logger.WarnContext(c.Request.Context(), "authentication request rejected", "module", "auth", "operation", "csrf", "outcome", "failed", "error_code", "csrf_invalid", "request_id", c.GetString("request_id"))
				h.writeFailure(c, auth.ErrCSRF)
				return
			}
		}
		c.Next()
	}
}

func (h *authenticationHandler) sameOrigin(request *http.Request) bool {
	if request.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	} // Non-browser clients still require the cookie-bound CSRF proof.
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.Contains(origin, "#") {
		return false
	}
	scheme := "http"
	if h.secure || request.TLS != nil {
		scheme = "https"
	}
	return parsed.Scheme == scheme && strings.EqualFold(parsed.Host, request.Host)
}

func (h *authenticationHandler) bootstrap(c *gin.Context) {
	started := time.Now()
	initialized, err := h.service.Initialized(c.Request.Context())
	if err != nil {
		h.handleStateFailure(c, "bootstrap", started, err)
		return
	}
	response := generated.AuthBootstrap{Initialized: initialized}
	if value, ok := c.Get("auth_identity"); ok {
		identity := value.(domain.Identity)
		response.Authenticated = true
		response.Administrator = &generated.Administrator{Id: identity.Administrator.ID, Username: identity.Administrator.Username}
		response.ExpiresAt = &identity.Session.ExpiresAt
		token, _ := c.Cookie(sessionCookie)
		response.CsrfToken = h.service.CSRFForSession(token)
	} else {
		if token, _ := c.Cookie(sessionCookie); token != "" {
			h.expireCookie(c, sessionCookie)
		}
		cookie, _ := c.Cookie(anonymousCookie)
		csrf, valid := h.service.AnonymousCSRF(cookie)
		if !valid {
			cookie, csrf = h.service.AnonymousBinding()
			h.setCookie(c, &http.Cookie{Name: anonymousCookie, Value: cookie, MaxAge: 1200, Expires: time.Now().Add(20 * time.Minute)})
		}
		response.CsrfToken = csrf
	}
	c.JSON(http.StatusOK, response)
}

func (h *authenticationHandler) initialize(c *gin.Context) {
	var body generated.InitializeRequest
	if !decodeAuthBody(c, &body, []string{"username", "password", "service_url", "output_directory", "session_lifetime_days"}) {
		return
	}
	result, err := h.service.Initialize(c.Request.Context(), body.Username, body.Password, domain.Settings{ServiceURL: body.ServiceUrl, OutputDirectory: body.OutputDirectory, SessionLifetimeDays: int(body.SessionLifetimeDays)}, trustedPeer(c.Request), c.GetString("request_id"))
	if err != nil {
		h.writeFailure(c, err)
		return
	}
	h.startSession(c, result, true, http.StatusCreated)
}

func (h *authenticationHandler) login(c *gin.Context) {
	var body generated.LoginRequest
	if !decodeAuthBody(c, &body, []string{"username", "password", "remember_login"}) {
		return
	}
	oldToken, _ := c.Cookie(sessionCookie)
	result, err := h.service.Login(c.Request.Context(), body.Username, body.Password, oldToken, trustedPeer(c.Request), c.GetString("request_id"))
	if err != nil {
		h.writeFailure(c, err)
		return
	}
	h.startSession(c, result, body.RememberLogin, http.StatusOK)
}

func (h *authenticationHandler) logout(c *gin.Context) {
	value, _ := c.Get("auth_identity")
	if err := h.service.Logout(c.Request.Context(), value.(domain.Identity), c.GetString("request_id")); err != nil {
		h.writeFailure(c, err)
		return
	}
	if h.provider != nil {
		h.provider.InvalidateSession(value.(domain.Identity).Session.TokenHash)
	}
	h.expireCookie(c, sessionCookie)
	h.expireCookie(c, anonymousCookie)
	c.Status(http.StatusNoContent)
}

func (h *authenticationHandler) startSession(c *gin.Context, result auth.SessionResult, remember bool, status int) {
	cookie := &http.Cookie{Name: sessionCookie, Value: result.Token}
	if remember {
		cookie.Expires = result.Identity.Session.ExpiresAt
		cookie.MaxAge = max(1, int(math.Ceil(time.Until(cookie.Expires).Seconds())))
	}
	h.setCookie(c, cookie)
	h.expireCookie(c, anonymousCookie)
	c.JSON(status, generated.AuthSession{Administrator: generated.Administrator{Id: result.Identity.Administrator.ID, Username: result.Identity.Administrator.Username}, ExpiresAt: result.Identity.Session.ExpiresAt, CsrfToken: result.CSRFToken})
}

func (h *authenticationHandler) setCookie(c *gin.Context, cookie *http.Cookie) {
	cookie.HttpOnly = true
	cookie.Secure = h.secure || c.Request.TLS != nil
	cookie.SameSite = http.SameSiteLaxMode
	cookie.Path = "/"
	http.SetCookie(c.Writer, cookie)
}
func (h *authenticationHandler) expireCookie(c *gin.Context, name string) {
	h.setCookie(c, &http.Cookie{Name: name, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func trustedPeer(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return "unknown"
	}
	return host
}

func decodeAuthBody(c *gin.Context, target any, required []string) bool {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "请求必须使用 JSON 格式")
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	if err != nil {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "请求内容无效或超过 4096 字节")
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "请求内容无效")
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "请求内容无效")
		return false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "请求内容无效")
		return false
	}
	for _, key := range required {
		if raw, exists := fields[key]; !exists || string(raw) == "null" {
			writeError(c, http.StatusBadRequest, generated.ValidationFailed, "请填写所有必填字段")
			return false
		}
	}
	return true
}

func (h *authenticationHandler) writeFailure(c *gin.Context, err error) {
	var validation *auth.ValidationError
	var throttled *auth.ThrottledError
	switch {
	case errors.As(err, &validation):
		c.AbortWithStatusJSON(http.StatusBadRequest, generated.ErrorResponse{Error: generated.APIError{Code: generated.ValidationFailed, Message: "请检查输入内容", RequestId: c.GetString("request_id"), Fields: &validation.Fields}})
	case errors.As(err, &throttled):
		c.Header("Retry-After", strconv.Itoa(max(1, int(math.Ceil(throttled.RetryAfter.Seconds())))))
		writeError(c, http.StatusTooManyRequests, generated.RateLimited, "尝试次数过多，请稍后重试")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(c, http.StatusUnauthorized, generated.InvalidCredentials, "用户名或密码不正确")
	case errors.Is(err, auth.ErrUnauthenticated):
		writeError(c, http.StatusUnauthorized, generated.AuthenticationRequired, "登录已过期或失效，请重新登录")
	case errors.Is(err, auth.ErrCSRF):
		writeError(c, http.StatusForbidden, generated.CsrfInvalid, "安全验证失败，请刷新页面后重试")
	case errors.Is(err, auth.ErrSetupRequired):
		writeError(c, http.StatusConflict, generated.SetupRequired, "请先完成系统初始化")
	case errors.Is(err, auth.ErrAlreadyInitialized):
		writeError(c, http.StatusConflict, generated.AlreadyInitialized, "系统已完成初始化，请登录")
	default:
		writeError(c, http.StatusServiceUnavailable, generated.ServiceUnavailable, "服务暂时不可用，请稍后重试")
	}
}

// State reads have no mutation-service completion record, so transport owns their
// failure diagnostic. Never emit raw SQL/driver error text or request payloads.
func (h *authenticationHandler) handleStateFailure(c *gin.Context, operation string, started time.Time, err error) {
	if errors.Is(err, context.Canceled) && c.Request.Context().Err() != nil {
		// An aborted browser bootstrap is normal during navigation/StrictMode.
		// The client is gone; do not invent a service-unavailable response.
		h.logger.DebugContext(c.Request.Context(), "authentication state request canceled", "module", "auth", "operation", operation, "outcome", "canceled", "request_id", c.GetString("request_id"), "duration_ms", time.Since(started).Milliseconds())
		c.Abort()
		return
	}
	attributes := []any{"module", "auth", "operation", operation, "outcome", "failed", "error_code", "authentication_unavailable", "request_id", c.GetString("request_id"), "duration_ms", time.Since(started).Milliseconds()}
	var storage *auth.StorageError
	if errors.As(err, &storage) {
		attributes = append(attributes, "storage_operation", storage.Operation, "storage_reason", storage.Reason)
	}
	h.logger.ErrorContext(c.Request.Context(), "authentication state unavailable", attributes...)
	h.writeFailure(c, err)
}
