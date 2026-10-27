package sqlite

import (
	"context"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite/model"
	"gorm.io/gorm"
)

type ConnectionRepository struct{ DB *gorm.DB }

func (r ConnectionRepository) Active115(ctx context.Context) (*model.Connection, error) {
	var c model.Connection
	err := r.DB.WithContext(ctx).Where("kind = ? AND is_active = ?", "115", true).Take(&c).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &c, err
}
func (r ConnectionRepository) Save(ctx context.Context, c *model.Connection) error {
	return r.DB.WithContext(ctx).Save(c).Error
}

func (r ConnectionRepository) Connection115(ctx context.Context, id string) (*model.Connection, error) {
	var c model.Connection
	err := r.DB.WithContext(ctx).Where("kind = ? AND is_active = ? AND id = ?", "115", true, id).Take(&c).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &c, err
}

func (r ConnectionRepository) UpdateCheck(ctx context.Context, id, status, capabilities string, checkedAt time.Time) error {
	return r.DB.WithContext(ctx).Model(&model.Connection{}).Where("id = ? AND kind = ? AND is_active = ?", id, "115", true).Updates(map[string]any{"status": status, "capabilities_json": capabilities, "updated_at": checkedAt}).Error
}
