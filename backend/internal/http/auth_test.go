// auth_test.go：验证认证 HTTP 全流程、默认管理边界、CSRF 与 Cookie 策略，以及状态读取取消和日志脱敏回归。

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
)

// authBrowser 在测试内维护 Cookie、CSRF 与连接对端，模拟浏览器会话而不依赖真实浏览器。
type authBrowser struct {
	handler http.Handler
	cookies map[string]*http.Cookie
	csrf    string
	peer    string
}

// newAuthBrowser 建立临时 SQLite 与可控认证服务，返回模拟浏览器；数据库由测试清理关闭。
func newAuthBrowser(t *testing.T, secure bool, now func() time.Time) (*authBrowser, *auth.Service, *sqlite.Database) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	service, err := auth.New(db, logger, auth.Options{PasswordCost: bcrypt.MinCost, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewRouter(db, logger, nil, AuthOptions{Service: service, CookieSecure: secure})
	if err != nil {
		t.Fatal(err)
	}
	return &authBrowser{handler: handler, cookies: map[string]*http.Cookie{}, peer: "192.0.2.1:1234"}, service, db
}

// request 模拟携带 Cookie 与 CSRF 的请求，应用自定义请求头并保存响应中的 Cookie 变化。
func (b *authBrowser) request(t *testing.T, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://media.test"+path, strings.NewReader(body))
	request.RemoteAddr = b.peer
	for _, cookie := range b.cookies {
		request.AddCookie(cookie)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if b.csrf != "" {
		request.Header.Set("X-CSRF-Token", b.csrf)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	b.handler.ServeHTTP(response, request)
	for _, cookie := range response.Result().Cookies() {
		if cookie.MaxAge < 0 {
			delete(b.cookies, cookie.Name)
		} else {
			b.cookies[cookie.Name] = cookie
		}
	}
	return response
}

// bootstrap 读取引导状态并更新浏览器持有的 CSRF 证明。
func (b *authBrowser) bootstrap(t *testing.T) generated.AuthBootstrap {
	t.Helper()
	response := b.request(t, "GET", "/api/v1/auth/bootstrap", "", nil)
	if response.Code != 200 {
		t.Fatalf("bootstrap %d: %s", response.Code, response.Body)
	}
	var state generated.AuthBootstrap
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	b.csrf = state.CsrfToken
	return state
}

const setupJSON = `{"username":"admin","password":"ValidPass123!","service_url":"https://media.example.com","output_directory":"/app/media","session_lifetime_days":7}`
const loginJSON = `{"username":"admin","password":"ValidPass123!","remember_login":false}`

// setup 完成引导及首次管理员初始化，保存新会话的 CSRF 证明。
func (b *authBrowser) setup(t *testing.T) generated.AuthSession {
	t.Helper()
	b.bootstrap(t)
	response := b.request(t, "POST", "/api/v1/auth/initialize", setupJSON, nil)
	if response.Code != 201 {
		t.Fatalf("initialize %d: %s", response.Code, response.Body)
	}
	var session generated.AuthSession
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	b.csrf = session.CsrfToken
	return session
}

// assertAuthError 检查错误状态、稳定错误码和非空请求关联标识。
func assertAuthError(t *testing.T, response *httptest.ResponseRecorder, status int, code generated.APIErrorCode) {
	t.Helper()
	var body generated.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != status || body.Error.Code != code || body.Error.RequestId == "" {
		t.Fatalf("error %d %+v; want %d %s", response.Code, body, status, code)
	}
}

// TestAuthHTTPFlowCSRFAndCookieRotation 验证初始化、重复登录和注销中的同源 CSRF、Cookie 安全属性及会话轮换。
func TestAuthHTTPFlowCSRFAndCookieRotation(t *testing.T) {
	browser, service, _ := newAuthBrowser(t, false, nil)
	state := browser.bootstrap(t)
	if state.Initialized || state.Authenticated || state.Administrator != nil || state.ExpiresAt != nil || state.CsrfToken == "" {
		t.Fatalf("anonymous bootstrap exposed state: %+v", state)
	}
	anonymous := browser.cookies[anonymousCookie]
	if anonymous == nil || !anonymous.HttpOnly || anonymous.Secure || anonymous.SameSite != http.SameSiteLaxMode || anonymous.Path != "/" {
		t.Fatalf("preauth flags: %+v", anonymous)
	}
	for _, headers := range []map[string]string{{"X-CSRF-Token": ""}, {"X-CSRF-Token": "bad"}, {"Origin": "https://attacker.test"}, {"Sec-Fetch-Site": "cross-site"}, {"Origin": "https://attacker.test", "X-Forwarded-Host": "attacker.test", "X-Forwarded-Proto": "https"}} {
		assertAuthError(t, browser.request(t, "POST", "/api/v1/auth/initialize", setupJSON, headers), 403, generated.CsrfInvalid)
	}
	oldCSRF := browser.csrf
	session := browser.setup(t)
	cookie := browser.cookies[sessionCookie]
	if cookie == nil || !cookie.HttpOnly || cookie.Secure || cookie.MaxAge <= 0 || cookie.Expires.IsZero() || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session flags: %+v", cookie)
	}
	if session.CsrfToken == oldCSRF || browser.cookies[anonymousCookie] != nil {
		t.Fatal("setup failed to rotate pre-auth binding")
	}
	state = browser.bootstrap(t)
	if !state.Initialized || !state.Authenticated || state.Administrator == nil || state.Administrator.Username != "admin" || state.ExpiresAt == nil {
		t.Fatalf("authenticated bootstrap: %+v", state)
	}
	firstCookie := cookie.Value
	assertAuthError(t, browser.request(t, "POST", "/api/v1/auth/logout", "", map[string]string{"X-CSRF-Token": oldCSRF}), 403, generated.CsrfInvalid)
	// 再次登录应撤销旧会话，并按未记住登录的选项签发浏览器会话 Cookie。
	response := browser.request(t, "POST", "/api/v1/auth/login", loginJSON, map[string]string{"Origin": "http://media.test"})
	if response.Code != 200 {
		t.Fatalf("login %d: %s", response.Code, response.Body)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("credential response cacheable")
	}
	cookie = browser.cookies[sessionCookie]
	if cookie.Value == firstCookie || cookie.MaxAge != 0 || !cookie.Expires.IsZero() {
		t.Fatalf("session-cookie persistence or rotation: %+v", cookie)
	}
	if _, err := service.Authenticate(t.Context(), firstCookie); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("old session not rotated: %v", err)
	}
	browser.bootstrap(t)
	response = browser.request(t, "POST", "/api/v1/auth/logout", "", nil)
	if response.Code != 204 || browser.cookies[sessionCookie] != nil {
		t.Fatalf("logout %d %s", response.Code, response.Body)
	}
	assertAuthError(t, browser.request(t, "POST", "/api/v1/auth/logout", "", nil), 401, generated.AuthenticationRequired)
	state = browser.bootstrap(t)
	if !state.Initialized || state.Authenticated || state.Administrator != nil {
		t.Fatalf("revoked bootstrap: %+v", state)
	}
}

// TestManagementRoutesDefaultProtectedAndUnknownAPI404 验证新增管理路由默认要求认证与变更 CSRF，健康和未知 API 保持各自合同。
func TestManagementRoutesDefaultProtectedAndUnknownAPI404(t *testing.T) {
	browser, _, _ := newAuthBrowser(t, false, nil)
	browser.handler.(*gin.Engine).GET("/api/v1/future", func(c *gin.Context) { c.JSON(200, gin.H{"protected": true}) })
	browser.handler.(*gin.Engine).POST("/api/v1/future", func(c *gin.Context) { c.Status(204) })
	assertAuthError(t, browser.request(t, "GET", "/api/v1/future", "", nil), 401, generated.AuthenticationRequired)
	assertAuthError(t, browser.request(t, "GET", "/api/v1/missing", "", nil), 404, generated.NotFound)
	if response := browser.request(t, "GET", "/health/live", "", nil); response.Code != 200 {
		t.Fatal("health incorrectly protected")
	}
	browser.setup(t)
	if response := browser.request(t, "GET", "/api/v1/future", "", nil); response.Code != 200 {
		t.Fatal("valid auth denied")
	}
	assertAuthError(t, browser.request(t, "POST", "/api/v1/future", "", map[string]string{"X-CSRF-Token": ""}), 403, generated.CsrfInvalid)
	if response := browser.request(t, "POST", "/api/v1/future", "", nil); response.Code != 204 {
		t.Fatal("valid management csrf denied")
	}
}

// TestAuthValidationBoundsAndCredentialErrors 验证请求大小、必填和未知字段，以及初始化状态和错误凭据的安全错误映射。
func TestAuthValidationBoundsAndCredentialErrors(t *testing.T) {
	browser, _, _ := newAuthBrowser(t, false, nil)
	browser.bootstrap(t)
	assertAuthError(t, browser.request(t, "POST", "/api/v1/auth/login", loginJSON, nil), 409, generated.SetupRequired)
	for _, body := range []string{"{}", "null", setupJSON + " {}", strings.Replace(setupJSON, `"username":"admin"`, `"username":null`, 1), strings.Replace(setupJSON, `"username":"admin"`, `"unknown":true,"username":"admin"`, 1), strings.Replace(setupJSON, "https://media.example.com", "https://media.example.com/prefix", 1), strings.Replace(setupJSON, "/app/media", "relative", 1), strings.Replace(setupJSON, "ValidPass123!", strings.Repeat("s", 5000), 1)} {
		assertAuthError(t, browser.request(t, "POST", "/api/v1/auth/initialize", body, nil), 400, generated.ValidationFailed)
	}
	browser.setup(t)
	assertAuthError(t, browser.request(t, "POST", "/api/v1/auth/initialize", setupJSON, nil), 409, generated.AlreadyInitialized)
	for _, body := range []string{strings.Replace(loginJSON, "ValidPass123!", "WrongPass123!", 1), strings.Replace(loginJSON, "admin", "other", 1)} {
		response := browser.request(t, "POST", "/api/v1/auth/login", body, nil)
		assertAuthError(t, response, 401, generated.InvalidCredentials)
		if !strings.Contains(response.Body.String(), "用户名或密码不正确") {
			t.Fatal("unsafe credential error")
		}
	}
	assertAuthError(t, browser.request(t, "POST", "/api/v1/auth/login", `{"username":"admin","password":"ValidPass123!"}`, nil), 400, generated.ValidationFailed)
}

// TestSecureCookieExpiryAndUntrustedPeerHeaders 验证显式 HTTPS Cookie、Origin 校验与会话过期，伪造转发来源不能绕过登录限流。
func TestSecureCookieExpiryAndUntrustedPeerHeaders(t *testing.T) {
	now := time.Now().UTC()
	browser, _, _ := newAuthBrowser(t, true, func() time.Time { return now })
	browser.setup(t)
	if !browser.cookies[sessionCookie].Secure {
		t.Fatal("configured reverse-proxy cookie not Secure")
	}
	assertAuthError(t, browser.request(t, "POST", "/api/v1/auth/login", loginJSON, map[string]string{"Origin": "http://media.test"}), 403, generated.CsrfInvalid)
	if response := browser.request(t, "POST", "/api/v1/auth/login", loginJSON, map[string]string{"Origin": "https://media.test"}); response.Code != 200 {
		t.Fatalf("external HTTPS rejected: %d %s", response.Code, response.Body)
	}
	browser.bootstrap(t)
	for i := 0; i < 3; i++ {
		browser.request(t, "POST", "/api/v1/auth/login", strings.Replace(loginJSON, "ValidPass123!", "WrongPass123!", 1), map[string]string{"X-Forwarded-For": string(rune('a' + i))})
	}
	response := browser.request(t, "POST", "/api/v1/auth/login", loginJSON, map[string]string{"X-Forwarded-For": "another-ip"})
	assertAuthError(t, response, 429, generated.RateLimited)
	if response.Header().Get("Retry-After") == "" {
		t.Fatal("throttle missing Retry-After")
	}
	now = now.Add(7 * 24 * time.Hour)
	assertAuthError(t, browser.request(t, "POST", "/api/v1/auth/logout", "", nil), 401, generated.AuthenticationRequired)
	state := browser.bootstrap(t)
	if state.Authenticated || state.Administrator != nil || state.ExpiresAt != nil {
		t.Fatal("expired session disclosed identity")
	}
}

// TestAuthHTTPSecretRedaction 验证 HTTP 和业务日志不包含密码、会话 Cookie 或 CSRF 令牌。
func TestAuthHTTPSecretRedaction(t *testing.T) {
	browser, _, db := newAuthBrowser(t, false, nil)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	service, err := auth.New(db, logger, auth.Options{PasswordCost: bcrypt.MinCost})
	if err != nil {
		t.Fatal(err)
	}
	browser.handler, err = NewRouter(db, logger, nil, AuthOptions{Service: service})
	if err != nil {
		t.Fatal(err)
	}
	result := browser.setup(t)
	browser.request(t, "POST", "/api/v1/auth/login", strings.Replace(loginJSON, "ValidPass123!", "WrongPass123!", 1), nil)
	for _, secret := range []string{"ValidPass123!", "WrongPass123!", browser.cookies[sessionCookie].Value, result.CsrfToken} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("credential leaked in transport/service logs")
		}
	}
}

