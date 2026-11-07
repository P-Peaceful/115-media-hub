package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
)

type memoryRepository struct {
	administrator domain.Administrator
	settings      domain.Settings
	session       domain.Session
	failAudit     error
	sessionError  error
	revokeError   error
}

func (r *memoryRepository) Administrator(context.Context) (domain.Administrator, error) {
	if r.administrator.ID == "" {
		return domain.Administrator{}, ErrSetupRequired
	}
	return r.administrator, nil
}
func (r *memoryRepository) Settings(context.Context) (domain.Settings, error) { return r.settings, nil }
func (r *memoryRepository) FindSession(_ context.Context, hash string, now time.Time) (domain.Identity, error) {
	if hash != r.session.TokenHash || !now.Before(r.session.ExpiresAt) {
		return domain.Identity{}, ErrUnauthenticated
	}
	administrator := r.administrator
	administrator.PasswordHash = ""
	return domain.Identity{Session: r.session, Administrator: administrator}, nil
}
func (r *memoryRepository) Initialize(_ context.Context, administrator domain.Administrator, settings domain.Settings, session domain.Session, _ domain.Audit) error {
	if r.administrator.ID != "" {
		return ErrAlreadyInitialized
	}
	r.administrator = administrator
	r.settings = settings
	r.session = session
	return nil
}
func (r *memoryRepository) CreateSession(_ context.Context, session domain.Session, _, _ string, _ domain.Audit) error {
	if r.sessionError != nil {
		return r.sessionError
	}
	r.session = session
	return nil
}
func (r *memoryRepository) RevokeSession(context.Context, string, domain.Audit) error {
	if r.revokeError != nil {
		return r.revokeError
	}
	r.session = domain.Session{}
	return nil
}
func (r *memoryRepository) ResetPassword(_ context.Context, hash string, _ domain.Audit) error {
	r.administrator.PasswordHash = hash
	r.session = domain.Session{}
	return nil
}
func (r *memoryRepository) RecordAudit(context.Context, domain.Audit) error { return r.failAudit }

func TestIssuedBindingsExpiryRotationAndRedactedOutcomes(t *testing.T) {
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	var logs bytes.Buffer
	repo := &memoryRepository{}
	service, err := New(repo, slog.New(slog.NewJSONHandler(&logs, nil)), Options{Now: func() time.Time { return now }, PasswordCost: bcrypt.MinCost})
	if err != nil {
		t.Fatal(err)
	}
	cookie, csrf := service.AnonymousBinding()
	if !service.ValidateAnonymous(cookie, csrf) || service.ValidateAnonymous(cookie, "incorrect") || service.ValidateAnonymous(cookie+"x", csrf) {
		t.Fatal("issued anonymous binding validation failed")
	}
	result, err := service.Initialize(t.Context(), "admin", "ValidPass123!", domain.Settings{ServiceURL: "https://example.com", OutputDirectory: "/media", SessionLifetimeDays: 1}, "peer", "request-setup")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := service.Authenticate(t.Context(), result.Token)
	if err != nil || !service.ValidateSessionCSRF(identity, result.Token, result.CSRFToken) {
		t.Fatalf("issued authenticated binding: %v", err)
	}
	if service.ValidateSessionCSRF(identity, result.Token, csrf) {
		t.Fatal("anonymous csrf accepted after rotation")
	}
	if !identity.Session.ExpiresAt.Equal(now.Add(24 * time.Hour)) {
		t.Fatal("fixed lifetime incorrect")
	}
	if _, err := service.Login(t.Context(), "wrong", "WrongPass123!", "", "another-peer", "request-login"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal(err)
	}
	now = now.Add(20 * time.Minute)
	if service.ValidateAnonymous(cookie, csrf) {
		t.Fatal("anonymous binding did not expire")
	}
	// Authentication never slides the original expiry.
	if _, err := service.Authenticate(t.Context(), result.Token); err != nil {
		t.Fatal(err)
	}
	now = identity.Session.ExpiresAt
	if _, err := service.Authenticate(t.Context(), result.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("session usable at fixed expiry: %v", err)
	}
	for _, secret := range []string{"ValidPass123!", "WrongPass123!", result.Token, result.CSRFToken, csrf, cookie, repo.administrator.PasswordHash} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("secret entered operational log")
		}
	}
	decoder := json.NewDecoder(&logs)
	count := 0
	for {
		var event map[string]any
		err := decoder.Decode(&event)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
		for _, key := range []string{"module", "operation", "outcome", "duration_ms", "request_id"} {
			if _, ok := event[key]; !ok {
				t.Fatalf("missing outcome field %s: %+v", key, event)
			}
		}
	}
	if count != 2 {
		t.Fatalf("expected one terminal record per use case: %d", count)
	}
}

