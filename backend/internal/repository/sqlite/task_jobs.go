package sqlite

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
)

type taskJobModel struct {
	ID                  string `gorm:"primaryKey"`
	Type                jobs.Type
	LibraryID           string
	LibraryName         string
	Source              jobs.Source
	ScheduleID          *string
	ScheduledFor        *time.Time
	ParentID            *string
	Status              jobs.Status
	Outcome             *jobs.Outcome
	CreatedAt           time.Time
	StartedAt           *time.Time
	FinishedAt          *time.Time
	ProgressPercent     *float64
	SpeedBytesPerSecond *int64
	CurrentFile         *string
	TotalItems          *int64
	SucceededItems      int64
	FailedItems         int64
	SkippedItems        int64
	RequestID           string
	PendingAction       jobs.Action
	NeedsResume         bool
	Attempt             int
	RetryScope          jobs.RetryScope
	NotifyOn            []jobs.Outcome `gorm:"serializer:json"`
}

func (taskJobModel) TableName() string { return "task_jobs" }

func (m taskJobModel) domain() jobs.Job {
	return jobs.Job{
		ID: m.ID, Type: m.Type, LibraryID: m.LibraryID, LibraryName: m.LibraryName, Source: m.Source,
		ScheduleID: m.ScheduleID, ScheduledFor: m.ScheduledFor, ParentID: m.ParentID, Status: m.Status,
		Outcome: m.Outcome, CreatedAt: m.CreatedAt, StartedAt: m.StartedAt, FinishedAt: m.FinishedAt,
		ProgressPercent: m.ProgressPercent, SpeedBytesPerSecond: m.SpeedBytesPerSecond, CurrentFile: m.CurrentFile,
		TotalItems: m.TotalItems, SucceededItems: m.SucceededItems, FailedItems: m.FailedItems, SkippedItems: m.SkippedItems,
		RequestID: m.RequestID, PendingAction: m.PendingAction, NeedsResume: m.NeedsResume,
		Attempt: m.Attempt, RetryScope: m.RetryScope, NotifyOn: m.NotifyOn,
	}
}

func newTaskJobModel(job jobs.Job) taskJobModel {
	return taskJobModel{
		ID: job.ID, Type: job.Type, LibraryID: job.LibraryID, LibraryName: job.LibraryName, Source: job.Source,
		ScheduleID: job.ScheduleID, ScheduledFor: job.ScheduledFor, ParentID: job.ParentID, Status: job.Status,
		CreatedAt: job.CreatedAt, RequestID: job.RequestID, RetryScope: job.RetryScope,
		NotifyOn: append([]jobs.Outcome{}, job.NotifyOn...),
	}
}

func (d *Database) QueryJobs(ctx context.Context, filter jobs.JobFilter) (jobs.JobPage, error) {
	result := jobs.JobPage{Items: []jobs.Job{}, ByType: map[jobs.Type]int64{}, ByStatus: map[jobs.Status]int64{}}
	for _, kind := range jobs.Types() {
		result.ByType[kind] = 0
	}
	for _, status := range jobs.Statuses() {
		result.ByStatus[status] = 0
	}
	err := d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := taskJobQuery(tx, filter.LibraryID, filter.Type, filter.Status).Count(&result.Total).Error; err != nil {
			return err
		}
		var typeCounts []struct {
			Type  jobs.Type
			Count int64
		}
		if err := taskJobQuery(tx, filter.LibraryID, "", filter.Status).
			Select("type, count(*) AS count").Group("type").Scan(&typeCounts).Error; err != nil {
			return err
		}
		for _, count := range typeCounts {
			result.ByType[count.Type] = count.Count
		}
		var statusCounts []struct {
			Status jobs.Status
			Count  int64
		}
		if err := taskJobQuery(tx, filter.LibraryID, filter.Type, "").
			Select("status, count(*) AS count").Group("status").Scan(&statusCounts).Error; err != nil {
			return err
		}
		for _, count := range statusCounts {
			result.ByStatus[count.Status] = count.Count
		}
		query := taskJobQuery(tx, filter.LibraryID, filter.Type, filter.Status)
		if filter.Cursor != "" {
			var cursor taskJobModel
			if err := tx.Select("id", "created_at").Where("id = ?", filter.Cursor).Take(&cursor).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return jobs.ErrInvalid
				}
				return err
			}
			query = query.Where("(created_at, id) < (?, ?)", cursor.CreatedAt, cursor.ID)
		}
		var rows []taskJobModel
		if err := query.Order("created_at DESC, id DESC").Limit(filter.Limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > filter.Limit {
			rows = rows[:filter.Limit]
			result.NextCursor = &rows[len(rows)-1].ID
		}
		for _, row := range rows {
			result.Items = append(result.Items, row.domain())
		}
		return nil
	})
	return result, err
}

