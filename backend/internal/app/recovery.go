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

// ResetAdministratorPassword opens only an existing configured database for local maintenance.
// The caller owns secure input and stops the server before invoking this command.
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
