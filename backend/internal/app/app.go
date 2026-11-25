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
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/strm"
)

// App 持有数据库、任务中心和 HTTP 服务的生命周期依赖；数据库须在服务停止后释放。
type App struct {
	database        *sqlite.Database
	server          *http.Server
	jobs            *jobs.Service
	logger          *slog.Logger
	shutdownTimeout time.Duration
}

// New 打开并迁移 SQLite，装配认证、媒体库、115、STRM 和任务中心；构造失败时关闭数据库。
func New(ctx context.Context, config Config, logger *slog.Logger, assets fs.FS) (*App, error) {
	started := time.Now()
	database, err := sqlite.Open(ctx, config.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	authentication, err := auth.New(database, logger, auth.Options{})
	if err != nil {
		return nil, errors.Join(fmt.Errorf("initialize authentication: %w", err), database.Close())
	}
	libraries := library.New(database, logger)
	provider := provider115.NewWithConfig(database, logger, config.ProviderRequestInterval, config.ProviderFastScan)
	strmFiles := strm.New(database, provider, logger)
	tasks := jobs.New(database, libraries, map[domain.Type]jobs.Module{domain.STRM: strmFiles.Module()}, logger)
	handler, err := transport.NewRouter(database, logger, assets, transport.AuthOptions{
		Service: authentication, CookieSecure: config.CookieSecure,
		Provider: provider, Library: libraries, Jobs: tasks, STRM: strmFiles,
	})
	if err != nil {
		return nil, errors.Join(fmt.Errorf("initialize HTTP routes: %w", err), database.Close())
	}
	logger.InfoContext(ctx, "database initialized", "module", "app", "operation", "initialize_database", "outcome", "succeeded", "duration_ms", time.Since(started).Milliseconds())
	return &App{database: database, jobs: tasks, logger: logger, shutdownTimeout: config.ShutdownTimeout, server: &http.Server{Addr: config.HTTPAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 75 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}}, nil
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
	if result != nil {
		return result
	}
	a.logger.Info("HTTP server stopped", "module", "app", "operation", "shutdown", "outcome", "succeeded", "duration_ms", time.Since(started).Milliseconds())
	return nil
}

// Close 释放数据库连接；应在 Serve 完成任务与请求停止之后调用。
func (a *App) Close() error { return a.database.Close() }
