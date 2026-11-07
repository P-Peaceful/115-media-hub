// Package auth owns authentication policy and atomic security use cases.
package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
)

// Repository owns storage mechanics; mutations include their required audit in the same transaction.
type Repository interface {
	Administrator(context.Context) (domain.Administrator, error)
	Settings(context.Context) (domain.Settings, error)
	FindSession(context.Context, string, time.Time) (domain.Identity, error)
	Initialize(context.Context, domain.Administrator, domain.Settings, domain.Session, domain.Audit) error
	CreateSession(context.Context, domain.Session, string, string, domain.Audit) error
	RevokeSession(context.Context, string, domain.Audit) error
	ResetPassword(context.Context, string, domain.Audit) error
	RecordAudit(context.Context, domain.Audit) error
}

// Options provides a controllable clock/cost for tests; production uses cost 12.
type Options struct {
	Now          func() time.Time
	PasswordCost int
}

// Service contains no HTTP or SQL dependencies.
type Service struct {
	repo         Repository
	logger       *slog.Logger
	now          func() time.Time
	cost         int
	anonymousKey []byte
	dummyHash    []byte
	limiter      attemptLimiter
}

// New constructs authentication policy and a restart-local anonymous-cookie signing key.
func New(repo Repository, logger *slog.Logger, options Options) (*Service, error) {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.PasswordCost == 0 {
		options.PasswordCost = 12
	}
	if options.PasswordCost < bcrypt.MinCost || options.PasswordCost > bcrypt.MaxCost {
		return nil, fmt.Errorf("invalid bcrypt cost")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate anonymous signing key: %w", err)
	}
	dummyHash, err := bcrypt.GenerateFromPassword([]byte(rand.Text()), options.PasswordCost)
	if err != nil {
		return nil, fmt.Errorf("prepare credential comparison: %w", err)
	}
	return &Service{repo: repo, logger: logger.With("module", "auth"), now: options.Now, cost: options.PasswordCost, anonymousKey: key, dummyHash: dummyHash}, nil
}

// SessionResult contains the raw cookie solely for transport; it is never logged or persisted.
type SessionResult struct {
	Identity  domain.Identity
	Token     string
	CSRFToken string
}

// Initialized reveals only whether the singleton administrator exists.
func (s *Service) Initialized(ctx context.Context) (bool, error) {
	_, err := s.repo.Administrator(ctx)
	if errors.Is(err, ErrSetupRequired) {
		return false, nil
	}
	return err == nil, unavailable(err)
}

// Authenticate looks up an opaque credential and enforces fixed expiry.
func (s *Service) Authenticate(ctx context.Context, token string) (domain.Identity, error) {
	if !validSessionToken(token) {
		return domain.Identity{}, ErrUnauthenticated
	}
	identity, err := s.repo.FindSession(ctx, tokenHash(token), s.now().UTC())
	if errors.Is(err, ErrUnauthenticated) {
		return domain.Identity{}, err
	}
	return identity, unavailable(err)
}

// CSRFForSession reproduces a proof bound to the secret cookie without storing the raw proof.
func (s *Service) CSRFForSession(token string) string { return csrfToken(token) }

// ValidateSessionCSRF compares both the submitted proof and its persisted binding in constant time.
func (s *Service) ValidateSessionCSRF(identity domain.Identity, token, proof string) bool {
	return tokenEqual(proof, csrfToken(token)) && tokenEqual(tokenHash(proof), identity.Session.CSRFHash)
}

// Initialize creates the singleton settings, administrator and first persistent session.
func (s *Service) Initialize(ctx context.Context, username, password string, settings domain.Settings, peer, correlation string) (result SessionResult, resultErr error) {
	started := time.Now()
	defer func() { s.logOutcome(ctx, "initialize", correlation, started, resultErr) }()
	if err := validateCredentials(username, password); err != nil {
		return result, err
	}
	if err := validateSettings(settings); err != nil {
		return result, err
	}
	release, err := s.limiter.admit(peer, s.now())
	if err != nil {
		return result, err
	}
	defer release()
	initialized, err := s.Initialized(ctx)
	if err != nil {
		return result, err
	}
	if initialized {
		return result, ErrAlreadyInitialized
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return result, unavailable(err)
	}
	now := s.now().UTC()
	administrator := domain.Administrator{ID: rand.Text(), Username: username, PasswordHash: string(hash), CreatedAt: now}
	result = s.newSession(administrator, settings, now)
	err = s.repo.Initialize(ctx, administrator, settings, result.Identity.Session, s.audit(administrator.ID, "initialize", administrator.ID, "succeeded", correlation))
	if errors.Is(err, ErrAlreadyInitialized) {
		return SessionResult{}, err
	}
	if err != nil {
		return SessionResult{}, unavailable(err)
	}
	return result, nil
}

