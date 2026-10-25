package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/handler"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/security"
)

func TestAuthLoginSessionAndCSRFProtectedLogout(t *testing.T) {
	auth := security.New("admin", "secret")
	r := NewWithAuth(nil, nil, auth, false)
	login := httptest.NewRecorder()
	r.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"secret"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatalf("unsafe cookie attributes: %#v", cookie)
	}
	if cookie.Value == "" {
		t.Fatal("session cookie missing")
	}
	if login.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("login response must not be cached")
	}

	badLogout := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(cookie)
	r.ServeHTTP(badLogout, req)
	if badLogout.Code != http.StatusForbidden {
		t.Fatalf("logout without CSRF status = %d", badLogout.Code)
	}

	var session struct {
		Authenticated bool   `json:"authenticated"`
		CSRFToken     string `json:"csrfToken"`
	}
	sess := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	req.AddCookie(cookie)
	r.ServeHTTP(sess, req)
	if sess.Code != http.StatusOK {
		t.Fatalf("session status = %d", sess.Code)
	}
	if sess.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("session response must not be cached")
	}
	if err := json.Unmarshal(sess.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if !session.Authenticated || session.CSRFToken == "" {
		t.Fatalf("session response: %+v", session)
	}
	okLogout := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", session.CSRFToken)
	r.ServeHTTP(okLogout, req)
	if okLogout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d", okLogout.Code)
	}
	repeat := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", session.CSRFToken)
	r.ServeHTTP(repeat, req)
	if repeat.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session logout status = %d", repeat.Code)
	}
}

func TestAuthLoginFailureIsGenericAndDoesNotSetCookie(t *testing.T) {
	r := NewWithAuth(nil, nil, security.New("admin", "secret"), false)
	for _, body := range []string{`{"username":"no","password":"secret"}`, `{"username":"admin","password":"bad"}`} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body)))
		if w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), "admin") || len(w.Result().Cookies()) > 0 {
			t.Fatalf("unsafe login failure: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestManagementRoutesRequireSessionAndCSRF(t *testing.T) {
	auth := handler.Auth{Service: security.New("admin", "secret")}
	r := NewWithAuth(nil, nil, auth.Service, false)
	RegisterManagementRoutes(r, auth, func(group *gin.RouterGroup) {
		group.POST("/change", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	})
	unauth := httptest.NewRecorder()
	r.ServeHTTP(unauth, httptest.NewRequest(http.MethodPost, "/api/admin/change", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauth.Code)
	}
	login := httptest.NewRecorder()
	r.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"secret"}`)))
	cookie := login.Result().Cookies()[0]
	var credentials struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil {
		t.Fatal(err)
	}
	missing := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/change", nil)
	req.AddCookie(cookie)
	r.ServeHTTP(missing, req)
	if missing.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status = %d", missing.Code)
	}
	valid := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/admin/change", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", "invalid")
	r.ServeHTTP(valid, req)
	if valid.Code != http.StatusForbidden {
		t.Fatalf("invalid CSRF status = %d", valid.Code)
	}
	accepted := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/admin/change", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", credentials.CSRFToken)
	r.ServeHTTP(accepted, req)
	if accepted.Code != http.StatusNoContent {
		t.Fatalf("valid session status = %d", accepted.Code)
	}
}