func TestAttemptLimiterRecoversAndBoundsAdmission(t *testing.T) {
	var limiter attemptLimiter
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	for range 5 {
		release, err := limiter.admit("peer", now)
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if _, err := limiter.admit("peer", now); err == nil {
		t.Fatal("per-peer attempts unlimited")
	}
	release, err := limiter.admit("peer", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("window did not recover: %v", err)
	}
	release()
	var releases []func()
	for _, peer := range []string{"a", "b", "c", "d"} {
		release, err := limiter.admit(peer, now.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	if _, err := limiter.admit("fifth-active", now.Add(time.Minute)); err == nil {
		t.Fatal("active hashing unbounded")
	}
	for _, release := range releases {
		release()
	}
	for i := 0; i < 25; i++ {
		release, err := limiter.admit(string(rune('e'+i)), now.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if _, err := limiter.admit("global-overflow", now.Add(time.Minute)); err == nil {
		t.Fatal("aggregate attempts unbounded")
	}
	if len(limiter.peers) > 1024 {
		t.Fatal("peer state unbounded")
	}
}

func TestValidationRejectsUnsafeConfiguration(t *testing.T) {
	for _, url := range []string{"/relative", "ftp://example.com", "https://user:pass@example.com", "https://example.com/prefix", "https://example.com?q=x", "https://example.com#x", "https://example.com?", "https://example.com#", "https://example.com:99999", "http://example.com:0", "https://example.com:"} {
		if err := validateSettings(domain.Settings{ServiceURL: url, OutputDirectory: "/media", SessionLifetimeDays: 7}); err == nil {
			t.Fatalf("unsafe service URL accepted: %s", url)
		}
	}
	for _, path := range []string{"relative", "/x/../y", "//double", "/trailing/", "/null\x00", "/line\n", "/tab\tname"} {
		if err := validateSettings(domain.Settings{ServiceURL: "http://localhost:8080", OutputDirectory: path, SessionLifetimeDays: 7}); err == nil {
			t.Fatalf("malformed output path accepted: %s", path)
		}
	}
	if err := validateCredentials("中文用户名", "LongPass123!"); err == nil {
		t.Fatal("non-ASCII username accepted")
	}
}

func TestFailedLoginAuditFailureHasOneRedactedOutcome(t *testing.T) {
	var logs bytes.Buffer
	hash, err := bcrypt.GenerateFromPassword([]byte("ValidPass123!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repo := &memoryRepository{administrator: domain.Administrator{ID: "administrator-id", Username: "admin", PasswordHash: string(hash)}, failAudit: errors.Join(ErrAuditUnavailable, errors.New("storage unavailable"))}
	service, err := New(repo, slog.New(slog.NewJSONHandler(&logs, nil)), Options{PasswordCost: bcrypt.MinCost})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(t.Context(), "admin", "WrongPass123!", "", "peer", "audit-request"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("audit failure did not fail closed: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["error_code"] != "audit_unavailable" || record["level"] != "ERROR" || record["operation"] != "login" || record["outcome"] != "failed" {
		t.Fatalf("missing audit failure outcome: %+v", record)
	}
	if strings.Contains(logs.String(), "password") || strings.Contains(logs.String(), string(hash)) {
		t.Fatal("audit failure exposed a credential")
	}
}

func TestConcurrentCredentialTransitionsKeepExpectedErrors(t *testing.T) {
	var logs bytes.Buffer
	hash, err := bcrypt.GenerateFromPassword([]byte("ValidPass123!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repo := &memoryRepository{administrator: domain.Administrator{ID: "admin-id", Username: "admin", PasswordHash: string(hash)}, sessionError: ErrInvalidCredentials, revokeError: ErrUnauthenticated}
	service, err := New(repo, slog.New(slog.NewJSONHandler(&logs, nil)), Options{PasswordCost: bcrypt.MinCost})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(t.Context(), "admin", "ValidPass123!", "", "peer", "racing-login"); !errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("credential race misclassified: %v", err)
	}
	if err := service.Logout(t.Context(), domain.Identity{}, "racing-logout"); !errors.Is(err, ErrUnauthenticated) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("revocation race misclassified: %v", err)
	}
	decoder := json.NewDecoder(&logs)
	for _, code := range []string{"invalid_credentials", "authentication_required"} {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		if record["error_code"] != code || record["level"] == "ERROR" {
			t.Fatalf("expected security outcome logged as operational failure: %+v", record)
		}
	}
}

func TestPasswordPolicyAcceptsOnlyPrintableNonSpaceASCII(t *testing.T) {
	for _, test := range []struct {
		name, password string
		valid          bool
	}{
		{"minimum", "abcdefgh", true}, {"maximum", strings.Repeat("A", 32), true},
		{"digits_only", "12345678", true}, {"punctuation_only", "!@#$%^&*()_+-=[]{};:,.<>/?\\|`~\"'", true},
		{"too_short", "abcdefg", false}, {"too_long", strings.Repeat("A", 33), false},
		{"chinese", "密码Password123", false}, {"space", "Valid Pass123", false},
		{"leading_space", " ValidPass123", false}, {"trailing_space", "ValidPass123 ", false},
		{"tab", "Valid\tPass123", false}, {"linefeed", "ValidPass123\n", false},
		{"carriage_return", "ValidPass123\r", false}, {"nul", "Valid\x00Pass123", false},
		{"delete", "ValidPass123\x7f", false}, {"nonbreaking_space", "ValidPass123\u00a0", false},
		{"nonascii_control", "ValidPass123\u0085", false}, {"invalid_utf8", "ValidPass123\xff", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePassword(test.password)
			if (err == nil) != test.valid {
				t.Fatalf("policy accepted=%v, expected=%v", err == nil, test.valid)
			}
			if err != nil {
				var validation *ValidationError
				if !errors.As(err, &validation) || validation.Fields["password"] == "" {
					t.Fatal("missing safe password field validation")
				}
			}
		})
	}
}

func TestPasswordPolicyIsSharedByInitializeLoginAndRecovery(t *testing.T) {
	repo := &memoryRepository{}
	service, err := New(repo, slog.New(slog.NewJSONHandler(io.Discard, nil)), Options{PasswordCost: bcrypt.MinCost})
	if err != nil {
		t.Fatal(err)
	}
	maximum := strings.Repeat("A", 32)
	settings := domain.Settings{ServiceURL: "https://example.com", OutputDirectory: "/media", SessionLifetimeDays: 7}
	if _, err := service.Initialize(t.Context(), "admin", maximum+"A", settings, "peer", "invalid-setup"); err == nil || repo.administrator.ID != "" {
		t.Fatal("invalid initialization changed account state")
	}
	result, err := service.Initialize(t.Context(), "admin", maximum, settings, "peer", "maximum-setup")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(t.Context(), "admin", maximum, "", "peer", "maximum-login"); err != nil {
		t.Fatalf("32-character password did not authenticate: %v", err)
	}
	for _, invalid := range []string{maximum + "A", "密码Password123", "ValidPass123 ", "ValidPass123\n"} {
		if _, err := service.Login(t.Context(), "admin", invalid, "", "peer", "invalid-login"); err == nil {
			t.Fatal("login accepted invalid password shape")
		}
		before := repo.administrator.PasswordHash
		if err := service.ResetPassword(t.Context(), invalid, "invalid-reset"); err == nil || repo.administrator.PasswordHash != before {
			t.Fatal("invalid recovery changed password")
		}
	}
	punctuation := "!@#$%^&*()[]{}:;,.<>/?\\|`~\"'_-+="
	if err := service.ResetPassword(t.Context(), punctuation, "punctuation-reset"); err != nil {
		t.Fatalf("ASCII punctuation recovery rejected: %v", err)
	}
	if _, err := service.Authenticate(t.Context(), result.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("recovery failed to revoke old session")
	}
	if _, err := service.Login(t.Context(), "admin", punctuation, "", "peer", "punctuation-login"); err != nil {
		t.Fatalf("ASCII punctuation did not authenticate: %v", err)
	}
}
