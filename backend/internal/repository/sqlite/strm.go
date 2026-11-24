package sqlite

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/strm"
)

type strmArtifactModel strm.Artifact

func (strmArtifactModel) TableName() string { return "strm_artifacts" }

type strmIntentModel struct {
	TargetPath      string `gorm:"primaryKey"`
	LibraryID       string
	ArtifactJSON    string
	PreviousContent *string
	Legacy          bool
}

func (strmIntentModel) TableName() string { return "strm_write_intents" }

func (m strmIntentModel) intent() (strm.WriteIntent, error) {
	result := strm.WriteIntent{PreviousContent: m.PreviousContent, Legacy: m.Legacy}
	if err := json.Unmarshal([]byte(m.ArtifactJSON), &result.Artifact); err != nil {
		return strm.WriteIntent{}, err
	}
	if result.Artifact.TargetPath != m.TargetPath || result.Artifact.LibraryID != m.LibraryID {
		return strm.WriteIntent{}, strm.ErrConflict
	}
	return result, nil
}

func (d *Database) StrmArtifactByTarget(ctx context.Context, target string) (strm.Artifact, error) {
	var row strmArtifactModel
	err := d.orm.WithContext(ctx).Where("target_path = ?", target).Take(&row).Error
	return strm.Artifact(row), strmReadError(err)
}

func strmReadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return strm.ErrNotFound
	}
	return err
}

func (d *Database) StrmIntentByTarget(ctx context.Context, target string) (strm.WriteIntent, error) {
	var row strmIntentModel
	if err := d.orm.WithContext(ctx).Where("target_path = ?", target).Take(&row).Error; err != nil {
		return strm.WriteIntent{}, strmReadError(err)
	}
	return row.intent()
}

func (d *Database) StrmIntentsByLibrary(ctx context.Context, libraryID string) ([]strm.WriteIntent, error) {
	var rows []strmIntentModel
	if err := d.orm.WithContext(ctx).Where("library_id = ?", libraryID).Order("target_path").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]strm.WriteIntent, 0, len(rows))
	for _, row := range rows {
		intent, err := row.intent()
		if err != nil {
			return nil, err
		}
		result = append(result, intent)
	}
	return result, nil
}

// SaveStrmIntent 先保留目标路径的归属，文件发布由事务外的服务完成。
func (d *Database) SaveStrmIntent(ctx context.Context, intent strm.WriteIntent) error {
	artifact := intent.Artifact
	if intent.Legacy || artifact.PlaybackURL == "" {
		return strm.ErrConflict
	}
	body, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	model := strmIntentModel{
		TargetPath: artifact.TargetPath, LibraryID: artifact.LibraryID,
		ArtifactJSON: string(body), PreviousContent: intent.PreviousContent,
	}
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current strmArtifactModel
		err := tx.Where("target_path = ?", artifact.TargetPath).Take(&current).Error
		switch {
		case err == nil:
			if current.ID != artifact.ID || current.LibraryID != artifact.LibraryID || current.AccountID != artifact.AccountID ||
				current.SourceFileID != artifact.SourceFileID || current.PlaybackToken != artifact.PlaybackToken {
				return strm.ErrConflict
			}
			if intent.PreviousContent != nil && current.PlaybackURL != "" && *intent.PreviousContent != current.PlaybackURL+"\n" {
				return strm.ErrConflict
			}
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		case intent.PreviousContent != nil:
			return strm.ErrConflict
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return strm.ErrConflict
		}
		return nil
	})
}

