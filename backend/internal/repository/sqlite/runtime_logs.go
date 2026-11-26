// runtime_logs.go：负责运行日志批量入库、历史分页、留存裁剪及与安全审计原子提交的清理。

package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/logs"
)

// 固定小数位和 UTC 使 SQLite 文本索引精确按纳秒排序。
const runtimeLogTimestamp = "2006-01-02T15:04:05.000000000Z"

type runtimeLogModel struct {
	ID          int64
	OccurredAt  string
	Level       string
	Module      string
	Operation   string
	JobID       string
	RequestID   string
	ErrorCode   string
	Message     string
	DetailsJSON string
}

// TableName 将模型绑定到迁移创建的运行日志表。
func (runtimeLogModel) TableName() string { return "runtime_logs" }

// InsertRuntimeLogs 批量保存已脱敏的日志并返回 SQLite 分配的持久化 ID。
func (d *Database) InsertRuntimeLogs(ctx context.Context, entries []logs.Entry) ([]logs.Entry, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	models := make([]runtimeLogModel, len(entries))
	for i, entry := range entries {
		details := "{}"
		if len(entry.Details) > 0 {
			encoded, err := json.Marshal(entry.Details)
			if err != nil {
				return nil, fmt.Errorf("encode runtime log details: %w", err)
			}
			details = string(encoded)
		}
		models[i] = runtimeLogModel{
			OccurredAt: entry.OccurredAt.UTC().Format(runtimeLogTimestamp),
			Level:      entry.Level, Module: entry.Module, Operation: entry.Operation,
			JobID: entry.JobID, RequestID: entry.RequestID, ErrorCode: entry.ErrorCode,
			Message: entry.Message, DetailsJSON: details,
		}
	}
	if err := d.orm.WithContext(ctx).CreateInBatches(&models, 100).Error; err != nil {
		return nil, fmt.Errorf("insert runtime logs: %w", err)
	}
	saved := make([]logs.Entry, len(entries))
	for i, entry := range entries {
		entry.ID = models[i].ID
		entry.OccurredAt = entry.OccurredAt.UTC()
		saved[i] = entry
	}
	return saved, nil
}