// TestDirectTLSAlwaysSetsSecureCookie 验证直接 TLS 请求在未显式配置时也启用 Secure Cookie。
func TestDirectTLSAlwaysSetsSecureCookie(t *testing.T) {
	browser, _, _ := newAuthBrowser(t, false, nil)
	request := httptest.NewRequest("GET", "https://media.test/api/v1/auth/bootstrap", nil)
	response := httptest.NewRecorder()
	browser.handler.ServeHTTP(response, request)
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("direct TLS cookie lacks Secure: %+v", cookies)
	}
}

// TestAnonymousBootstrapRefetchReusesBinding 验证重复引导复用有效匿名绑定，旧证明仍能完成首次初始化。
func TestAnonymousBootstrapRefetchReusesBinding(t *testing.T) {
	browser, _, _ := newAuthBrowser(t, false, nil)
	first := browser.bootstrap(t)
	cookie := browser.cookies[anonymousCookie].Value
	second := browser.bootstrap(t)
	if second.CsrfToken != first.CsrfToken || browser.cookies[anonymousCookie].Value != cookie {
		t.Fatal("refetch unexpectedly rotated valid pre-auth binding")
	}
	if response := browser.request(t, "POST", "/api/v1/auth/initialize", setupJSON, nil); response.Code != 201 {
		t.Fatalf("reused pre-auth proof rejected: %d %s", response.Code, response.Body)
	}
}

