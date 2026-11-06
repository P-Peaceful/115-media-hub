package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/app"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/web"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("application stopped", "module", "app", "operation", "run", "outcome", "failed", "error_code", "startup_or_shutdown_failed", "error", err.Error())
		os.Exit(1)
	}
}

func run() (resultErr error) {
	config, err := app.LoadConfig()
	if err != nil {
		return err
	}
	logger, logs, err := app.NewLogger(config, os.Stdout)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, logs.Close()) }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	assets, err := web.Assets()
	if err != nil {
		return fmt.Errorf("load frontend assets: %w", err)
	}
	application, err := app.New(ctx, config, logger, assets)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, application.Close()) }()
	listener, err := net.Listen("tcp", config.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen HTTP: %w", err)
	}
	return application.Serve(ctx, listener)
}
