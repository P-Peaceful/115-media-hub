// library.go：存取媒体库 JSON 配置，兼容缺失的已有文件策略，并以修订号防止并发覆盖。

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

// libraryModel 保存媒体库配置快照及乐观并发修订号。
type libraryModel struct {
	ID         string `gorm:"primaryKey"`
	ConfigJSON string
	Revision   int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TableName 指定持久化表名 media_libraries，避免依赖 ORM 自动命名。
func (libraryModel) TableName() string { return "media_libraries" }

// libraryFromModel 解析保存的配置；仅为缺失的 STRM 策略补 skip，显式非法值作为损坏拒绝且不回写。
func libraryFromModel(m libraryModel) (library.Library, error) {
	var c library.Config
	if err := json.Unmarshal([]byte(m.ConfigJSON), &c); err != nil {
		return library.Library{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(m.ConfigJSON), &fields); err != nil {
		return library.Library{}, err
	}
	// 仅新引入且缺失的策略字段使用读取默认值；显式空值、null 或非法值
	// 都属于配置损坏，不能伪装成引入策略前的旧配置。
	if _, present := fields["strm_existing_file_policy"]; !present {
		c.STRMExistingFilePolicy = "skip"
	}
	if c.STRMExistingFilePolicy != "skip" && c.STRMExistingFilePolicy != "overwrite" {
		return library.Library{}, errors.New("invalid stored STRM existing-file policy")
	}
	return library.Library{Config: c, ID: m.ID, CreatedAt: m.CreatedAt.UTC(), UpdatedAt: m.UpdatedAt.UTC(), Revision: m.Revision}, nil
}

// modelFromLibrary 将媒体库配置编码为 JSON，保留身份、时间及当前修订号。
func modelFromLibrary(l library.Library) (libraryModel, error) {
	b, err := json.Marshal(l.Config)
	return libraryModel{ID: l.ID, ConfigJSON: string(b), CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt, Revision: l.Revision}, err
}

// ListLibraries 按 ID 升序读取游标后的有界媒体库列表，配置损坏时返回错误。
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

// GetLibrary 读取指定媒体库并解析配置，记录不存在时返回业务未找到错误。
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

// CreateLibrary 编码并插入媒体库配置，不执行任何媒体文件操作。
func (d *Database) CreateLibrary(ctx context.Context, l library.Library) error {
	m, err := modelFromLibrary(l)
	if err != nil {
		return err
	}
	return d.orm.WithContext(ctx).Create(&m).Error
}

// UpdateLibrary 按旧修订号条件更新并递增版本，零值也会写入；并发变更返回冲突。
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

// DeleteLibrary 在同一事务删除配置并保存审计，不删除网盘内容或本地媒体文件。
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
