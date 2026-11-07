package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	driver "github.com/mattn/go-sqlite3"
	"gorm.io/gorm"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
)

type administratorModel struct {
	Singleton    int
	ID           string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (administratorModel) TableName() string { return "administrators" }

type settingsModel struct {
	Singleton           int
	ServiceURL          string
	OutputDirectory     string
	SessionLifetimeDays int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (settingsModel) TableName() string { return "system_settings" }

type sessionModel struct {
	TokenHash       string `gorm:"primaryKey"`
	AdministratorID string
	CSRFHash        string
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

func (sessionModel) TableName() string { return "admin_sessions" }

type auditModel struct {
	ActorID       string
	Action        string
	TargetID      string
	Outcome       string
	CorrelationID string
	OccurredAt    time.Time
}

func (auditModel) TableName() string { return "audit_records" }

// Administrator loads the one local administrator, distinguishing first-run state.
func (d *Database) Administrator(ctx context.Context) (domain.Administrator, error) {
	var model administratorModel
	if err := d.orm.WithContext(ctx).Where("singleton = ?", 1).Take(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Administrator{}, auth.ErrSetupRequired
		}
		return domain.Administrator{}, authStorageError("load_administrator", err)
	}
	return domain.Administrator{ID: model.ID, Username: model.Username, PasswordHash: model.PasswordHash, CreatedAt: model.CreatedAt}, nil
}

// Settings loads the first-run business settings independently of transport.
func (d *Database) Settings(ctx context.Context) (domain.Settings, error) {
	var model settingsModel
	if err := d.orm.WithContext(ctx).Where("singleton = ?", 1).Take(&model).Error; err != nil {
		return domain.Settings{}, authStorageError("load_settings", err)
	}
	return domain.Settings{ServiceURL: model.ServiceURL, OutputDirectory: model.OutputDirectory, SessionLifetimeDays: model.SessionLifetimeDays}, nil
}

// FindSession returns an unexpired session and a public administrator identity.
func (d *Database) FindSession(ctx context.Context, hash string, now time.Time) (domain.Identity, error) {
	var model sessionModel
	if err := d.orm.WithContext(ctx).Where("token_hash = ? AND expires_at > ?", hash, now).Take(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Identity{}, auth.ErrUnauthenticated
		}
		return domain.Identity{}, authStorageError("find_session", err)
	}
	var administrator administratorModel
	if err := d.orm.WithContext(ctx).Select("id", "username").Where("id = ?", model.AdministratorID).Take(&administrator).Error; err != nil {
		return domain.Identity{}, authStorageError("find_session_administrator", err)
	}
	return domain.Identity{Session: domain.Session{TokenHash: model.TokenHash, AdministratorID: model.AdministratorID, CSRFHash: model.CSRFHash, CreatedAt: model.CreatedAt, ExpiresAt: model.ExpiresAt}, Administrator: domain.Administrator{ID: administrator.ID, Username: administrator.Username}}, nil
}

// Initialize atomically persists credentials, settings, first session and mandatory audit.
func (d *Database) Initialize(ctx context.Context, administrator domain.Administrator, settings domain.Settings, session domain.Session, audit domain.Audit) error {
	err := d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		model := administratorModel{Singleton: 1, ID: administrator.ID, Username: administrator.Username, PasswordHash: administrator.PasswordHash, CreatedAt: administrator.CreatedAt, UpdatedAt: administrator.CreatedAt}
		if err := tx.Create(&model).Error; err != nil {
			var sqliteErr driver.Error
			if errors.As(err, &sqliteErr) && sqliteErr.Code == driver.ErrConstraint {
				return auth.ErrAlreadyInitialized
			}
			return err
		}
		configuration := settingsModel{Singleton: 1, ServiceURL: settings.ServiceURL, OutputDirectory: settings.OutputDirectory, SessionLifetimeDays: settings.SessionLifetimeDays, CreatedAt: audit.OccurredAt, UpdatedAt: audit.OccurredAt}
		if err := tx.Create(&configuration).Error; err != nil {
			return err
		}
		if err := insertSession(tx, session); err != nil {
			return err
		}
		return insertAudit(tx, audit)
	})
	if err != nil {
		return fmt.Errorf("initialize administrator transaction: %w", err)
	}
	return nil
}

// CreateSession includes old-session rotation, bounded expiry cleanup and success audit.
func (d *Database) CreateSession(ctx context.Context, session domain.Session, oldHash, expectedPasswordHash string, audit domain.Audit) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A password reset may finish after comparison and before session creation.
		// Recheck the credential inside this write transaction to prevent resurrecting an old login.
		var administrator administratorModel
		if err := tx.Where("id = ? AND password_hash = ?", session.AdministratorID, expectedPasswordHash).Take(&administrator).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return auth.ErrInvalidCredentials
			}
			return fmt.Errorf("verify credential generation: %w", err)
		}

		if oldHash != "" {
			if err := tx.Where("token_hash = ?", oldHash).Delete(&sessionModel{}).Error; err != nil {
				return fmt.Errorf("rotate session: %w", err)
			}
		}
		// Never scan/delete an unbounded set during authentication.
		if err := tx.Exec("DELETE FROM admin_sessions WHERE token_hash IN (SELECT token_hash FROM admin_sessions WHERE expires_at <= ? LIMIT 100)", session.CreatedAt).Error; err != nil {
			return fmt.Errorf("clean expired sessions: %w", err)
		}
		if err := insertSession(tx, session); err != nil {
			return err
		}
		return insertAudit(tx, audit)
	})
}

// RevokeSession rolls back revocation if its required security audit cannot be stored.
func (d *Database) RevokeSession(ctx context.Context, hash string, audit domain.Audit) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("token_hash = ?", hash).Delete(&sessionModel{})
		if result.Error != nil {
			return fmt.Errorf("revoke session: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return auth.ErrUnauthenticated
		}
		return insertAudit(tx, audit)
	})
}

// ResetPassword preserves settings and atomically invalidates every prior session.
func (d *Database) ResetPassword(ctx context.Context, hash string, audit domain.Audit) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&administratorModel{}).Where("singleton = ?", 1).Updates(map[string]any{"password_hash": hash, "updated_at": audit.OccurredAt})
		if result.Error != nil {
			return fmt.Errorf("reset password: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return auth.ErrSetupRequired
		}
		if err := tx.Where("1 = 1").Delete(&sessionModel{}).Error; err != nil {
			return fmt.Errorf("revoke all sessions: %w", err)
		}
		return insertAudit(tx, audit)
	})
}

// RecordAudit stores a failed-login outcome; errors must be handled by the service.
func (d *Database) RecordAudit(ctx context.Context, audit domain.Audit) error {
	return insertAudit(d.orm.WithContext(ctx), audit)
}

func insertSession(tx *gorm.DB, session domain.Session) error {
	model := sessionModel{TokenHash: session.TokenHash, AdministratorID: session.AdministratorID, CSRFHash: session.CSRFHash, CreatedAt: session.CreatedAt, ExpiresAt: session.ExpiresAt}
	if err := tx.Create(&model).Error; err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

func insertAudit(tx *gorm.DB, audit domain.Audit) error {
	model := auditModel{ActorID: audit.ActorID, Action: audit.Action, TargetID: audit.TargetID, Outcome: audit.Outcome, CorrelationID: audit.CorrelationID, OccurredAt: audit.OccurredAt}
	if err := tx.Create(&model).Error; err != nil {
		return fmt.Errorf("persist security audit: %w", errors.Join(auth.ErrAuditUnavailable, err))
	}
	return nil
}
