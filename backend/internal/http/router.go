// Package http owns HTTP transport, request correlation, and frontend delivery.
package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
)

// Version is set by the build using -ldflags; unversioned local builds are dev.
var Version = "dev"

var processStarted = time.Now()

// Readiness exposes only the database operation needed by the health transport.
type Readiness interface{ Ping(context.Context) error }

// NewRouter constructs isolated Gin routes without global logger/recovery state.
func NewRouter(database Readiness, logger *slog.Logger, assets fs.FS, options ...AuthOptions) (http.Handler, error) {
	if assets != nil {
		if _, err := fs.ReadFile(assets, "index.html"); err != nil {
			return nil, err
		}
	}
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		return nil, err
	}
	router.HandleMethodNotAllowed = true
	router.Use(requestMiddleware(logger))
	var authOptions AuthOptions
	if len(options) > 0 {
		authOptions = options[0]
	}
	authHandler := &authenticationHandler{service: authOptions.Service, provider: authOptions.Provider, secure: authOptions.CookieSecure, logger: logger}
	router.Use(authHandler.managementBoundary())
	if authOptions.Service != nil {
		authHandler.register(router)
		if authOptions.Provider != nil {
			(&providerHandler{service: authOptions.Provider}).register(router)
		}
	}
	router.GET("/api/v1/system/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, generated.SystemStatus{UptimeSeconds: int64(time.Since(processStarted).Seconds()), Version: Version})
	})
	router.GET("/health/live", func(c *gin.Context) { c.JSON(http.StatusOK, generated.HealthResponse{Status: generated.Ok}) })
	router.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := database.Ping(ctx); err != nil {
			logger.WarnContext(ctx, "database readiness failed", "module", "health", "operation", "readiness", "request_id", c.GetString("request_id"), "error_code", "database_unavailable")
			writeError(c, http.StatusServiceUnavailable, generated.ServiceUnavailable, "Service unavailable")
			return
		}
		c.JSON(http.StatusOK, generated.HealthResponse{Status: generated.Ok})
	})
	router.NoMethod(func(c *gin.Context) {
		writeError(c, http.StatusMethodNotAllowed, generated.MethodNotAllowed, "Method not allowed")
	})
	router.NoRoute(func(c *gin.Context) { serveFrontend(c, assets) })
	return router, nil
}

func writeError(c *gin.Context, status int, code generated.APIErrorCode, message string) {
	c.AbortWithStatusJSON(status, generated.ErrorResponse{Error: generated.APIError{Code: code, Message: message, RequestId: c.GetString("request_id")}})
}

func requestMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		requestID := c.GetHeader("X-Request-ID")
		if !validRequestID(requestID) {
			requestID = rand.Text()
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		defer func() {
			if recover() != nil {
				logger.ErrorContext(c.Request.Context(), "request panic", "module", "http", "operation", "request", "request_id", requestID, "error_code", "internal_error")
				if !c.Writer.Written() {
					writeError(c, http.StatusInternalServerError, generated.InternalError, "Internal server error")
				} else {
					c.Abort()
				}
			}
			route := c.FullPath()
			if route == "" {
				route = "unmatched"
			}
			level := slog.LevelInfo
			if strings.HasPrefix(route, "/health/") {
				level = slog.LevelDebug
			}
			logger.Log(c.Request.Context(), level, "http request", "module", "http", "operation", "request", "request_id", requestID, "method", c.Request.Method, "route", route, "status", c.Writer.Status(), "duration_ms", time.Since(started).Milliseconds())
		}()
		c.Next()
	}
}

func validRequestID(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func serveFrontend(c *gin.Context, assets fs.FS) {
	// Normalize before checking namespace ownership so dot segments cannot enter SPA fallback.
	requestPath := path.Clean("/" + strings.TrimPrefix(c.Request.URL.Path, "/"))
	reserved := false
	for _, prefix := range []string{"/api", "/health", "/p", "/assets"} {
		if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
			reserved = true
			break
		}
	}
	if assets == nil || reserved && !strings.HasPrefix(requestPath, "/assets/") || c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		writeError(c, http.StatusNotFound, generated.NotFound, "Not found")
		return
	}
	name := strings.TrimPrefix(path.Clean(requestPath), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	data, err := fs.ReadFile(assets, name)
	if err != nil {
		// Missing assets and file-like URLs must never receive an HTML success response.
		if reserved || path.Ext(name) != "" || strings.HasPrefix(name, ".") {
			writeError(c, http.StatusNotFound, generated.NotFound, "Not found")
			return
		}
		data, err = fs.ReadFile(assets, "index.html")
		name = "index.html"
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, generated.InternalError, "Internal server error")
		return
	}
	http.ServeContent(c.Writer, c.Request, name, time.Time{}, bytes.NewReader(data))
}
