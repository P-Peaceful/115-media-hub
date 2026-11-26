// runtime_logs_test.go：使用真实 SQLite 验证运行日志筛选、持久化、留存、原子清理和升级回滚。

package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	domainauth "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/logs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/migrations"
)

// TestRuntimeLogsFiltering 验证纳秒及重复时间排序、组合筛选、字面关键词和删行后仍可翻页。
func TestRuntimeLogsFiltering(t *testing.T) {
	db := taskDatabase(t)
	ctx := t.Context()
	at := time.Date(2026, 11, 27, 8, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	written, err := db.InsertRuntimeLogs(ctx, []logs.Entry{
		{OccurredAt: at, Level: "info", Module: "jobs", JobID: "job-1", Message: "影片 100%_完成"},
		{OccurredAt: at, Level: "info", Module: "jobs", JobID: "job-1", Operation: "mÄTcH", RequestID: "REQ.1", Message: "下载", Details: map[string]any{"done": true, "count": int64(9007199254740993)}},
		{OccurredAt: at.Add(time.Nanosecond), Level: "warn", Module: "jobs", JobID: "job-1", ErrorCode: "busy", Message: "影片 100xy完成"},
		{OccurredAt: at.Add(-time.Nanosecond), Level: "info", Module: "system", Message: "影片 100%_完成"},
		{OccurredAt: at.Add(time.Second), Level: "info", Module: "jobs", JobID: "job-2", Message: "different job"},
	})
	if err != nil || len(written) != 5 {
		t.Fatalf("insert logs: %+v %v", written, err)
	}
	for _, entry := range written {
		if entry.ID <= 0 || entry.OccurredAt.Location() != time.UTC {
			t.Fatalf("persisted entry lacks ID or UTC time: %+v", entry)
		}
	}
	tests := []struct {
		filter logs.Filter
		want   []int
	}{
		{filter: logs.Filter{Limit: 10}, want: []int{4, 2, 1, 0, 3}},
		{filter: logs.Filter{Since: at, Level: "info", Module: "jobs", JobID: "job-1", Limit: 10}, want: []int{1, 0}},
		{filter: logs.Filter{Since: at, Query: "100%_", Limit: 10}, want: []int{0}},
		{filter: logs.Filter{Query: "MÄtch", Limit: 10}, want: []int{1}},
		{filter: logs.Filter{Query: "mäTCH", Limit: 10}, want: []int{}},
		{filter: logs.Filter{Query: "q.1", Limit: 10}, want: []int{1}},
		{filter: logs.Filter{Query: "BuSY", Limit: 10}, want: []int{2}},
		{filter: logs.Filter{Query: "JOB-2", Limit: 10}, want: []int{4}},
		{filter: logs.Filter{Before: logs.Cursor{OccurredAt: at.Add(time.Nanosecond), ID: written[2].ID}, MaxID: written[1].ID, Limit: 10}, want: []int{1, 0}},
	}
	for _, test := range tests {
		got, err := db.ListRuntimeLogs(ctx, test.filter)
		if err != nil || len(got) != len(test.want) {
			t.Fatalf("filter %+v returned %+v: %v", test.filter, got, err)
		}
		for i, index := range test.want {
			entry := got[i]
			if entry.ID != written[index].ID || !entry.OccurredAt.Equal(written[index].OccurredAt) {
				t.Fatalf("filter %+v returned wrong order or time: %+v", test.filter, got)
			}
		}
		for i, entry := range written {
			if test.filter.Matches(entry) != slices.Contains(test.want, i) {
				t.Fatalf("live predicate disagrees with SQLite for filter %+v and entry %+v", test.filter, entry)
			}
		}
	}
	page, err := db.ListRuntimeLogs(ctx, logs.Filter{Since: at, Level: "info", JobID: "job-1", Limit: 1})
	if err != nil || len(page) != 1 || page[0].Details["done"] != true || page[0].Details["count"] != json.Number("9007199254740993") {
		t.Fatalf("first page lost structured details: %+v %v", page, err)
	}
	if _, err := db.sql.ExecContext(ctx, "DELETE FROM runtime_logs WHERE id = ?", page[0].ID); err != nil {
		t.Fatal(err)
	}
	cursor := logs.Cursor{OccurredAt: page[0].OccurredAt, ID: page[0].ID}
	page, err = db.ListRuntimeLogs(ctx, logs.Filter{Since: at, Level: "info", JobID: "job-1", Before: cursor, Limit: 1})
	if err != nil || len(page) != 1 || page[0].ID != written[0].ID {
		t.Fatalf("cursor depends on live anchor row: %+v %v", page, err)
	}
}

// TestRuntimeLogExportBoundary 验证截断计算使用筛选条件，随后入库的旧时间日志不会进入导出。
func TestRuntimeLogExportBoundary(t *testing.T) {
	db := taskDatabase(t)
	ctx := t.Context()
	at := time.Now().UTC()
	filter := logs.Filter{Level: "warn", Query: "%", Limit: 1}
	if maxID, truncated, err := db.RuntimeLogExportBoundary(ctx, filter, 1); err != nil || maxID != 0 || truncated {
		t.Fatalf("empty boundary: %d %t %v", maxID, truncated, err)
	}
	written, err := db.InsertRuntimeLogs(ctx, []logs.Entry{
		{OccurredAt: at.Add(time.Second), Level: "warn", Message: "% newer"},
		{OccurredAt: at, Level: "warn", Message: "% older"},
		{OccurredAt: at, Level: "info", Message: "unmatched"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if maxID, truncated, err := db.RuntimeLogExportBoundary(ctx, filter, 2); err != nil || maxID != written[2].ID || truncated {
		t.Fatalf("exact boundary: %d %t %v", maxID, truncated, err)
	}
	maxID, truncated, err := db.RuntimeLogExportBoundary(ctx, filter, 1)
	if err != nil || maxID != written[2].ID || !truncated {
		t.Fatalf("truncated boundary: %d %t %v", maxID, truncated, err)
	}
	if _, err := db.InsertRuntimeLogs(ctx, []logs.Entry{{OccurredAt: at.Add(-time.Second), Level: "warn", Message: "% backdated"}}); err != nil {
		t.Fatal(err)
	}
	filter.MaxID, filter.Limit = maxID, 10
	got, err := db.ListRuntimeLogs(ctx, filter)
	if err != nil || len(got) != 2 || got[0].ID != written[0].ID || got[1].ID != written[1].ID {
		t.Fatalf("anchored export: %+v %v", got, err)
	}
}

// TestRuntimeLogsClearAudit 验证审计失败撤销删除，成功清理仅影响指定范围的运行日志。
func TestRuntimeLogsClearAudit(t *testing.T) {
	db := taskDatabase(t)
	ctx := t.Context()
	at := time.Now().UTC()
	job := jobs.Job{ID: "kept", LibraryID: "lib", LibraryName: "Library", Type: jobs.Scan, Status: jobs.Queued, Source: jobs.Manual, CreatedAt: at}
	if err := db.InsertJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	audit := domainauth.Audit{ActorID: "admin", Action: "clear_runtime_logs", TargetID: "before_7d", Outcome: "succeeded", CorrelationID: "req-123", OccurredAt: at}
	prior := audit
	prior.Action = "previous_security_event"
	if err := db.RecordAudit(ctx, prior); err != nil {
		t.Fatal(err)
	}
	written, err := db.InsertRuntimeLogs(ctx, []logs.Entry{
		{OccurredAt: at.Add(-time.Nanosecond), Level: "info", JobID: job.ID},
		{OccurredAt: at, Level: "warn", JobID: job.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.ExecContext(ctx, `CREATE TRIGGER deny_runtime_clear_audit BEFORE INSERT ON audit_records
		WHEN NEW.action = 'clear_runtime_logs' BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	if deleted, err := db.ClearRuntimeLogs(ctx, at, audit); !errors.Is(err, auth.ErrAuditUnavailable) || deleted != 0 {
		t.Fatalf("failed audit: %d %v", deleted, err)
	}
	got, err := db.ListRuntimeLogs(ctx, logs.Filter{Limit: 10})
	if err != nil || len(got) != 2 {
		t.Fatalf("failed audit removed logs: %+v %v", got, err)
	}
	if _, err := db.sql.ExecContext(ctx, "DROP TRIGGER deny_runtime_clear_audit"); err != nil {
		t.Fatal(err)
	}
	if deleted, err := db.ClearRuntimeLogs(ctx, at, audit); err != nil || deleted != 1 {
		t.Fatalf("scoped clear: %d %v", deleted, err)
	}
	got, err = db.ListRuntimeLogs(ctx, logs.Filter{Limit: 10})
	if err != nil || len(got) != 1 || got[0].ID != written[1].ID {
		t.Fatalf("strict cutoff did not preserve matching timestamp: %+v %v", got, err)
	}
	audit.TargetID = "all"
	if deleted, err := db.ClearRuntimeLogs(ctx, time.Time{}, audit); err != nil || deleted != 1 {
		t.Fatalf("clear all: %d %v", deleted, err)
	}
	if got, err := db.ListRuntimeLogs(ctx, logs.Filter{Limit: 10}); err != nil || len(got) != 0 || got == nil {
		t.Fatalf("all logs should be removed with empty slice: %+v %v", got, err)
	}
	var auditCount int
	if err := db.sql.QueryRowContext(ctx, `SELECT count(*) FROM audit_records WHERE actor_id = ? AND correlation_id = ?`,
		audit.ActorID, audit.CorrelationID).Scan(&auditCount); err != nil || auditCount != 3 {
		t.Fatalf("audit count: %d %v", auditCount, err)
	}
	if _, err := db.ReadJob(ctx, job.ID); err != nil {
		t.Fatalf("clear removed a task: %v", err)
	}
	if page, err := db.QueryJobLogs(ctx, job.ID, jobs.Page{Limit: 10}); err != nil || len(page.Items) != 1 {
		t.Fatalf("clear removed task logs: %+v %v", page, err)
	}
}

// TestRuntimeLogsPruning 验证先处理过期记录，再按写入顺序限制总量，并在每次事务限制删除数。
func TestRuntimeLogsPruning(t *testing.T) {
	db := taskDatabase(t)
	ctx := t.Context()
	at := time.Now().UTC()
	written, err := db.InsertRuntimeLogs(ctx, []logs.Entry{
		{OccurredAt: at.Add(time.Second), Level: "info"},
		{OccurredAt: at.Add(-time.Second), Level: "info"},
		{OccurredAt: at, Level: "info"},
		{OccurredAt: at.Add(-time.Nanosecond), Level: "info"},
		{OccurredAt: at, Level: "info"},
		{OccurredAt: at.Add(time.Second), Level: "info"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if deleted, err := db.PruneRuntimeLogs(ctx, at, 2, 3); err != nil || deleted != 3 {
		t.Fatalf("first prune: %d %v", deleted, err)
	}
	got, err := db.ListRuntimeLogs(ctx, logs.Filter{Limit: 10})
	if err != nil || len(got) != 3 || got[0].ID != written[5].ID || got[1].ID != written[4].ID || got[2].ID != written[2].ID {
		t.Fatalf("retention priority and batch limit: %+v %v", got, err)
	}
	if deleted, err := db.PruneRuntimeLogs(ctx, at, 2, 3); err != nil || deleted != 1 {
		t.Fatalf("second prune: %d %v", deleted, err)
	}
	if deleted, err := db.PruneRuntimeLogs(ctx, at, 2, 3); err != nil || deleted != 0 {
		t.Fatalf("repeat prune: %d %v", deleted, err)
	}
}

// TestRuntimeLogsFailedBatchRollsBack 验证后续批次约束失败时已执行的插入也会回滚。
func TestRuntimeLogsFailedBatchRollsBack(t *testing.T) {
	db := taskDatabase(t)
	ctx := t.Context()
	entries := make([]logs.Entry, 101)
	for i := range entries {
		entries[i] = logs.Entry{OccurredAt: time.Now(), Level: "info"}
	}
	entries[100].Level = "unsupported"
	if got, err := db.InsertRuntimeLogs(ctx, entries); err == nil || len(got) != 0 {
		t.Fatalf("invalid batch result: %+v %v", got, err)
	}
	if got, err := db.ListRuntimeLogs(ctx, logs.Filter{Limit: 1}); err != nil || len(got) != 0 {
		t.Fatalf("failed batch left persisted rows: %+v %v", got, err)
	}
}

// TestRuntimeLogsMigrationUpgradeAndRollback 验证版本 8 数据可升级，重启保留日志，且只允许空表回滚。
func TestRuntimeLogsMigrationUpgradeAndRollback(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "v8.db")
	pool, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.ExecContext(ctx, `INSERT INTO audit_records
		(actor_id, action, target_id, outcome, correlation_id, occurred_at) VALUES ('admin', 'previous', '', 'succeeded', '', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	written, err := db.InsertRuntimeLogs(ctx, []logs.Entry{{OccurredAt: time.Now(), Level: "info", Message: "kept after restart"}})
	if err := errors.Join(err, db.Close()); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 8); err == nil || !strings.Contains(err.Error(), "runtime_logs_down_requires_consistent_backup") {
		t.Fatalf("unsafe rollback result: %v", err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	})
	got, err := reopened.ListRuntimeLogs(ctx, logs.Filter{Limit: 10})
	if err != nil || len(got) != 1 || got[0].ID != written[0].ID || got[0].Message != "kept after restart" {
		t.Fatalf("restart or rejected rollback lost logs: %+v %v", got, err)
	}
	if _, err := pool.ExecContext(ctx, "DELETE FROM runtime_logs"); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	var auditCount int
	if err := pool.QueryRowContext(ctx, "SELECT count(*) FROM audit_records WHERE action = 'previous'").Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("upgrade or rollback removed prior security audit: %d %v", auditCount, err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.ListRuntimeLogs(ctx, logs.Filter{Limit: 1}); err != nil || len(got) != 0 {
		t.Fatalf("re-upgraded runtime logs: %+v %v", got, err)
	}
}
