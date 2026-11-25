// auth.go：持久化管理员、系统设置、会话和安全审计，使用事务保护凭据代次及安全变更。

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

// administratorModel 映射唯一管理员及其密码哈希，Singleton 由数据库约束为 1。
type administratorModel struct {
	Singleton    int
	ID           string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName 指定持久化表名 administrators，避免依赖 ORM 自动命名。
func (administratorModel) TableName() string { return "administrators" }

// settingsModel 映射首次设置和会话期限，与管理员同为单例配置。
type settingsModel struct {
	Singleton           int
	ServiceURL          string
	OutputDirectory     string
	SessionLifetimeDays int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// TableName 指定持久化表名 system_settings，避免依赖 ORM 自动命名。
func (settingsModel) TableName() string { return "system_settings" }

// sessionModel 仅存储会话及 CSRF 摘要和固定到期时间，不保存原始凭证。
type sessionModel struct {
	TokenHash       string `gorm:"primaryKey"`
	AdministratorID string
	CSRFHash        string
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

// TableName 指定持久化表名 admin_sessions，避免依赖 ORM 自动命名。
func (sessionModel) TableName() string { return "admin_sessions" }

// auditModel 映射安全事件及关联标识，参与需要审计的安全变更事务。
type auditModel struct {
	ActorID       string
	Action        string
	TargetID      string
	Outcome       string
	CorrelationID string
	OccurredAt    time.Time
}

// TableName 指定持久化表名 audit_records，避免依赖 ORM 自动命名。
func (auditModel) TableName() string { return "audit_records" }

// Administrator 读取唯一管理员，缺失时返回首次初始化状态。
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

// Settings 读取单例业务设置，将存储失败包装为安全的操作诊断。
func (d *Database) Settings(ctx context.Context) (domain.Settings, error) {
	var model settingsModel
	if err := d.orm.WithContext(ctx).Where("singleton = ?", 1).Take(&model).Error; err != nil {
		return domain.Settings{}, authStorageError("load_settings", err)
	}
	return domain.Settings{ServiceURL: model.ServiceURL, OutputDirectory: model.OutputDirectory, SessionLifetimeDays: model.SessionLifetimeDays}, nil
}

// FindSession 读取未过期会话及公开管理员身份，不返回密码哈希。
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

// Initialize 原子保存凭据、设置、首个会话与审计，单例约束冲突返回已初始化错误。
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

// CreateSession 在事务内复核已验证的密码代次，再轮换旧会话、有界清理过期会话并保存审计。
func (d *Database) CreateSession(ctx context.Context, session domain.Session, oldHash, expectedPasswordHash string, audit domain.Audit) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 密码重置可能发生在比较成功后、创建会话前；在写事务中复核
		// 已验证的密码代次，避免旧密码登录重新产生有效会话。
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
		// 认证请求只清理有界数量的过期会话，避免无界扫描和删除。
		if err := tx.Exec("DELETE FROM admin_sessions WHERE token_hash IN (SELECT token_hash FROM admin_sessions WHERE expires_at <= ? LIMIT 100)", session.CreatedAt).Error; err != nil {
			return fmt.Errorf("clean expired sessions: %w", err)
		}
		if err := insertSession(tx, session); err != nil {
			return err
		}
		return insertAudit(tx, audit)
	})
}

// RevokeSession 撤销指定会话；审计保存失败时回滚撤销操作。
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

// ResetPassword 更新现有管理员密码并撤销全部会话，与审计原子提交且保留设置。
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

// RecordAudit 保存独立的失败登录审计，失败必须由服务处理。
func (d *Database) RecordAudit(ctx context.Context, audit domain.Audit) error {
	return insertAudit(d.orm.WithContext(ctx), audit)
}

// insertSession 使用传入事务保存令牌摘要、CSRF 摘要和固定期限。
func insertSession(tx *gorm.DB, session domain.Session) error {
	model := sessionModel{TokenHash: session.TokenHash, AdministratorID: session.AdministratorID, CSRFHash: session.CSRFHash, CreatedAt: session.CreatedAt, ExpiresAt: session.ExpiresAt}
	if err := tx.Create(&model).Error; err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// insertAudit 使用传入事务保存安全审计，失败时附加必须审计不可用的错误分类。
func insertAudit(tx *gorm.DB, audit domain.Audit) error {
	model := auditModel{ActorID: audit.ActorID, Action: audit.Action, TargetID: audit.TargetID, Outcome: audit.Outcome, CorrelationID: audit.CorrelationID, OccurredAt: audit.OccurredAt}
	if err := tx.Create(&model).Error; err != nil {
		return fmt.Errorf("persist security audit: %w", errors.Join(auth.ErrAuditUnavailable, err))
	}
	return nil
}

// UpdateCredentials 事务内验证当前会话和密码代次，更新凭据、撤销其他会话并记录审计。
func (d *Database) UpdateCredentials(ctx context.Context, username, hash, expectedHash, currentSession string, audit domain.Audit) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session sessionModel
		if err := tx.Where("token_hash = ? AND expires_at > ?", currentSession, audit.OccurredAt).Take(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return auth.ErrUnauthenticated
			}
			return err
		}
		result := tx.Model(&administratorModel{}).Where("singleton = ? AND password_hash = ?", 1, expectedHash).Updates(map[string]any{"username": username, "password_hash": hash, "updated_at": audit.OccurredAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return auth.ErrInvalidCredentials
		}
		if err := tx.Where("token_hash <> ?", currentSession).Delete(&sessionModel{}).Error; err != nil {
			return err
		}
		return insertAudit(tx, audit)
	})
}

// UpdateSessionLifetime 原子更新单例会话期限与审计；尚未初始化时不创建设置。
func (d *Database) UpdateSessionLifetime(ctx context.Context, days int, audit domain.Audit) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&settingsModel{}).Where("singleton = ?", 1).Updates(map[string]any{"session_lifetime_days": days, "updated_at": audit.OccurredAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return auth.ErrSetupRequired
		}
		return insertAudit(tx, audit)
	})
}
