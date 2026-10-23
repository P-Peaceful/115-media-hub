package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/app"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/config"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/logging"
)

func run() error {
	path := flag.String("config", "", "optional YAML configuration path")
	flag.Parse()
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	logger, closer, err := logging.New(cfg.LogDir)
	if err != nil {
		return err
	}
	defer closer.Close()
	slog.SetDefault(logger)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return app.Run(ctx, cfg)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "startup failed:", err)
		os.Exit(1)
	}
}
