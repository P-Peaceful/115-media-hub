// recovery.go：为本地维护提供管理员密码重置入口，仅操作既有数据库，结束后释放恢复流程创建的连接。

package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
)

// ResetAdministratorPassword 仅打开已存在的配置数据库并重置管理员密码，返回时合并关闭错误。
// 调用方负责安全读取密码，并应先停止正在使用该数据库的服务。
func ResetAdministratorPassword(ctx context.Context, config Config, logger *slog.Logger, password string) (resultErr error) {
	if _, err := os.Stat(config.DatabasePath); err != nil {
		return fmt.Errorf("password recovery requires an existing configured database: %w", err)
	}
	database, err := sqlite.Open(ctx, config.DatabasePath)
	if err != nil {
		return fmt.Errorf("open recovery database: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, database.Close()) }()
	service, err := auth.New(database, logger, auth.Options{})
	if err != nil {
		return err
	}
	return service.ResetPassword(ctx, password, "local-"+rand.Text())
}
