// app.go：构建数据库、业务服务和 HTTP 依赖，管理任务中心与服务器的启动、取消及有界关闭。

package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"time"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	transport "github.com/wenzhengcheng/115-media-hub/backend/internal/http"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/logs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/strm"
)

// App 持有数据库、任务中心、运行日志和 HTTP 服务的生命周期依赖；数据库须在服务停止后释放。
type App struct {
	database        *sqlite.Database
	server          *http.Server
	jobs            *jobs.Service
	logs            *logs.Service
	logger          *slog.Logger
	shutdownTimeout time.Duration
}

// New 打开并迁移 SQLite，装配认证、媒体库、115、STRM、任务中心和运行日志；构造失败时释放已打开的资源。
func New(ctx context.Context, config Config, logger *slog.Logger, assets fs.FS) (*App, error) {
	started := time.Now()
	database, err := sqlite.Open(ctx, config.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	runtimeLogs := logs.New(database, logger)
	logger = runtimeLogs.WrapLogger(logger)
	application := &App{database: database, logs: runtimeLogs, logger: logger, shutdownTimeout: config.ShutdownTimeout}
	authentication, err := auth.New(database, logger, auth.Options{})
	if err != nil {
		return nil, errors.Join(fmt.Errorf("initialize authentication: %w", err), application.Close())
	}
	libraries := library.New(database, logger)
	provider := provider115.NewWithConfig(database, logger, config.ProviderRequestInterval, config.ProviderFastScan)
	strmFiles := strm.New(database, provider, logger)
	application.jobs = jobs.New(database, libraries, map[domain.Type]jobs.Module{domain.STRM: strmFiles.Module()}, logger,
		jobs.Options{ObserveLog: runtimeLogs.CaptureJobLog})
	handler, err := transport.NewRouter(database, logger, assets, transport.AuthOptions{
		Service: authentication, CookieSecure: config.CookieSecure,
		Provider: provider, Library: libraries, Jobs: application.jobs, STRM: strmFiles, Logs: runtimeLogs,
	})
	if err != nil {
		return nil, errors.Join(fmt.Errorf("initialize HTTP routes: %w", err), application.Close())
	}
	logger.InfoContext(ctx, "database initialized", "module", "app", "operation", "initialize_database", "outcome", "succeeded", "duration_ms", time.Since(started).Milliseconds())
	application.server = &http.Server{
		Addr: config.HTTPAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 75 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20,
	}
	return application, nil
}

// Serve 启动任务中心并接管监听器，直到服务失败或上下文取消，再在关闭期限内停止请求和任务。
// 调用方应在返回后调用 Close 释放数据库。
func (a *App) Serve(ctx context.Context, listener net.Listener) error {
	if err := a.jobs.Start(ctx); err != nil {
		return errors.Join(fmt.Errorf("start task center: %w", err), listener.Close())
	}
	finished := make(chan error, 1)
	go func() { finished <- a.server.Serve(listener) }()
	a.logger.InfoContext(ctx, "HTTP server started", "module", "app", "operation", "serve", "address", listener.Addr().String())
	var serveErr error
	serverExited := false
	select {
	case serveErr = <-finished:
		serverExited = true
	case <-ctx.Done():
	}
	started := time.Now()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()
	a.logs.StopStreams()
	jobsStopped := make(chan error, 1)
	go func() { jobsStopped <- a.jobs.Stop(shutdownCtx) }()
	var result error
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		result = fmt.Errorf("shutdown HTTP: %w", errors.Join(err, a.server.Close()))
	}
	if !serverExited {
		serveErr = <-finished
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		result = errors.Join(result, fmt.Errorf("serve HTTP: %w", serveErr))
	}
	if err := <-jobsStopped; err != nil {
		result = errors.Join(result, fmt.Errorf("stop task center: %w", err))
	}
	if result == nil {
		a.logger.Info("HTTP server stopped", "module", "app", "operation", "shutdown", "outcome", "succeeded", "duration_ms", time.Since(started).Milliseconds())
	} else {
		a.logger.Error("HTTP server shutdown failed", "module", "app", "operation", "shutdown", "outcome", "failed",
			"error_code", "shutdown_failed", "duration_ms", time.Since(started).Milliseconds())
	}
	return errors.Join(result, a.logs.Close(shutdownCtx))
}

// Close 排空仍未结束的日志采集后释放数据库；也可处理未进入 Serve 的构造和监听失败。
func (a *App) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()
	return errors.Join(a.logs.Close(ctx), a.database.Close())
}
