// logging.go：将认证操作结果映射为稳定的日志级别和错误码，避免记录底层错误与提交的凭据。

package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// logOutcome 为认证用例记录一次带耗时和关联标识的结束结果，区分取消、预期拒绝及基础设施失败。
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
	// 不记录底层错误或提交载荷，SQL 错误可能带有凭据等实际值。
	s.logger.Log(ctx, level, "authentication operation completed", attributes...)
}
