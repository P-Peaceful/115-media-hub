// provider115.go：保存单个 115 账号授权及审计，区分公开账号状态与仅服务端可读的 Cookie。

package sqlite

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

// providerAccountModel 映射唯一 115 账号；Cookie 按当前产品约定原样存储，不能用于公开响应或日志。
type providerAccountModel struct {
	Singleton   int `gorm:"primaryKey"`
	AccountName string
	Cookie      string
	ConnectedAt time.Time
}

// TableName 指定持久化表名 provider_115_account，避免依赖 ORM 自动命名。
func (providerAccountModel) TableName() string { return "provider_115_account" }

// ProviderAccount 读取不含 Cookie 的账号状态，未授权时返回空状态。
func (d *Database) ProviderAccount(ctx context.Context) (provider115.Account, error) {
	var model providerAccountModel
	err := d.orm.WithContext(ctx).Where("singleton = ?", 1).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return provider115.Account{}, nil
	}
	if err != nil {
		return provider115.Account{}, err
	}
	return provider115.Account{Connected: true, AccountName: &model.AccountName, ConnectedAt: &model.ConnectedAt}, nil
}

// SaveProviderAccount 事务内确认发起授权的会话仍有效，再保存单例账号与授权审计。
func (d *Database) SaveProviderAccount(ctx context.Context, name, cookie string, now time.Time, actor, correlation, sessionHash string) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session sessionModel
		if err := tx.Where("token_hash = ? AND expires_at > ?", sessionHash, now).Take(&session).Error; err != nil {
			return err
		}
		model := providerAccountModel{Singleton: 1, AccountName: name, Cookie: cookie, ConnectedAt: now}
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&model).Error; err != nil {
			return err
		}
		return insertAudit(tx, domain.Audit{ActorID: actor, Action: "authorize_115", TargetID: "115", Outcome: "succeeded", CorrelationID: correlation, OccurredAt: now})
	})
}

// DeleteProviderAccount 原子移除单例授权与记录审计，不操作网盘数据。
func (d *Database) DeleteProviderAccount(ctx context.Context, actor, correlation string, now time.Time) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("singleton = ?", 1).Delete(&providerAccountModel{}).Error; err != nil {
			return err
		}
		return insertAudit(tx, domain.Audit{ActorID: actor, Action: "remove_115", TargetID: "115", Outcome: "succeeded", CorrelationID: correlation, OccurredAt: now})
	})
}

// ProviderCredential 仅供服务端请求读取 Cookie，未授权时返回空字符串。
func (d *Database) ProviderCredential(ctx context.Context) (string, error) {
	var model providerAccountModel
	err := d.orm.WithContext(ctx).Select("cookie").Where("singleton = ?", 1).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return model.Cookie, nil
}
