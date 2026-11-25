// main.go：装配进程启动流程，选择本地维护命令或 HTTP 服务，并统一回收日志与应用资源。

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

// main 设置 Gin 运行模式并启动程序；失败时记录结构化日志并返回非零退出码。
func main() {
	gin.SetMode(gin.ReleaseMode)
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("application stopped", "module", "app", "operation", "run", "outcome", "failed", "error_code", "startup_or_shutdown_failed", "error", err.Error())
		os.Exit(1)
	}
}

// run 读取配置、创建日志和信号上下文，再启动维护命令或服务；退出时汇总资源关闭错误。
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
	if len(os.Args) > 1 {
		return runCommand(ctx, config, logger, os.Args[1:], os.Stdin, os.Stderr)
	}
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