// CompleteStrmIntent 将产物登记和移除写入意图放进同一个事务。
func (d *Database) CompleteStrmIntent(ctx context.Context, intent strm.WriteIntent) error {
	artifact := intent.Artifact
	if artifact.PlaybackURL == "" {
		return strm.ErrConflict
	}
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row strmIntentModel
		if err := tx.Where("target_path = ?", artifact.TargetPath).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return strm.ErrConflict
			}
			return err
		}
		stored, err := row.intent()
		if err != nil {
			return err
		}
		if !matchesStrmIntentArtifact(stored, intent) {
			return strm.ErrConflict
		}
		result := tx.Where("target_path = ? AND artifact_json = ? AND previous_content IS ? AND legacy = ?",
			artifact.TargetPath, row.ArtifactJSON, intent.PreviousContent, intent.Legacy).Delete(&strmIntentModel{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return strm.ErrConflict
		}
		model := strmArtifactModel(artifact)
		return tx.Omit("LibraryName").Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"source_file_id", "playback_code", "source_path", "source_name", "output_root", "relative_path", "playback_url", "size_bytes", "generated_at"}),
		}).Create(&model).Error
	})
}

func matchesStrmIntentArtifact(stored, completed strm.WriteIntent) bool {
	if stored.Legacy != completed.Legacy {
		return false
	}
	// 迁移过来的意图可在磁盘验证成功后补上播放地址，其他登记字段必须一致。
	if stored.Legacy && stored.Artifact.PlaybackURL == "" {
		stored.Artifact.PlaybackURL = completed.Artifact.PlaybackURL
	}
	stored.Artifact.GeneratedAt = stored.Artifact.GeneratedAt.UTC()
	completed.Artifact.GeneratedAt = completed.Artifact.GeneratedAt.UTC()
	completed.Artifact.LibraryName = nil
	return stored.Artifact == completed.Artifact
}

func (d *Database) DiscardStrmIntent(ctx context.Context, target string) error {
	return d.orm.WithContext(ctx).Where("target_path = ?", target).Delete(&strmIntentModel{}).Error
}

func (d *Database) strmWithLibrary(ctx context.Context) *gorm.DB {
	return d.orm.WithContext(ctx).Model(&strmArtifactModel{}).
		Select("strm_artifacts.*, json_extract(media_libraries.config_json, '$.name') AS library_name").
		Joins("LEFT JOIN media_libraries ON media_libraries.id = strm_artifacts.library_id")
}

func (d *Database) StrmArtifact(ctx context.Context, id string) (strm.Artifact, error) {
	var row strmArtifactModel
	err := d.strmWithLibrary(ctx).Where("strm_artifacts.id = ?", id).Take(&row).Error
	return strm.Artifact(row), strmReadError(err)
}

func (d *Database) ListStrmArtifacts(ctx context.Context, filter strm.ListFilter) (strm.ArtifactPage, error) {
	result := strm.ArtifactPage{Items: []strm.Artifact{}, Page: filter.Page, Limit: filter.Limit}
	query := func(db *gorm.DB) *gorm.DB {
		if filter.LibraryID != "" {
			db = db.Where("strm_artifacts.library_id = ?", filter.LibraryID)
		}
		if filter.Query != "" {
			db = db.Where("instr(lower(strm_artifacts.relative_path), lower(?)) > 0", filter.Query)
		}
		return db
	}
	sort := "generated_at"
	if filter.Sort == "name" {
		sort = "relative_path"
	}
	desc := filter.Order != "asc"
	var rows []strmArtifactModel
	// 计数和分页读取同一快照，生成中的新增产物不会让两者不一致。
	err := d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := query(tx.Model(&strmArtifactModel{})).Count(&result.Total).Error; err != nil {
			return err
		}
		return query(tx.Model(&strmArtifactModel{}).
			Select("strm_artifacts.*, json_extract(media_libraries.config_json, '$.name') AS library_name").
			Joins("LEFT JOIN media_libraries ON media_libraries.id = strm_artifacts.library_id")).
			Order(clause.OrderByColumn{Column: clause.Column{Table: "strm_artifacts", Name: sort}, Desc: desc}).
			Order(clause.OrderByColumn{Column: clause.Column{Table: "strm_artifacts", Name: "id"}, Desc: desc}).
			Limit(filter.Limit).Offset((filter.Page - 1) * filter.Limit).Find(&rows).Error
	})
	if err != nil {
		return strm.ArtifactPage{}, err
	}
	for _, row := range rows {
		result.Items = append(result.Items, strm.Artifact(row))
	}
	return result, nil
}
