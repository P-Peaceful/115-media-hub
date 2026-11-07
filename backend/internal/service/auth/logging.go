package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

func (s *Service) logOutcome(ctx context.Context, operation, correlation string, started time.Time, err error) {
	outcome, code, level := "succeeded", "", slog.LevelInfo
	if err != nil {
		outcome, code = "failed", "authentication_failed"
		var throttled *ThrottledError
		var validation *ValidationError
		switch {
		case errors.Is(err, context.Canceled):
			outcome, code = "canceled", "canceled"
		case errors.Is(err, ErrAuditUnavailable):
			code, level = "audit_unavailable", slog.LevelError
		case errors.Is(err, ErrUnavailable):
			code, level = "authentication_unavailable", slog.LevelError
		case errors.As(err, &throttled):
			code, level = "rate_limited", slog.LevelWarn
		case errors.As(err, &validation):
			code = "validation_failed"
		case errors.Is(err, ErrAlreadyInitialized):
			code = "already_initialized"
		case errors.Is(err, ErrSetupRequired):
			code = "setup_required"
		case errors.Is(err, ErrUnauthenticated):
			code = "authentication_required"
		case errors.Is(err, ErrInvalidCredentials):
			code = "invalid_credentials"
		}
	}
	attributes := []any{"operation", operation, "outcome", outcome, "duration_ms", time.Since(started).Milliseconds(), "request_id", correlation}
	if code != "" {
		attributes = append(attributes, "error_code", code)
	}
	// Underlying errors and submitted payloads are intentionally absent; SQL errors can contain values.
	s.logger.Log(ctx, level, "authentication operation completed", attributes...)
}
