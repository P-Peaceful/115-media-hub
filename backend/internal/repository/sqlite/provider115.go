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

type providerAccountModel struct {
	Singleton   int `gorm:"primaryKey"`
	AccountName string
	Cookie      string
	ConnectedAt time.Time
}

func (providerAccountModel) TableName() string { return "provider_115_account" }
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
func (d *Database) DeleteProviderAccount(ctx context.Context, actor, correlation string, now time.Time) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("singleton = ?", 1).Delete(&providerAccountModel{}).Error; err != nil {
			return err
		}
		return insertAudit(tx, domain.Audit{ActorID: actor, Action: "remove_115", TargetID: "115", Outcome: "succeeded", CorrelationID: correlation, OccurredAt: now})
	})
}

// ProviderCredential loads the provider credential for server-side requests only.
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
