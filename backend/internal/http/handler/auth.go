package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/security"
)

const sessionCookieName = "media_hub_session"

type Auth struct {
	Service      *security.Service
	CookieSecure bool
}

func (h Auth) Login(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h.Service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authentication unavailable"})
		return
	}
	var body generated.LoginRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	id, csrf, expires, err := h.Service.Login(body.Username, body.Password)
	if err != nil {
		if errors.Is(err, security.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authentication failed"})
		return
	}
	maxAge := int(security.SessionLifetime.Seconds())
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, id, maxAge, "/", "", h.CookieSecure, true)
	c.JSON(http.StatusOK, generated.AuthenticatedSession{Authenticated: generated.True, CsrfToken: csrf, ExpiresAt: expires})
}

func (h Auth) Session(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h.Service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authentication unavailable"})
		return
	}
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil {
		c.JSON(http.StatusOK, generated.SessionState{Authenticated: false})
		return
	}
	info, ok := h.Service.Session(cookie)
	if !ok {
		h.clearCookie(c)
		c.JSON(http.StatusOK, generated.SessionState{Authenticated: false})
		return
	}
	c.JSON(http.StatusOK, generated.SessionState{Authenticated: true, CsrfToken: &info.CSRFToken, ExpiresAt: &info.ExpiresAt})
}

func (h Auth) Logout(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h.Service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authentication unavailable"})
		return
	}
	id, err := c.Cookie(sessionCookieName)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	sessionValid, csrfValid := h.Service.CheckCSRF(id, c.GetHeader("X-CSRF-Token"))
	if !sessionValid {
		h.clearCookie(c)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !csrfValid {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid csrf token"})
		return
	}
	h.Service.Logout(id)
	h.clearCookie(c)
	c.Status(http.StatusNoContent)
}

func (h Auth) clearCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, "", -1, "/", "", h.CookieSecure, true)
}

func (h Auth) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := c.Cookie(sessionCookieName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		mutating := c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions
		if mutating {
			sessionValid, csrfValid := h.Service.CheckCSRF(id, c.GetHeader("X-CSRF-Token"))
			if !sessionValid {
				h.clearCookie(c)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				return
			}
			if !csrfValid {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid csrf token"})
				return
			}
		} else if _, ok := h.Service.Session(id); !ok {
			h.clearCookie(c)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