func taskJobQuery(db *gorm.DB, libraryID string, kind jobs.Type, status jobs.Status) *gorm.DB {
	query := db.Model(&taskJobModel{})
	if libraryID != "" {
		query = query.Where("library_id = ?", libraryID)
	}
	if kind != "" {
		query = query.Where("type = ?", kind)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	return query
}

func readTaskJob(db *gorm.DB, id string) (taskJobModel, error) {
	var model taskJobModel
	err := db.Where("id = ?", id).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model, jobs.ErrNotFound
	}
	return model, err
}

func (d *Database) ReadJob(ctx context.Context, id string) (jobs.Job, error) {
	model, err := readTaskJob(d.orm.WithContext(ctx), id)
	return model.domain(), err
}

func (d *Database) InsertJob(ctx context.Context, job jobs.Job) error {
	err := d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		model := newTaskJobModel(job)
		if err := tx.Create(&model).Error; err != nil {
			return err
		}
		return addTaskLog(tx, job.ID, "info", "任务已加入队列", job.CreatedAt)
	})
	var dbErr sqlite3.Error
	if errors.As(err, &dbErr) && dbErr.ExtendedCode == sqlite3.ErrConstraintUnique {
		return jobs.ErrConflict
	}
	return err
}

func (d *Database) ClaimJob(ctx context.Context, now time.Time) (jobs.Job, error) {
	var model taskJobModel
	err := d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("status = 'queued'").
			Where("NOT EXISTS (SELECT 1 FROM task_jobs AS active WHERE active.library_id = task_jobs.library_id AND active.status = 'running')").
			Order("created_at ASC, id ASC").Take(&model).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return jobs.ErrNotFound
		}
		if err != nil {
			return err
		}
		model.Status = jobs.Running
		model.Attempt++
		if model.StartedAt == nil {
			model.StartedAt = &now
		}
		result := tx.Model(&taskJobModel{}).Where("id = ? AND status = 'queued'", model.ID).
			Updates(map[string]any{"status": jobs.Running, "started_at": model.StartedAt, "attempt": model.Attempt})
		if err := changedOne(result); err != nil {
			return err
		}
		return addTaskLog(tx, model.ID, "info", "任务开始执行", now)
	})
	return model.domain(), err
}

func (d *Database) RunningJobs(ctx context.Context) ([]jobs.Job, error) {
	var rows []taskJobModel
	if err := d.orm.WithContext(ctx).Where("status = 'running'").Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	runs := make([]jobs.Job, 0, len(rows))
	for _, row := range rows {
		runs = append(runs, row.domain())
	}
	return runs, nil
}

func (d *Database) ChangeJobControl(ctx context.Context, job jobs.Job, action jobs.Action, now time.Time) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{}
		message := ""
		switch action {
		case jobs.Cancel:
			message = "已请求取消任务"
			if job.Status == jobs.Running {
				updates["pending_action"] = jobs.Cancel
			} else {
				message = "任务已取消"
				updates["status"] = jobs.Canceled
				updates["finished_at"] = now
			}
		case jobs.Pause:
			message = "已请求暂停任务"
			updates["pending_action"] = jobs.Pause
		case jobs.Resume:
			message = "任务恢复排队"
			updates["status"] = jobs.Queued
		}
		result := tx.Model(&taskJobModel{}).
			Where("id = ? AND status = ? AND pending_action = ? AND attempt = ?", job.ID, job.Status, job.PendingAction, job.Attempt).
			Updates(updates)
		if err := changedOne(result); err != nil {
			return err
		}
		return addTaskLog(tx, job.ID, "info", message, now)
	})
}

