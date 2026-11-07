// Package auth defines administrator authentication data independently of transport and SQL.
package auth

import "time"

// Administrator is the sole local administrator. PasswordHash never crosses transport boundaries.
type Administrator struct {
	ID           string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

// Settings contains the first-run business settings.
type Settings struct {
	ServiceURL          string
	OutputDirectory     string
	SessionLifetimeDays int
}

// Session stores only non-reversible bindings for the browser credential and CSRF proof.
type Session struct {
	TokenHash       string
	AdministratorID string
	CSRFHash        string
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

// Identity is an authenticated session and its administrator's public identity.
type Identity struct {
	Session       Session
	Administrator Administrator
}

// Audit is a secret-free durable record of a security operation.
type Audit struct {
	ActorID       string
	Action        string
	TargetID      string
	Outcome       string
	CorrelationID string
	OccurredAt    time.Time
}