// TestBootstrapStorageFailureHasRedactedOperationalDiagnostic 验证引导存储故障输出可关联诊断，且不泄露原始驱动错误。
func TestBootstrapStorageFailureHasRedactedOperationalDiagnostic(t *testing.T) {
	browser, _, db := newAuthBrowser(t, false, nil)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	service, err := auth.New(db, logger, auth.Options{PasswordCost: bcrypt.MinCost})
	if err != nil {
		t.Fatal(err)
	}
	browser.handler, err = NewRouter(db, logger, nil, AuthOptions{Service: service})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	response := browser.request(t, "GET", "/api/v1/auth/bootstrap", "", map[string]string{"X-Request-ID": "bootstrap-storage-failure"})
	assertAuthError(t, response, 503, generated.ServiceUnavailable)
	decoder := json.NewDecoder(&logs)
	var outcome map[string]any
	if err := decoder.Decode(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome["module"] != "auth" || outcome["operation"] != "bootstrap" || outcome["storage_operation"] != "load_administrator" || outcome["storage_reason"] != "database_unavailable" || outcome["request_id"] != "bootstrap-storage-failure" || outcome["level"] != "ERROR" {
		t.Fatalf("missing safe storage diagnostic: %+v", outcome)
	}
	if strings.Contains(logs.String()+response.Body.String(), "sql:") {
		t.Fatal("raw driver error reached log or client")
	}
}

// TestCanceledBootstrapDoesNotFabricateServiceFailure 验证已取消引导不向离线客户端虚构 503，日志归类为取消而非服务故障。
func TestCanceledBootstrapDoesNotFabricateServiceFailure(t *testing.T) {
	browser, _, db := newAuthBrowser(t, false, nil)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	service, err := auth.New(db, logger, auth.Options{PasswordCost: bcrypt.MinCost})
	if err != nil {
		t.Fatal(err)
	}
	browser.handler, err = NewRouter(db, logger, nil, AuthOptions{Service: service})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	request := httptest.NewRequest("GET", "http://media.test/api/v1/auth/bootstrap", nil).WithContext(ctx)
	request.Header.Set("X-Request-ID", "canceled-bootstrap")
	response := httptest.NewRecorder()
	browser.handler.ServeHTTP(response, request)
	if response.Code == 503 || response.Body.Len() != 0 {
		t.Fatalf("disconnected client received fabricated error: %d %s", response.Code, response.Body)
	}
	if strings.Contains(logs.String(), `"level":"ERROR"`) || !strings.Contains(logs.String(), `"outcome":"canceled"`) || !strings.Contains(logs.String(), `"request_id":"canceled-bootstrap"`) {
		t.Fatalf("cancellation misclassified as storage failure: %s", logs.String())
	}
}

// authJSONWithPassword 以 JSON 编解码替换测试密码，确保标点和控制字符按请求格式正确转义。
func authJSONWithPassword(t *testing.T, template, password string) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(template), &body); err != nil {
		t.Fatal(err)
	}
	body["password"] = password
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// TestHTTPPasswordPolicyRejectsInvalidSetupAndLogin 验证初始化和登录拒绝超长、非 ASCII、空格及控制字符密码，非法初始化不创建管理员。
func TestHTTPPasswordPolicyRejectsInvalidSetupAndLogin(t *testing.T) {
	for _, password := range []string{strings.Repeat("A", 33), "密码Password123", "Valid Pass123!", " ValidPass123!", "ValidPass123! ", "ValidPass123!\n", "ValidPass123!\t", "ValidPass123!\x7f"} {
		t.Run(password, func(t *testing.T) {
			browser, _, _ := newAuthBrowser(t, false, nil)
			browser.bootstrap(t)
			response := browser.request(t, "POST", "/api/v1/auth/initialize", authJSONWithPassword(t, setupJSON, password), nil)
			assertAuthError(t, response, 400, generated.ValidationFailed)
			var errorBody generated.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &errorBody); err != nil {
				t.Fatal(err)
			}
			if errorBody.Error.Fields == nil || (*errorBody.Error.Fields)["password"] == "" {
				t.Fatal("missing password field guidance")
			}
			if browser.bootstrap(t).Initialized {
				t.Fatal("invalid setup created administrator")
			}
			browser.setup(t)
			response = browser.request(t, "POST", "/api/v1/auth/login", authJSONWithPassword(t, loginJSON, password), nil)
			assertAuthError(t, response, 400, generated.ValidationFailed)
		})
	}
}