func (d *Database) FinishJob(ctx context.Context, snapshot jobs.Job, status jobs.Status, code string, now time.Time) (jobs.Job, error) {
	var model taskJobModel
	err := d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := readTaskJob(tx, snapshot.ID)
		if err != nil {
			return err
		}
		if current.Status != jobs.Running || current.PendingAction != snapshot.PendingAction || current.Attempt != snapshot.Attempt {
			return jobs.ErrConflict
		}
		model = current
		updates, level, message := terminalUpdates(current, status, code, now)
		result := tx.Model(&taskJobModel{}).Where("id = ?", snapshot.ID).Updates(updates)
		if err := changedOne(result); err != nil {
			return err
		}
		if err := addTaskLog(tx, snapshot.ID, level, message, now); err != nil {
			return err
		}
		model, err = readTaskJob(tx, snapshot.ID)
		return err
	})
	return model.domain(), err
}

func terminalUpdates(current taskJobModel, status jobs.Status, code string, now time.Time) (map[string]any, string, string) {
	if status == jobs.Completed && current.FailedItems > 0 && current.SucceededItems == 0 && current.SkippedItems == 0 {
		status = jobs.Failed
	}
	updates := map[string]any{"status": status, "pending_action": "", "needs_resume": status == jobs.Paused || status == jobs.Queued}
	level, message := "info", "任务已结束"
	switch status {
	case jobs.Queued:
		message = "重启核验通过，任务已重新排队"
	case jobs.Paused:
		message = "任务已暂停"
	case jobs.Canceled:
		updates["finished_at"] = now
		message = "任务已取消"
	case jobs.Failed:
		updates["finished_at"], updates["outcome"] = now, jobs.Failure
		level, message = "error", failureMessage(code)
	case jobs.Completed:
		updates["finished_at"], updates["outcome"] = now, jobs.Success
		message = "任务已完成"
		if current.FailedItems > 0 {
			updates["outcome"] = jobs.PartialFailure
			level, message = "warn", "任务已完成，部分文件处理失败"
		}
	}
	if status != jobs.Queued {
		updates["speed_bytes_per_second"], updates["current_file"] = nil, nil
	}
	return updates, level, message
}

func failureMessage(code string) string {
	switch code {
	case "task_type_unavailable":
		return "业务模块尚未接入任务中心"
	case "library_unavailable", "restart_library_unavailable":
		return "目标媒体库已不存在、停用或不支持此任务"
	case "restart_verification_unavailable":
		return "任务执行中断，业务模块未提供重启核验能力"
	case "restart_verification_failed":
		return "任务执行中断，业务模块未能核验执行结果"
	case "load_library_failed":
		return "无法读取目标媒体库"
	case "storage_unavailable":
		return "无法读取任务数据"
	case "module_execution_failed":
		return "业务模块执行失败"
	default:
		return "任务文件处理失败"
	}
}

func changedOne(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return jobs.ErrConflict
	}
	return nil
}

func (d *Database) UpdateJobProgress(ctx context.Context, id string, attempt int, progress jobs.Progress) error {
	return changedOne(d.orm.WithContext(ctx).Model(&taskJobModel{}).Where("id = ? AND status = 'running' AND attempt = ?", id, attempt).
		Updates(map[string]any{"progress_percent": progress.ProgressPercent, "speed_bytes_per_second": progress.SpeedBytesPerSecond,
			"current_file": progress.CurrentFile, "total_items": progress.TotalItems}))
}

type taskItemModel struct {
	ID      string `gorm:"primaryKey"`
	JobID   string
	ItemKey string
	Path    string
	Reason  string
	Status  jobs.ItemStatus
}

func (taskItemModel) TableName() string { return "task_items" }

func (d *Database) UpsertJobItem(ctx context.Context, id string, attempt int, item jobs.Item) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		job, err := readTaskJob(tx, id)
		if err != nil {
			return err
		}
		if job.Status != jobs.Running || job.Attempt != attempt {
			return jobs.ErrConflict
		}
		var previous taskItemModel
		err = tx.Where("job_id = ? AND item_key = ?", id, item.Key).Take(&previous).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		model := taskItemModel{ID: item.ID, JobID: id, ItemKey: item.Key, Path: item.Path, Reason: item.Reason, Status: item.Status}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "job_id"}, {Name: "item_key"}}, DoUpdates: clause.AssignmentColumns([]string{"path", "reason", "status"}),
		}).Create(&model).Error; err != nil {
			return err
		}
		if previous.Status == item.Status {
			return nil
		}
		update := map[string]any{}
		if previous.Status != "" {
			column := taskItemCountColumn(previous.Status)
			update[column] = gorm.Expr(column + " - 1")
		}
		column := taskItemCountColumn(item.Status)
		update[column] = gorm.Expr(column + " + 1")
		return changedOne(tx.Model(&taskJobModel{}).Where("id = ?", id).Updates(update))
	})
}

