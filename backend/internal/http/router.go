// router.go：组装 HTTP 路由、请求关联与异常恢复，提供健康检查和带命名空间边界的前端页面回退。

// Package http 负责 HTTP 传输、请求关联及前端资源交付。
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

// Version 由构建时的 -ldflags 注入；未指定版本的本地构建使用 dev。
var Version = "dev"

// processStarted 记录进程初始化时刻，供系统状态计算运行时长。
var processStarted = time.Now()

// Readiness 仅暴露就绪检查所需的数据库探测能力，不让 HTTP 层依赖具体存储。
type Readiness interface {
	// Ping 检查数据库是否可用，遵守健康探测上下文的取消和超时。
	Ping(context.Context) error
}

// NewRouter 构造独立 Gin 路由并注入业务服务，关闭代理信任与尾斜线重定向；配置前端时先检查首页存在。
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
	// 播放凭据位于路径中，遇到多余斜线时由路由返回错误，避免自动重定向回显原始路径。
	router.RedirectTrailingSlash = false
	router.Use(requestMiddleware(logger))
	var authOptions AuthOptions
	if len(options) > 0 {
		authOptions = options[0]
	}
	authHandler := &authenticationHandler{service: authOptions.Service, provider: authOptions.Provider, secure: authOptions.CookieSecure, logger: logger}
	router.Use(authHandler.managementBoundary())
	if authOptions.Service != nil {
		authHandler.register(router)
		if authOptions.Library != nil {
			(&libraryHandler{service: authOptions.Library}).register(router)
		}
		if authOptions.Provider != nil {
			(&providerHandler{service: authOptions.Provider}).register(router)
		}
		if authOptions.STRM != nil {
			(&strmHandler{service: authOptions.STRM, libraries: authOptions.Library}).register(router)
		}
		if authOptions.Jobs != nil {
			(&taskHandler{service: authOptions.Jobs, auth: authOptions.Service, logger: logger}).register(router)
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

// writeError 中止当前请求并输出携带请求标识的统一错误响应，消息应由调用方提供脱敏文本。
func writeError(c *gin.Context, status int, code generated.APIErrorCode, message string) {
	c.AbortWithStatusJSON(status, generated.ErrorResponse{Error: generated.APIError{Code: code, Message: message, RequestId: c.GetString("request_id")}})
}

// requestMiddleware 校验或生成请求标识，捕获未处理异常并记录耗时；访问日志仅记录路由模板，避免泄露路径凭据。
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

// validRequestID 限制请求标识为 1 至 64 个字节的字母、数字或点、下划线、短横线，避免原始头部污染日志。
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

// serveFrontend 归一化请求路径后提供静态资源或 SPA 首页；保留 API、健康、播放及缺失资源的错误响应。
func serveFrontend(c *gin.Context, assets fs.FS) {
	// 先归一化再校验命名空间，防止点路径绕过接口边界进入 SPA 回退。
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
		// 缺失资源及类似文件的 URL 必须返回错误，不能用首页伪装成成功响应。
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
