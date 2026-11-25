// task_schedules.go：持久化定时计划及修订号，将推进下次触发时间与批量任务入队放在同一事务。

package sqlite

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"gorm.io/gorm"

	auth "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
)

// taskScheduleModel 保存定时规则、时区、通知结果及修订号，集合字段使用 JSON 编码。
type taskScheduleModel struct {
	ID             string `gorm:"primaryKey"`
	Name           string
	Type           jobs.Type
	LibraryID      *string
	Enabled        bool
	Period         jobs.Period
	Timezone       string
	Time           *string
	Weekdays       []int `gorm:"serializer:json"`
	MonthDay       *int
	Cron           *string
	NotifyOn       []jobs.Outcome `gorm:"serializer:json"`
	NextRunAt      *time.Time
	LastSkipReason string
	Revision       int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TableName 指定持久化表名 task_schedules，避免依赖 ORM 自动命名。
func (taskScheduleModel) TableName() string { return "task_schedules" }

// domain 将持久化计划映射为领域规则、修订号和下次触发时间。
func (model taskScheduleModel) domain() jobs.Schedule {
	return jobs.Schedule{
		ScheduleInput: jobs.ScheduleInput{
			Name: model.Name, Type: model.Type, LibraryID: model.LibraryID, Enabled: model.Enabled, Period: model.Period,
			Timezone: model.Timezone, Time: model.Time, Weekdays: model.Weekdays, MonthDay: model.MonthDay, Cron: model.Cron, NotifyOn: model.NotifyOn,
		},
		ID: model.ID, NextRunAt: model.NextRunAt, Revision: model.Revision, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
	}
}

// newTaskScheduleModel 构造计划存储值，复制集合避免与调用方共享可变切片。
func newTaskScheduleModel(schedule jobs.Schedule) taskScheduleModel {
	return taskScheduleModel{
		ID: schedule.ID, Name: schedule.Name, Type: schedule.Type, LibraryID: schedule.LibraryID, Enabled: schedule.Enabled,
		Period: schedule.Period, Timezone: schedule.Timezone, Time: schedule.Time, Weekdays: append([]int{}, schedule.Weekdays...),
		MonthDay: schedule.MonthDay, Cron: schedule.Cron, NotifyOn: append([]jobs.Outcome{}, schedule.NotifyOn...),
		NextRunAt: schedule.NextRunAt, Revision: schedule.Revision, CreatedAt: schedule.CreatedAt, UpdatedAt: schedule.UpdatedAt,
	}
}

// QuerySchedules 按 ID 升序游标查询计划，可按类型、媒体库和启用状态筛选。
func (d *Database) QuerySchedules(ctx context.Context, filter jobs.ScheduleFilter) (jobs.SchedulePage, error) {
	query := d.orm.WithContext(ctx).Where("id > ?", filter.Cursor)
	if filter.Type != "" {
		query = query.Where("type = ?", filter.Type)
	}
	if filter.LibraryID != "" {
		query = query.Where("library_id = ?", filter.LibraryID)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	var rows []taskScheduleModel
	err := query.Order("id ASC").Limit(filter.Limit + 1).Find(&rows).Error
	result := jobs.SchedulePage{Items: []jobs.Schedule{}}
	if err != nil {
		return result, err
	}
	if len(rows) > filter.Limit {
		rows = rows[:filter.Limit]
		result.NextCursor = &rows[len(rows)-1].ID
	}
	for _, model := range rows {
		result.Items = append(result.Items, model.domain())
	}
	return result, nil
}

// ReadSchedule 按 ID 读取计划，不存在时返回任务业务未找到错误。
func (d *Database) ReadSchedule(ctx context.Context, id string) (jobs.Schedule, error) {
	var model taskScheduleModel
	err := d.orm.WithContext(ctx).Where("id = ?", id).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return jobs.Schedule{}, jobs.ErrNotFound
	}
	return model.domain(), err
}

// InsertSchedule 保存已由服务校验的新计划及其首次计算的触发时间。
func (d *Database) InsertSchedule(ctx context.Context, plan jobs.Schedule) error {
	model := newTaskScheduleModel(plan)
	return d.orm.WithContext(ctx).Create(&model).Error
}

// SaveSchedule 以修订号条件更新计划并递增版本，显式写入禁用状态及空触发时间。
func (d *Database) SaveSchedule(ctx context.Context, plan jobs.Schedule) error {
	model := newTaskScheduleModel(plan)
	model.Revision++
	return changedOne(d.orm.WithContext(ctx).Model(&taskScheduleModel{}).
		Where("id = ? AND revision = ?", plan.ID, plan.Revision).
		Select("name", "type", "library_id", "enabled", "period", "timezone", "time", "weekdays", "month_day",
			"cron", "notify_on", "next_run_at", "updated_at", "revision", "last_skip_reason").
		Updates(&model))
}

// RemoveSchedule 事务删除计划并保存审计，保留已有任务历史。
func (d *Database) RemoveSchedule(ctx context.Context, id, actorID, requestID string) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ?", id).Delete(&taskScheduleModel{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return jobs.ErrNotFound
		}
		return insertAudit(tx, auth.Audit{ActorID: actorID, Action: "delete_task_schedule", TargetID: id, Outcome: "succeeded",
			CorrelationID: requestID, OccurredAt: time.Now().UTC()})
	})
}

// DueSchedules 按触发时间和 ID 读取最多 100 个到期且启用的计划。
func (d *Database) DueSchedules(ctx context.Context, now time.Time) ([]jobs.Schedule, error) {
	var rows []taskScheduleModel
	err := d.orm.WithContext(ctx).Where("enabled AND next_run_at <= ?", now).
		Order("next_run_at, id").Limit(100).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	plans := make([]jobs.Schedule, 0, len(rows))
	for _, row := range rows {
		plans = append(plans, row.domain())
	}
	return plans, nil
}

// AdvanceSchedule 条件推进当前触发点并原子批量入队及记日志；任何任务插入失败均回滚推进。
func (d *Database) AdvanceSchedule(ctx context.Context, plan jobs.Schedule, next *time.Time, reason string, runs []jobs.Job) error {
	return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&taskScheduleModel{}).Where("id = ? AND enabled AND revision = ? AND next_run_at = ?", plan.ID, plan.Revision, plan.NextRunAt).
			Updates(map[string]any{"next_run_at": next, "last_skip_reason": reason, "enabled": next != nil, "updated_at": plan.UpdatedAt})
		if err := changedOne(result); err != nil {
			return err
		}
		if len(runs) == 0 {
			return nil
		}
		models := make([]taskJobModel, 0, len(runs))
		logs := make([]taskLogModel, 0, len(runs))
		for _, run := range runs {
			models = append(models, newTaskJobModel(run))
			logs = append(logs, taskLogModel{ID: rand.Text(), JobID: run.ID, Level: "info", Message: "定时任务已加入队列", CreatedAt: run.CreatedAt})
		}
		// 同一触发点冲突说明触发快照已过期，推进时间也必须回滚。
		if err := tx.CreateInBatches(&models, 100).Error; err != nil {
			return err
		}
		return tx.CreateInBatches(&logs, 100).Error
	})
}
