package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pressly/goose/v3"
	"golang.org/x/crypto/bcrypt"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/migrations"
)

func testAuthDatabase(t *testing.T) (*Database, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.db")
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db, path
}

func testAuthService(t *testing.T, repo auth.Repository, now func() time.Time) *auth.Service {
	t.Helper()
	service, err := auth.New(repo, slog.New(slog.NewJSONHandler(io.Discard, nil)), auth.Options{PasswordCost: bcrypt.MinCost, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func authSettings() domain.Settings {
	return domain.Settings{ServiceURL: "https://media.example.com", OutputDirectory: "/media", SessionLifetimeDays: 7}
}

func TestConcurrentInitializeAndPersistedSession(t *testing.T) {
	db, path := testAuthDatabase(t)
	service := testAuthService(t, db, nil)
	var wait sync.WaitGroup
	results := make(chan auth.SessionResult, 2)
	failures := make(chan error, 2)
	for range 2 {
		wait.Go(func() {
			result, err := service.Initialize(t.Context(), "admin", "ValidPass123!", authSettings(), "peer", "setup")
			if err != nil {
				failures <- err
			} else {
				results <- result
			}
		})
	}
	wait.Wait()
	if len(results) != 1 || len(failures) != 1 {
		t.Fatalf("setup outcomes: success=%d failure=%d", len(results), len(failures))
	}
	if err := <-failures; !errors.Is(err, auth.ErrAlreadyInitialized) {
		t.Fatal(err)
	}
	result := <-results
	for _, table := range []string{"administrators", "system_settings", "admin_sessions", "audit_records"} {
		var count int64
		if err := db.orm.Table(table).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	var stored sessionModel
	if err := db.orm.Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TokenHash == result.Token || stored.CSRFHash == result.CSRFToken || len(stored.TokenHash) != 64 {
		t.Fatal("raw session credential persisted")
	}
	reopened, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	}()
	restarted := testAuthService(t, reopened, nil)
	identity, err := restarted.Authenticate(t.Context(), result.Token)
	if err != nil || identity.Administrator.Username != "admin" || !restarted.ValidateSessionCSRF(identity, result.Token, result.CSRFToken) {
		t.Fatalf("restart lost authenticated binding: %+v %v", identity, err)
	}
	if identity.Administrator.PasswordHash != "" {
		t.Fatal("authenticated identity exposed hash")
	}
}

func TestRequiredAuditFailureRollsBackSecurityMutations(t *testing.T) {
	db, _ := testAuthDatabase(t)
	service := testAuthService(t, db, nil)
	failAudit := func() {
		t.Helper()
		if err := db.orm.Exec("CREATE TRIGGER reject_audit BEFORE INSERT ON audit_records BEGIN SELECT RAISE(FAIL, 'audit unavailable'); END").Error; err != nil {
			t.Fatal(err)
		}
	}
	restoreAudit := func() {
		t.Helper()
		if err := db.orm.Exec("DROP TRIGGER reject_audit").Error; err != nil {
			t.Fatal(err)
		}
	}
	failAudit()
	if _, err := service.Initialize(t.Context(), "admin", "ValidPass123!", authSettings(), "setup", "setup-fail"); !errors.Is(err, auth.ErrUnavailable) {
		t.Fatalf("initialization audit: %v", err)
	}
	if initialized, err := service.Initialized(t.Context()); err != nil || initialized {
		t.Fatalf("partial setup persisted: %v %v", initialized, err)
	}
	restoreAudit()
	result, err := service.Initialize(t.Context(), "admin", "ValidPass123!", authSettings(), "setup2", "setup-pass")
	if err != nil {
		t.Fatal(err)
	}
	failAudit()
	if _, err := service.Login(t.Context(), "admin", "WrongPass123!", "", "login-fail", "failure-audit"); !errors.Is(err, auth.ErrUnavailable) {
		t.Fatalf("failed login audit: %v", err)
	}
	if _, err := service.Login(t.Context(), "admin", "ValidPass123!", result.Token, "login-pass", "login-audit"); !errors.Is(err, auth.ErrUnavailable) {
		t.Fatalf("success login audit: %v", err)
	}
	if err := service.Logout(t.Context(), result.Identity, "logout-audit"); !errors.Is(err, auth.ErrUnavailable) {
		t.Fatalf("logout audit: %v", err)
	}
	if err := service.ResetPassword(t.Context(), "NewPass123!", "reset-audit"); !errors.Is(err, auth.ErrUnavailable) {
		t.Fatalf("reset audit: %v", err)
	}
	if _, err := service.Authenticate(t.Context(), result.Token); err != nil {
		t.Fatalf("failed transactions revoked old session: %v", err)
	}
	administrator, err := db.Administrator(t.Context())
	if err != nil || bcrypt.CompareHashAndPassword([]byte(administrator.PasswordHash), []byte("ValidPass123!")) != nil {
		t.Fatal("failed reset changed password")
	}
	var sessions int64
	if err := db.orm.Table("admin_sessions").Count(&sessions).Error; err != nil || sessions != 1 {
		t.Fatalf("failed login left extra session: %d %v", sessions, err)
	}
}

func TestRecoveryRevokesAllSessionsAndPreservesSettings(t *testing.T) {
	db, _ := testAuthDatabase(t)
	service := testAuthService(t, db, nil)
	if err := service.ResetPassword(t.Context(), "NewPass123!", "absent"); !errors.Is(err, auth.ErrSetupRequired) {
		t.Fatalf("reset created admin: %v", err)
	}
	first, err := service.Initialize(t.Context(), "admin", "ValidPass123!", authSettings(), "setup", "setup")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Login(t.Context(), "admin", "ValidPass123!", "", "login", "login")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ResetPassword(t.Context(), "NewPass123!", "local-recovery"); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{first.Token, second.Token} {
		if _, err := service.Authenticate(t.Context(), token); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("reset left session active: %v", err)
		}
	}
	settings, err := db.Settings(t.Context())
	if err != nil || settings != authSettings() {
		t.Fatalf("reset changed settings: %+v %v", settings, err)
	}
	if _, err := service.Login(t.Context(), "admin", "ValidPass123!", "", "old", "old-password"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("old password: %v", err)
	}
	if _, err := service.Login(t.Context(), "admin", "NewPass123!", "", "new", "new-password"); err != nil {
		t.Fatal(err)
	}
	var audit auditModel
	if err := db.orm.Where("action = ?", "reset_password").Take(&audit).Error; err != nil || audit.ActorID != "local_maintenance" || audit.CorrelationID != "local-recovery" {
		t.Fatalf("recovery audit: %+v %v", audit, err)
	}
}

type delayedSessionRepository struct {
	*Database
	compared chan struct{}
	proceed  chan struct{}
}

func (r delayedSessionRepository) CreateSession(ctx context.Context, session domain.Session, oldHash, expected string, audit domain.Audit) error {
	close(r.compared)
	<-r.proceed
	return r.Database.CreateSession(ctx, session, oldHash, expected, audit)
}

func TestResetPreventsInFlightOldPasswordLogin(t *testing.T) {
	db, _ := testAuthDatabase(t)
	initial := testAuthService(t, db, nil)
	if _, err := initial.Initialize(t.Context(), "admin", "ValidPass123!", authSettings(), "setup", "setup"); err != nil {
		t.Fatal(err)
	}
	repo := delayedSessionRepository{Database: db, compared: make(chan struct{}), proceed: make(chan struct{})}
	service := testAuthService(t, repo, nil)
	finished := make(chan error, 1)
	go func() {
		_, err := service.Login(t.Context(), "admin", "ValidPass123!", "", "peer", "delayed-login")
		finished <- err
	}()
	<-repo.compared
	if err := initial.ResetPassword(t.Context(), "NewPass123!", "reset"); err != nil {
		t.Fatal(err)
	}
	close(repo.proceed)
	if err := <-finished; !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("old password login survived reset: %v", err)
	}
	var sessions, successfulLogins int64
	if err := db.orm.Table("admin_sessions").Count(&sessions).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.orm.Table("audit_records").Where("action = ? AND outcome = ?", "login", "succeeded").Count(&successfulLogins).Error; err != nil {
		t.Fatal(err)
	}
	if sessions != 0 || successfulLogins != 0 {
		t.Fatalf("old credential committed session/audit: %d %d", sessions, successfulLogins)
	}
}

func TestMigrationUpgradeFromApplicationOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	pool, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := migrations.Files.ReadFile("00001_application.sql")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, fstest.MapFS{"00001_application.sql": &fstest.MapFile{Data: first}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := db.Administrator(t.Context()); !errors.Is(err, auth.ErrSetupRequired) {
		t.Fatalf("upgrade state: %v", err)
	}
	var sqlText string
	if err := db.sql.QueryRowContext(t.Context(), "SELECT sql FROM sqlite_master WHERE name = 'administrators'").Scan(&sqlText); err != nil || !strings.Contains(sqlText, "singleton = 1") {
		t.Fatalf("upgrade missing constraint: %s %v", sqlText, err)
	}
}

func TestSecuritySettingsPreserveCurrentSessionAndRevokeOthers(t *testing.T) {
	db, _ := testAuthDatabase(t)
	service := testAuthService(t, db, nil)
	ctx := t.Context()
	initial, err := service.Initialize(ctx, "admin", "ValidPass123!", authSettings(), "peer", "init")
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.Login(ctx, "admin", "ValidPass123!", "", "peer", "login")
	if err != nil {
		t.Fatal(err)
	}
	if err = service.UpdateCredentials(ctx, initial.Identity, "changed", "WrongPass!", "NewPass123!", "change"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if err = service.UpdateCredentials(ctx, initial.Identity, "changed", "ValidPass123!", "NewPass123!", "change"); err != nil {
		t.Fatal(err)
	}
	if identity, err := service.Authenticate(ctx, initial.Token); err != nil || identity.Administrator.Username != "changed" {
		t.Fatalf("current session: %+v %v", identity, err)
	}
	if _, err := service.Authenticate(ctx, other.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("other session: %v", err)
	}
	if _, err := service.Login(ctx, "admin", "ValidPass123!", "", "peer", "old"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("old login: %v", err)
	}
	if err := service.UpdateSessionLifetime(ctx, initial.Identity, 30, "settings"); err != nil {
		t.Fatal(err)
	}
	next, err := service.Login(ctx, "changed", "NewPass123!", "", "peer", "new")
	if err != nil {
		t.Fatal(err)
	}
	if next.Identity.Session.ExpiresAt.Sub(next.Identity.Session.CreatedAt) != 30*24*time.Hour {
		t.Fatal("new lifetime not applied")
	}
	if initial.Identity.Session.ExpiresAt.Sub(initial.Identity.Session.CreatedAt) != 7*24*time.Hour {
		t.Fatal("previous lifetime changed")
	}
}
