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

	transport "github.com/wenzhengcheng/115-media-hub/backend/internal/http"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
)

// App owns constructed dependencies and the HTTP server.
type App struct {
	database        *sqlite.Database
	server          *http.Server
	logger          *slog.Logger
	shutdownTimeout time.Duration
}

// New opens and migrates SQLite before constructing the HTTP transport.
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
	handler, err := transport.NewRouter(database, logger, assets, transport.AuthOptions{Service: authentication, CookieSecure: config.CookieSecure})
	if err != nil {
		return nil, errors.Join(fmt.Errorf("initialize HTTP routes: %w", err), database.Close())
	}
	logger.InfoContext(ctx, "database initialized", "module", "app", "operation", "initialize_database", "outcome", "succeeded", "duration_ms", time.Since(started).Milliseconds())
	return &App{database: database, logger: logger, shutdownTimeout: config.ShutdownTimeout, server: &http.Server{Addr: config.HTTPAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}}, nil
}

// Serve serves the supplied listener until failure or cancellation, draining admitted requests.
// The caller owns closing the App after Serve returns.
func (a *App) Serve(ctx context.Context, listener net.Listener) error {
	finished := make(chan error, 1)
	go func() { finished <- a.server.Serve(listener) }()
	a.logger.InfoContext(ctx, "HTTP server started", "module", "app", "operation", "serve", "address", listener.Addr().String())
	select {
	case err := <-finished:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
	}
	started := time.Now()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()
	err := a.server.Shutdown(shutdownCtx)
	if err != nil {
		err = errors.Join(err, a.server.Close())
	}
	serveErr := <-finished
	if !errors.Is(serveErr, http.ErrServerClosed) {
		err = errors.Join(err, serveErr)
	}
	if err != nil {
		return fmt.Errorf("shutdown HTTP: %w", err)
	}
	a.logger.Info("HTTP server stopped", "module", "app", "operation", "shutdown", "outcome", "succeeded", "duration_ms", time.Since(started).Milliseconds())
	return nil
}

// Close releases database resources; call it after Serve returns.
func (a *App) Close() error { return a.database.Close() }
