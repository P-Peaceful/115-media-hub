package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
)

type libraryModel struct {
	ID         string `gorm:"primaryKey"`
	ConfigJSON string
	Revision   int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (libraryModel) TableName() string { return "media_libraries" }
func libraryFromModel(m libraryModel) (library.Library, error) {
	var c library.Config
	if err := json.Unmarshal([]byte(m.ConfigJSON), &c); err != nil {
		return library.Library{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(m.ConfigJSON), &fields); err != nil {
		return library.Library{}, err
	}
	// Only the newly introduced policy has a persisted-read default. Explicit
	// empty/null/invalid values are corruption, not pre-policy configuration.
	if _, present := fields["strm_existing_file_policy"]; !present {
		c.STRMExistingFilePolicy = "skip"
	}
	if c.STRMExistingFilePolicy != "skip" && c.STRMExistingFilePolicy != "overwrite" {
		return library.Library{}, errors.New("invalid stored STRM existing-file policy")
	}
	return library.Library{Config: c, ID: m.ID, CreatedAt: m.CreatedAt.UTC(), UpdatedAt: m.UpdatedAt.UTC(), Revision: m.Revision}, nil
}
func modelFromLibrary(l library.Library) (libraryModel, error) {
	b, err := json.Marshal(l.Config)
	return libraryModel{ID: l.ID, ConfigJSON: string(b), CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt, Revision: l.Revision}, err
}
func (d *Database) ListLibraries(ctx context.Context, cursor string, limit int) ([]library.Library, error) {
	var models []libraryModel
	if err := d.orm.WithContext(ctx).Where("id > ?", cursor).Order("id ASC").Limit(limit).Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]library.Library, 0, len(models))
	for _, m := range models {
		item, err := libraryFromModel(m)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}
func (d *Database) GetLibrary(ctx context.Context, id string) (library.Library, error) {
	var m libraryModel
	err := d.orm.WithContext(ctx).Where("id = ?", id).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return library.Library{}, library.ErrNotFound
	}
	if err != nil {
		return library.Library{}, err
	}
	return libraryFromModel(m)
}
func (d *Database) CreateLibrary(ctx context.Context, l library.Library) error {
	m, err := modelFromLibrary(l)
	if err != nil {
		return err
	}
	return d.orm.WithContext(ctx).Create(&m).Error
}
func (d *Database) UpdateLibrary(ctx context.Context, l library.Library) error {
	m, err := modelFromLibrary(l)
	if err != nil {
		return err
	}
	result := d.orm.WithContext(ctx).Model(&libraryModel{}).Where("id = ? AND revision = ?", l.ID, l.Revision).Updates(map[string]any{"config_json": m.ConfigJSON, "updated_at": m.UpdatedAt, "revision": gorm.Expr("revision + 1")})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return library.ErrConflict
	}
	return nil
}
func (d *Database) DeleteLibrary(ctx context.Context, id, actor, request string) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ?", id).Delete(&libraryModel{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return library.ErrNotFound
		}
		return insertAudit(tx, domain.Audit{ActorID: actor, Action: "delete_library", TargetID: id, Outcome: "succeeded", CorrelationID: request, OccurredAt: time.Now().UTC()})
	})
}