// TestHTTPPasswordPolicyAcceptsMaximumLengthAndASCIIPunctuation 验证最大长度和 ASCII 标点密码可用于初始化及登录。
func TestHTTPPasswordPolicyAcceptsMaximumLengthAndASCIIPunctuation(t *testing.T) {
	for _, password := range []string{strings.Repeat("A", 32), "!@#$%^&*()_+-=[]{};:,.<>/?\\|`~\"'"} {
		t.Run(password, func(t *testing.T) {
			browser, _, _ := newAuthBrowser(t, false, nil)
			browser.bootstrap(t)
			response := browser.request(t, "POST", "/api/v1/auth/initialize", authJSONWithPassword(t, setupJSON, password), nil)
			if response.Code != 201 {
				t.Fatalf("valid ASCII setup rejected: %d %s", response.Code, response.Body)
			}
			browser.bootstrap(t)
			response = browser.request(t, "POST", "/api/v1/auth/login", authJSONWithPassword(t, loginJSON, password), nil)
			if response.Code != 200 {
				t.Fatalf("valid ASCII login rejected: %d %s", response.Code, response.Body)
			}
		})
	}
}

// TestSystemStatusRequiresSession 验证系统状态须已登录，且响应版本、运行时长和禁用缓存字段符合合同。
func TestSystemStatusRequiresSession(t *testing.T) {
	browser, _, _ := newAuthBrowser(t, false, time.Now)
	anonymous := browser.request(t, "GET", "/api/v1/system/status", "", nil)
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status %d: %s", anonymous.Code, anonymous.Body)
	}
	browser.setup(t)
	response := browser.request(t, "GET", "/api/v1/system/status", "", nil)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var status generated.SystemStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Version != Version || status.UptimeSeconds < 0 {
		t.Fatalf("invalid status: %+v", status)
	}
}