func taskItemCountColumn(status jobs.ItemStatus) string {
	switch status {
	case jobs.ItemSucceeded:
		return "succeeded_items"
	case jobs.ItemFailed:
		return "failed_items"
	default:
		return "skipped_items"
	}
}

func (d *Database) QueryFailedItems(ctx context.Context, id string, page jobs.Page) (jobs.ItemPage, error) {
	var rows []taskItemModel
	err := d.orm.WithContext(ctx).Where("job_id = ? AND status = 'failed' AND id > ?", id, page.Cursor).
		Order("id ASC").Limit(page.Limit + 1).Find(&rows).Error
	result := jobs.ItemPage{Items: []jobs.Item{}}
	if err != nil {
		return result, err
	}
	if len(rows) > page.Limit {
		rows = rows[:page.Limit]
		result.NextCursor = &rows[len(rows)-1].ID
	}
	for _, model := range rows {
		result.Items = append(result.Items, jobs.Item{ID: model.ID, Key: model.ItemKey, Path: model.Path, Reason: model.Reason, Status: model.Status})
	}
	return result, nil
}

type taskLogModel struct {
	ID        string `gorm:"primaryKey"`
	JobID     string
	Level     string
	Message   string
	CreatedAt time.Time
}

func (taskLogModel) TableName() string { return "task_logs" }

func addTaskLog(tx *gorm.DB, id, level, message string, now time.Time) error {
	return tx.Create(&taskLogModel{ID: rand.Text(), JobID: id, Level: level, Message: message, CreatedAt: now}).Error
}

func (d *Database) InsertJobLog(ctx context.Context, id string, attempt int, item jobs.JobLog) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`INSERT INTO task_logs (id, job_id, level, message, created_at)
			SELECT ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM task_jobs WHERE id = ? AND status = 'running' AND attempt = ?)
			AND (SELECT COUNT(*) FROM task_logs WHERE job_id = ?) < 5000`, item.ID, id, item.Level, item.Message, item.CreatedAt, id, attempt, id)
		if result.Error != nil || result.RowsAffected == 1 {
			return result.Error
		}
		// A current attempt can exceed its optional detail-log quota. Lifecycle
		// logs still use addTaskLog and remain outside that quota.
		job, err := readTaskJob(tx, id)
		if err != nil {
			return err
		}
		if job.Status != jobs.Running || job.Attempt != attempt {
			return jobs.ErrConflict
		}
		return nil
	})
}

func (d *Database) QueryJobLogs(ctx context.Context, id string, page jobs.Page) (jobs.LogPage, error) {
	var rows []taskLogModel
	db := d.orm.WithContext(ctx)
	query := db.Where("job_id = ?", id)
	if page.Cursor != "" {
		var cursor taskLogModel
		err := db.Select("id", "created_at").Where("id = ? AND job_id = ?", page.Cursor, id).Take(&cursor).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return jobs.LogPage{}, jobs.ErrInvalid
		}
		if err != nil {
			return jobs.LogPage{}, err
		}
		query = query.Where("(created_at, id) > (?, ?)", cursor.CreatedAt, cursor.ID)
	}
	err := query.Order("created_at ASC, id ASC").Limit(page.Limit + 1).Find(&rows).Error
	result := jobs.LogPage{Items: []jobs.JobLog{}}
	if err != nil {
		return result, err
	}
	if len(rows) > page.Limit {
		rows = rows[:page.Limit]
		result.NextCursor = &rows[len(rows)-1].ID
	}
	for _, model := range rows {
		result.Items = append(result.Items, jobs.JobLog{ID: model.ID, Level: model.Level, Message: model.Message, CreatedAt: model.CreatedAt})
	}
	return result, nil
}