// Login uses one safe credential error and rotates any supplied old session on success.
func (s *Service) Login(ctx context.Context, username, password, oldToken, peer, correlation string) (result SessionResult, resultErr error) {
	started := time.Now()
	defer func() { s.logOutcome(ctx, "login", correlation, started, resultErr) }()
	if err := validateCredentials(username, password); err != nil {
		return result, err
	}
	release, err := s.limiter.admit(peer, s.now())
	if err != nil {
		return result, err
	}
	defer release()
	administrator, err := s.repo.Administrator(ctx)
	if errors.Is(err, ErrSetupRequired) {
		return result, err
	}
	if err != nil {
		return result, unavailable(err)
	}
	comparison := []byte(administrator.PasswordHash)
	if administrator.Username != username {
		comparison = s.dummyHash
	}
	passwordErr := bcrypt.CompareHashAndPassword(comparison, []byte(password))
	if passwordErr != nil || administrator.Username != username {
		actor := ""
		if administrator.Username == username {
			actor = administrator.ID
		}
		if err := s.repo.RecordAudit(ctx, s.audit(actor, "login", administrator.ID, "failed", correlation)); err != nil {
			return result, unavailable(err)
		}
		return result, ErrInvalidCredentials
	}
	settings, err := s.repo.Settings(ctx)
	if err != nil {
		return result, unavailable(err)
	}
	result = s.newSession(administrator, settings, s.now().UTC())
	oldHash := ""
	if validSessionToken(oldToken) {
		oldHash = tokenHash(oldToken)
	}
	if err := s.repo.CreateSession(ctx, result.Identity.Session, oldHash, administrator.PasswordHash, s.audit(administrator.ID, "login", administrator.ID, "succeeded", correlation)); err != nil {
		return SessionResult{}, unavailable(err)
	}
	return result, nil
}

// Logout atomically revokes the session and records a required audit.
func (s *Service) Logout(ctx context.Context, identity domain.Identity, correlation string) (resultErr error) {
	started := time.Now()
	defer func() { s.logOutcome(ctx, "logout", correlation, started, resultErr) }()
	return unavailable(s.repo.RevokeSession(ctx, identity.Session.TokenHash, s.audit(identity.Administrator.ID, "logout", identity.Administrator.ID, "succeeded", correlation)))
}

// ResetPassword changes only an existing administrator and revokes every session atomically.
func (s *Service) ResetPassword(ctx context.Context, password, correlation string) (resultErr error) {
	started := time.Now()
	defer func() { s.logOutcome(ctx, "reset_password", correlation, started, resultErr) }()
	if err := ValidatePassword(password); err != nil {
		return err
	}
	administrator, err := s.repo.Administrator(ctx)
	if errors.Is(err, ErrSetupRequired) {
		return err
	}
	if err != nil {
		return unavailable(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return unavailable(err)
	}
	return unavailable(s.repo.ResetPassword(ctx, string(hash), s.audit("local_maintenance", "reset_password", administrator.ID, "succeeded", correlation)))
}

func (s *Service) newSession(administrator domain.Administrator, settings domain.Settings, now time.Time) SessionResult {
	token := randomToken()
	csrf := csrfToken(token)
	session := domain.Session{TokenHash: tokenHash(token), AdministratorID: administrator.ID, CSRFHash: tokenHash(csrf), CreatedAt: now, ExpiresAt: now.Add(time.Duration(settings.SessionLifetimeDays) * 24 * time.Hour)}
	administrator.PasswordHash = ""
	return SessionResult{Identity: domain.Identity{Session: session, Administrator: administrator}, Token: token, CSRFToken: csrf}
}

func (s *Service) audit(actor, action, target, outcome, correlation string) domain.Audit {
	return domain.Audit{ActorID: actor, Action: action, TargetID: target, Outcome: outcome, CorrelationID: correlation, OccurredAt: s.now().UTC()}
}

func unavailable(err error) error {
	if err == nil {
		return nil
	}
	for _, expected := range []error{ErrSetupRequired, ErrAlreadyInitialized, ErrInvalidCredentials, ErrUnauthenticated, ErrCSRF} {
		if errors.Is(err, expected) {
			return err
		}
	}
	return errors.Join(ErrUnavailable, err)
}
