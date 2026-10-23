package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/config"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/database"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/router"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/webui"
)

func Run(ctx context.Context, cfg config.Config) error {
	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: router.New(sqlDB.PingContext, webui.Files()), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	slog.Info("server starting", "address", cfg.HTTPAddr)
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		slog.Info("server stopped")
		return nil
	}
}
