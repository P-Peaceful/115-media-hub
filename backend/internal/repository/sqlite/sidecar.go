// sidecar.go：持久化附属下载意图，以唯一目标和完整快照条件更新保护并发恢复；不执行网络或文件操作。

package sqlite

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/sidecar"
)

// sidecarIntentModel 映射独立下载意图表，创建使用已验证的非零时间，推进通过 UpdateColumns 保留指定时间。
type sidecarIntentModel struct {
	sidecar.Intent `gorm:"embedded"`
}

// TableName 指定下载意图表名，避免 ORM 命名策略改变恢复合同。
func (sidecarIntentModel) TableName() string { return "sidecar_download_intents" }

// sidecarIntentModelFrom 将所有持久化时间归一化 UTC，不改变调用方持有的快照。
func sidecarIntentModelFrom(i sidecar.Intent) sidecarIntentModel {
	i.CreatedAt = i.CreatedAt.UTC()
	i.UpdatedAt = i.UpdatedAt.UTC()
	return sidecarIntentModel{Intent: i}
}

// ReserveSidecarIntent 为新下载原子保留唯一目标，重复目标或 ID 返回归属冲突，不接管旧意图。
func (d *Database) ReserveSidecarIntent(ctx context.Context, intent sidecar.Intent) error {
	if intent.Validate() != nil || intent.State != sidecar.StateReserved || intent.Revision != 1 {
		return sidecar.ErrConflict
	}
	model := sidecarIntentModelFrom(intent)
	result := d.orm.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return sidecar.ErrConflict
	}
	return nil
}

// SidecarIntentByTarget 读取目标的恢复快照，校验数据库记录后返回 UTC 时间；缺失返回领域错误。
func (d *Database) SidecarIntentByTarget(ctx context.Context, target string) (sidecar.Intent, error) {
	var row sidecarIntentModel
	if err := d.orm.WithContext(ctx).Where("target_path = ?", target).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sidecar.Intent{}, sidecar.ErrNotFound
		}
		return sidecar.Intent{}, err
	}
	row = sidecarIntentModelFrom(row.Intent)
	return row.Intent, row.Validate()
}

// SidecarIntentsByLibrary 按目标顺序读取本库待恢复意图，任一损坏记录都会阻止不确定的恢复。
func (d *Database) SidecarIntentsByLibrary(ctx context.Context, libraryID string) ([]sidecar.Intent, error) {
	var rows []sidecarIntentModel
	if err := d.orm.WithContext(ctx).Where("library_id = ?", libraryID).Order("target_path ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]sidecar.Intent, 0, len(rows))
	for _, row := range rows {
		row = sidecarIntentModelFrom(row.Intent)
		if err := row.Validate(); err != nil {
			return nil, err
		}
		result = append(result, row.Intent)
	}
	return result, nil
}

// sidecarSnapshot 条件包含零值和 NULL，防止伪造旧阶段、陈旧快照或另一任务删除当前归属。
func sidecarSnapshot(i sidecar.Intent) map[string]any {
	i = sidecarIntentModelFrom(i).Intent
	return map[string]any{
		"id": i.ID, "job_id": i.JobID, "attempt": i.Attempt,
		"library_id": i.LibraryID, "account_id": i.AccountID, "source_file_id": i.SourceFileID,
		"source_directory_id": i.SourceDirectoryID, "kind": i.Kind,
		"output_root": i.OutputRoot, "relative_path": i.RelativePath, "target_path": i.TargetPath,
		"temporary_name": i.TemporaryName, "temporary_device": i.TemporaryDevice, "temporary_inode": i.TemporaryInode,
		"state": i.State, "expected_bytes": i.ExpectedBytes, "completed_bytes": i.CompletedBytes,
		"revision": i.Revision, "created_at": i.CreatedAt, "updated_at": i.UpdatedAt,
	}
}

// AdvanceSidecarIntent 只推进相邻阶段，以完整旧快照做单条条件更新，不持有跨文件动作的事务。
func (d *Database) AdvanceSidecarIntent(ctx context.Context, before, after sidecar.Intent) error {
	if err := before.CanAdvance(after); err != nil {
		return err
	}
	result := d.orm.WithContext(ctx).Model(&sidecarIntentModel{}).Where(sidecarSnapshot(before)).
		UpdateColumns(map[string]any{
			"temporary_name": after.TemporaryName, "temporary_device": after.TemporaryDevice,
			"temporary_inode": after.TemporaryInode, "state": after.State,
			"completed_bytes": after.CompletedBytes, "revision": after.Revision, "updated_at": after.UpdatedAt.UTC(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return sidecar.ErrConflict
	}
	return nil
}

// DiscardSidecarIntent 只移除已核对的完整快照；不删除文件，也不能清除被其他流程推进的意图。
func (d *Database) DiscardSidecarIntent(ctx context.Context, intent sidecar.Intent) error {
	if err := intent.Validate(); err != nil {
		return err
	}
	result := d.orm.WithContext(ctx).Where(sidecarSnapshot(intent)).Delete(&sidecarIntentModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return sidecar.ErrConflict
	}
	return nil
}