// ListRuntimeLogs 按时间和 ID 倒序读取一批记录，在返回前释放 SQLite 连接供业务写入。
func (d *Database) ListRuntimeLogs(ctx context.Context, filter logs.Filter) ([]logs.Entry, error) {
	if filter.Limit <= 0 {
		return nil, errors.New("runtime log query requires a positive limit")
	}
	query := runtimeLogQuery(d.orm.WithContext(ctx), filter)
	var rows []runtimeLogModel
	if err := query.Order("occurred_at DESC, id DESC").Limit(filter.Limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list runtime logs: %w", err)
	}
	entries := make([]logs.Entry, 0, len(rows))
	for _, row := range rows {
		entry, err := row.domain()
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// runtimeLogQuery 只用固定列和绑定参数构造筛选，关键词作为字面子串比较。
func runtimeLogQuery(db *gorm.DB, filter logs.Filter) *gorm.DB {
	query := db.Model(&runtimeLogModel{})
	if !filter.Since.IsZero() {
		query = query.Where("occurred_at >= ?", filter.Since.UTC().Format(runtimeLogTimestamp))
	}
	if filter.Level != "" {
		query = query.Where("level = ?", filter.Level)
	}
	if filter.Module != "" {
		query = query.Where("module = ?", filter.Module)
	}
	if filter.JobID != "" {
		query = query.Where("job_id != '' AND job_id = ?", filter.JobID)
	}
	if filter.Before.ID > 0 {
		query = query.Where("(occurred_at, id) < (?, ?)", filter.Before.OccurredAt.UTC().Format(runtimeLogTimestamp), filter.Before.ID)
	}
	if filter.MaxID > 0 {
		query = query.Where("id <= ?", filter.MaxID)
	}
	if filter.Query != "" {
		query = query.Where(`(instr(lower(message), lower(?)) > 0 OR instr(lower(operation), lower(?)) > 0
			OR instr(lower(error_code), lower(?)) > 0 OR instr(lower(job_id), lower(?)) > 0
			OR instr(lower(request_id), lower(?)) > 0)`, filter.Query, filter.Query, filter.Query, filter.Query, filter.Query)
	}
	return query
}

// domain 解码 UTC 时间及 JSON 详情，保留整数精度以便同样的数据可重新导出。
func (m runtimeLogModel) domain() (logs.Entry, error) {
	at, err := time.Parse(runtimeLogTimestamp, m.OccurredAt)
	if err != nil {
		return logs.Entry{}, fmt.Errorf("decode runtime log time: %w", err)
	}
	var details map[string]any
	decoder := json.NewDecoder(strings.NewReader(m.DetailsJSON))
	decoder.UseNumber()
	if err := decoder.Decode(&details); err != nil {
		return logs.Entry{}, fmt.Errorf("decode runtime log details: %w", err)
	}
	return logs.Entry{
		ID: m.ID, OccurredAt: at, Level: m.Level, Module: m.Module, Operation: m.Operation,
		JobID: m.JobID, RequestID: m.RequestID, ErrorCode: m.ErrorCode, Message: m.Message, Details: details,
	}, nil
}

// runtimeLogMaxID 在调用方事务中读取最大持久化 ID；空表返回 0。
func runtimeLogMaxID(db *gorm.DB) (int64, error) {
	var maxID int64
	if err := db.Raw("SELECT coalesce(max(id), 0) FROM runtime_logs").Row().Scan(&maxID); err != nil {
		return 0, fmt.Errorf("read runtime log upper bound: %w", err)
	}
	return maxID, nil
}

// RuntimeLogExportBoundary 在同一短读事务内确定导出的写入上界及是否超过输出限额。
func (d *Database) RuntimeLogExportBoundary(ctx context.Context, filter logs.Filter, limit int) (int64, bool, error) {
	if limit <= 0 {
		return 0, false, errors.New("runtime log export requires a positive limit")
	}
	var maxID int64
	var truncated bool
	err := d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		maxID, err = runtimeLogMaxID(tx)
		if err != nil || maxID == 0 {
			return err
		}
		if filter.MaxID == 0 || maxID < filter.MaxID {
			filter.MaxID = maxID
		}
		var extra runtimeLogModel
		result := runtimeLogQuery(tx, filter).Select("id").Order("occurred_at DESC, id DESC").Offset(limit).Limit(1).Find(&extra)
		if result.Error != nil {
			return fmt.Errorf("check runtime log export limit: %w", result.Error)
		}
		maxID = filter.MaxID
		truncated = result.RowsAffected == 1
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	return maxID, truncated, nil
}

// ClearRuntimeLogs 按严格早于指定时间清理日志；零时间清空，审计失败时事务整体回滚。
func (d *Database) ClearRuntimeLogs(ctx context.Context, before time.Time, audit auth.Audit) (int64, error) {
	var deleted int64
	err := d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("1 = 1")
		if !before.IsZero() {
			query = tx.Where("occurred_at < ?", before.UTC().Format(runtimeLogTimestamp))
		}
		result := query.Delete(&runtimeLogModel{})
		if result.Error != nil {
			return fmt.Errorf("clear runtime logs: %w", result.Error)
		}
		deleted = result.RowsAffected
		return insertAudit(tx, audit)
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

// PruneRuntimeLogs 优先删除过期行，再删除超出条数上限的最早写入行；单次事务删除不超过批量上限。
func (d *Database) PruneRuntimeLogs(ctx context.Context, cutoff time.Time, maxRows, batchLimit int) (int64, error) {
	if maxRows < 0 || batchLimit <= 0 {
		return 0, errors.New("invalid runtime log retention limits")
	}
	var deleted int64
	err := d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		expired := tx.Exec(`DELETE FROM runtime_logs WHERE id IN (
			SELECT id FROM runtime_logs WHERE occurred_at < ? ORDER BY occurred_at, id LIMIT ?
		)`, cutoff.UTC().Format(runtimeLogTimestamp), batchLimit)
		if expired.Error != nil {
			return fmt.Errorf("prune expired runtime logs: %w", expired.Error)
		}
		deleted = expired.RowsAffected
		if deleted == int64(batchLimit) {
			return nil
		}
		overflow := tx.Exec(`DELETE FROM runtime_logs WHERE id IN (
			SELECT id FROM runtime_logs WHERE id <= (SELECT id FROM runtime_logs ORDER BY id DESC LIMIT 1 OFFSET ?)
			ORDER BY id LIMIT ?
		)`, maxRows, int64(batchLimit)-deleted)
		if overflow.Error != nil {
			return fmt.Errorf("prune excess runtime logs: %w", overflow.Error)
		}
		deleted += overflow.RowsAffected
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}
