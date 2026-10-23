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
	"github.com/wenzhengcheng/115-media-hub/backend/internal/webui"
)

func New(ping func(context.Context) error, files fs.FS) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// No proxy is trusted until deployment explicitly introduces one.
	_ = r.SetTrustedProxies(nil)
	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		// Do not log raw URLs, query parameters, bodies, cookies, or credentials.
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
	generated.RegisterHandlers(r, handler.Health{Ping: ping})
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
