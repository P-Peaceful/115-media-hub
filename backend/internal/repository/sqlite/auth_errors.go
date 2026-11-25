// auth_errors.go：将认证存储错误映射为允许记录的原因代码，保留底层错误链而不直接展示 SQL 信息。

package sqlite

import (
	"context"
	"database/sql"
	"errors"

	driver "github.com/mattn/go-sqlite3"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
)

// authStorageError 按取消、超时及 SQLite 错误种类生成白名单诊断，禁止将驱动原始消息当作安全日志内容。
func authStorageError(operation string, err error) error {
	reason := "database_unavailable"
	var sqliteErr driver.Error
	switch {
	case errors.Is(err, context.Canceled):
		reason = "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		reason = "deadline_exceeded"
	case errors.Is(err, sql.ErrConnDone):
		reason = "connection_closed"
	case errors.As(err, &sqliteErr):
		switch sqliteErr.Code {
		case driver.ErrError:
			reason = "invalid_query_or_schema"
		case driver.ErrBusy, driver.ErrLocked:
			reason = "database_busy"
		case driver.ErrReadonly:
			reason = "database_readonly"
		case driver.ErrCorrupt, driver.ErrNotADB:
			reason = "database_corrupt"
		case driver.ErrConstraint:
			reason = "constraint_failed"
		case driver.ErrIoErr, driver.ErrCantOpen:
			reason = "database_io_failed"
		}
	}
	return &auth.StorageError{Operation: operation, Reason: reason, Cause: err}
}
