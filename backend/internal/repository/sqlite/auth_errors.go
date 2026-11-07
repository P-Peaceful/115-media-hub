package sqlite

import (
	"context"
	"database/sql"
	"errors"

	driver "github.com/mattn/go-sqlite3"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
)

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
