package auth

import (
	"errors"
	"time"
)

var (
	ErrSetupRequired      = errors.New("administrator setup required")
	ErrAlreadyInitialized = errors.New("administrator already initialized")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated    = errors.New("authentication required")
	ErrCSRF               = errors.New("invalid CSRF proof")
	ErrAuditUnavailable   = errors.New("security audit unavailable")
	ErrUnavailable        = errors.New("authentication unavailable")
)

// ValidationError contains safe, field-specific messages.
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "invalid authentication input" }

// ThrottledError specifies the minimum delay before another expensive attempt.
type ThrottledError struct{ RetryAfter time.Duration }

func (e *ThrottledError) Error() string { return "authentication attempts throttled" }
