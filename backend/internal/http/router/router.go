package router

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/handler"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/security"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/connections"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/webui"
)

func New(ping func(context.Context) error, files fs.FS) *gin.Engine {
	return newRouter(ping, files, nil, false, nil)
}

func NewWithAuth(ping func(context.Context) error, files fs.FS, service *security.Service, cookieSecure bool) *gin.Engine {
	return newRouter(ping, files, service, cookieSecure, nil)
}

func NewWithConnections(ping func(context.Context) error, files fs.FS, authService *security.Service, cookieSecure bool, connectionService *connections.Service) *gin.Engine {
	return newRouter(ping, files, authService, cookieSecure, connectionService)
}

func RegisterManagementRoutes(r *gin.Engine, auth handler.Auth, register func(*gin.RouterGroup)) {
	group := r.Group("/api/admin")
	group.Use(auth.Middleware())
	register(group)
}

func newRouter(ping func(context.Context) error, files fs.FS, service *security.Service, cookieSecure bool, connectionService *connections.Service) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("http request", "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
	}, func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				slog.Error("http handler panic")
				c.AbortWithStatusJSON(500, gin.H{"error": "internal server error"})
			}
		}()
		c.Next()
	})
	auth := handler.Auth{Service: service, CookieSecure: cookieSecure}
	// Generated route registration is flat, so protect the management prefix before
	// registering it. Auth middleware also enforces CSRF on every mutation.
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/admin/") || c.Request.URL.Path == "/api/admin" {
			auth.Middleware()(c)
			return
		}
		c.Next()
	})
	generated.RegisterHandlers(r, apiHandler{Health: handler.Health{Ping: ping}, Auth: auth, Connections: handler.Connections{Service: connectionService}})
	ui := webui.Handler(files)
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		for _, prefix := range []string{"/api", "/p", "/health"} {
			if p == prefix || strings.HasPrefix(p, prefix+"/") {
				c.JSON(404, gin.H{"error": "not found"})
				return
			}
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		ui.ServeHTTP(c.Writer, c.Request)
	})
	return r
}

type apiHandler struct {
	handler.Health
	handler.Auth
	handler.Connections
}

func (h apiHandler) LoginAdmin(c *gin.Context)      { h.Auth.Login(c) }
func (h apiHandler) GetAdminSession(c *gin.Context) { h.Auth.Session(c) }
func (h apiHandler) LogoutAdmin(c *gin.Context)     { h.Auth.Logout(c) }

func (h apiHandler) Get115Connection(c *gin.Context)               { h.Connections.Get(c) }
func (h apiHandler) Create115QRCodeSession(c *gin.Context)         { h.Connections.Start(c) }
func (h apiHandler) Get115QRCodeSession(c *gin.Context, id string) { h.Connections.Poll(c, id) }
func (h apiHandler) Get115QRCodeImage(c *gin.Context, id string)   { h.Connections.QR(c, id) }
func (h apiHandler) Complete115QRCodeSession(c *gin.Context, id string) {
	h.Connections.Complete(c, id)
}
func (h apiHandler) Test115Connection(c *gin.Context)        { h.Connections.Test(c) }
func (h apiHandler) Refresh115AccountProfile(c *gin.Context) { h.Connections.RefreshProfile(c) }

var _ generated.ServerInterface = apiHandler{}
