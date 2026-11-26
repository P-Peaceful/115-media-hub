// sidecar_test.go：用真实 SQLite 验证下载意图的迁移、唯一保留、快照推进和删除保护，不模拟 SQL 语义。

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/sidecar"
	"github.com/wenzhengcheng/115-media-hub/backend/migrations"
)

// sidecarTestIntent 生成带本地时区的合法初始意图，确保仓储实际执行 UTC 往返而非只比较零时间。
func sidecarTestIntent(target string) sidecar.Intent {
	now := time.Date(2026, 11, 26, 8, 30, 0, 123456789, time.FixedZone("test", 8*60*60))
	expected := int64(4)
	return sidecar.Intent{
		ID: "intent", JobID: "job", Attempt: 2, LibraryID: "library", AccountID: "account",
		SourceFileID: "10", SourceDirectoryID: "20", Kind: sidecar.KindNFO,
		OutputRoot: filepath.Dir(target), RelativePath: filepath.Base(target), TargetPath: target,
		State: sidecar.StateReserved, ExpectedBytes: &expected, Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
}

// sidecarTestAdvance 构造相邻阶段的文件事实，所有推进都使用调用者持有的同一身份快照。
func sidecarTestAdvance(i sidecar.Intent, state sidecar.State) sidecar.Intent {
	i.State = state
	i.Revision++
	i.UpdatedAt = i.UpdatedAt.Add(time.Second)
	i.TemporaryName, i.TemporaryDevice, i.TemporaryInode = ".sidecar-owned", "2051", "12345"
	if state == sidecar.StateStaged || state == sidecar.StatePublished {
		i.CompletedBytes = *i.ExpectedBytes
	}
	return i
}

// TestSidecarIntentLifecycleAndSnapshotGuards 验证完整生命周期、UTC 持久化、陈旧快照和伪造旧状态不能推进或删除。
func TestSidecarIntentLifecycleAndSnapshotGuards(t *testing.T) {
	ctx := t.Context()
	db := taskDatabase(t)
	initial := sidecarTestIntent(filepath.Join(t.TempDir(), "movie.nfo"))
	initial.TemporaryName = ".sidecar-owned"
	if err := db.ReserveSidecarIntent(ctx, initial); err != nil {
		t.Fatal(err)
	}
	got, err := db.SidecarIntentByTarget(ctx, initial.TargetPath)
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(initial.CreatedAt) || !got.UpdatedAt.Equal(initial.UpdatedAt) ||
		got.CreatedAt.Location() != time.UTC || got.UpdatedAt.Location() != time.UTC {
		t.Fatalf("times lost UTC instant: %+v", got)
	}
	staging := sidecarTestAdvance(got, sidecar.StateStaging)
	if err := db.AdvanceSidecarIntent(ctx, got, staging); err != nil {
		t.Fatal(err)
	}
	if err := db.AdvanceSidecarIntent(ctx, got, staging); !errors.Is(err, sidecar.ErrConflict) {
		t.Fatalf("stale advance accepted: %v", err)
	}
	if err := db.DiscardSidecarIntent(ctx, got); !errors.Is(err, sidecar.ErrConflict) {
		t.Fatalf("stale discard accepted: %v", err)
	}
	// 即使版本号取自当前行，伪造 reserved 的旧快照仍不能跳过已有临时文件事实。
	forged := got
	forged.Revision = staging.Revision
	forged.UpdatedAt = staging.UpdatedAt
	if err := db.AdvanceSidecarIntent(ctx, forged, sidecarTestAdvance(forged, sidecar.StateStaging)); !errors.Is(err, sidecar.ErrConflict) {
		t.Fatalf("forged state accepted: %v", err)
	}
	staged := sidecarTestAdvance(staging, sidecar.StateStaged)
	if err := db.AdvanceSidecarIntent(ctx, staging, staged); err != nil {
		t.Fatal(err)
	}
	published := sidecarTestAdvance(staged, sidecar.StatePublished)
	if err := db.AdvanceSidecarIntent(ctx, staged, published); err != nil {
		t.Fatal(err)
	}
	got, err = db.SidecarIntentByTarget(ctx, initial.TargetPath)
	if err != nil || got.State != sidecar.StatePublished || got.CompletedBytes != 4 || got.Revision != 4 {
		t.Fatalf("published snapshot = %+v, %v", got, err)
	}
	if err := db.DiscardSidecarIntent(ctx, got); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SidecarIntentByTarget(ctx, initial.TargetPath); !errors.Is(err, sidecar.ErrNotFound) {
		t.Fatalf("discarded intent still found: %v", err)
	}
}

// TestSidecarIntentsKeepLibraryAndSourceIdentity 验证库配置删除不级联意图，旧任务不能更改任一不可变来源字段。
func TestSidecarIntentsKeepLibraryAndSourceIdentity(t *testing.T) {
	ctx := t.Context()
	db := taskDatabase(t)
	initial := sidecarTestIntent(filepath.Join(t.TempDir(), "movie.nfo"))
	if err := db.ReserveSidecarIntent(ctx, initial); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*sidecar.Intent){
		"id":        func(i *sidecar.Intent) { i.ID = "other" },
		"job":       func(i *sidecar.Intent) { i.JobID = "other" },
		"attempt":   func(i *sidecar.Intent) { i.Attempt++ },
		"library":   func(i *sidecar.Intent) { i.LibraryID = "other" },
		"account":   func(i *sidecar.Intent) { i.AccountID = "other" },
		"source":    func(i *sidecar.Intent) { i.SourceFileID = "other" },
		"directory": func(i *sidecar.Intent) { i.SourceDirectoryID = "other" },
		"kind":      func(i *sidecar.Intent) { i.Kind = sidecar.KindImage },
		"path": func(i *sidecar.Intent) {
			i.RelativePath = "other.nfo"
			i.TargetPath = filepath.Join(i.OutputRoot, i.RelativePath)
		},
		"root": func(i *sidecar.Intent) {
			i.OutputRoot = filepath.Join(i.OutputRoot, "other")
			i.TargetPath = filepath.Join(i.OutputRoot, i.RelativePath)
		},
		"expected": func(i *sidecar.Intent) { n := int64(5); i.ExpectedBytes = &n },
		"created":  func(i *sidecar.Intent) { i.CreatedAt = i.CreatedAt.Add(-time.Second) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			next := sidecarTestAdvance(initial, sidecar.StateStaging)
			mutate(&next)
			if err := db.AdvanceSidecarIntent(ctx, initial, next); !errors.Is(err, sidecar.ErrConflict) {
				t.Fatalf("source mutation accepted: %v", err)
			}
			forged := initial
			mutate(&forged)
			if err := db.DiscardSidecarIntent(ctx, forged); !errors.Is(err, sidecar.ErrConflict) {
				t.Fatalf("source mutation discarded original: %v", err)
			}
		})
	}
	if _, err := db.sql.ExecContext(ctx, `INSERT INTO media_libraries
		(id, config_json, revision, created_at, updated_at) VALUES (?, '{}', 1, ?, ?)`, initial.LibraryID, initial.CreatedAt, initial.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteLibrary(ctx, initial.LibraryID, "administrator", "test-request"); err != nil {
		t.Fatal(err)
	}
	rows, err := db.SidecarIntentsByLibrary(ctx, initial.LibraryID)
	if err != nil || len(rows) != 1 || rows[0].ID != initial.ID {
		t.Fatalf("configuration deletion lost recovery intent: %+v %v", rows, err)
	}
}

// TestSidecarIntentConcurrentTargetsAndReopen 验证两个真实连接并发争用相同目标仅一方成功，重开后归属保留。
func TestSidecarIntentConcurrentTargetsAndReopen(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "app.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
		if err := second.Close(); err != nil {
			t.Error(err)
		}
	})
	initial := sidecarTestIntent(filepath.Join(t.TempDir(), "movie.nfo"))
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for n, db := range []*Database{first, second} {
		wg.Go(func() {
			<-start
			i := initial
			if n == 1 {
				i.ID, i.LibraryID, i.JobID = "competitor", "another-library", "another-job"
			}
			results <- db.ReserveSidecarIntent(ctx, i)
		})
	}
	close(start)
	wg.Wait()
	close(results)
	succeeded, conflicted := 0, 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, sidecar.ErrConflict):
			conflicted++
		default:
			t.Fatal(err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("claims: success=%d conflict=%d", succeeded, conflicted)
	}
	owner, err := first.SidecarIntentByTarget(ctx, initial.TargetPath)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	}()
	got, err := reopened.SidecarIntentByTarget(ctx, initial.TargetPath)
	if err != nil || !reflect.DeepEqual(got, owner) {
		t.Fatalf("reopen changed intent: %+v %v", got, err)
	}
	// 清理后新一轮可以保留相同目标，但上一轮快照不能删除它。
	if err := first.DiscardSidecarIntent(ctx, owner); err != nil {
		t.Fatal(err)
	}
	replacement := initial
	replacement.ID, replacement.JobID = "replacement", "replacement-job"
	if err := second.ReserveSidecarIntent(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	if err := first.DiscardSidecarIntent(ctx, owner); !errors.Is(err, sidecar.ErrConflict) {
		t.Fatalf("previous owner deleted replacement: %v", err)
	}
}

// TestSidecarMigrationUpgradeAndGuardedRollback 验证版本 7 升级、非空回滚保护、空表回滚与重复升级均真实可用。
func TestSidecarMigrationUpgradeAndGuardedRollback(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "version-7.db")
	pool, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	}()
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.ExecContext(ctx, `INSERT INTO media_libraries
		(id, config_json, revision, created_at, updated_at) VALUES ('saved', '{}', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	initial := sidecarTestIntent(filepath.Join(t.TempDir(), "movie.nfo"))
	if err := db.ReserveSidecarIntent(ctx, initial); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 7); err == nil || !strings.Contains(err.Error(), "sidecar_intents_down_requires_consistent_backup") {
		t.Fatalf("unsafe rollback result: %v", err)
	}
	got, err := db.SidecarIntentByTarget(ctx, initial.TargetPath)
	if err != nil || got.ID != initial.ID {
		t.Fatalf("rejected rollback lost data: %+v %v", got, err)
	}
	if err := db.DiscardSidecarIntent(ctx, got); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 7); err != nil {
		t.Fatal(err)
	}
	var tables, kept int
	if err := pool.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE name = 'sidecar_download_intents'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("empty rollback table=%d %v", tables, err)
	}
	if err := pool.QueryRowContext(ctx, `SELECT count(*) FROM media_libraries WHERE id = 'saved'`).Scan(&kept); err != nil || kept != 1 {
		t.Fatalf("upgrade/rollback changed configuration: %d %v", kept, err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.ReserveSidecarIntent(ctx, initial); err != nil {
		t.Fatalf("re-upgrade reserve: %v", err)
	}
}

// TestSidecarIntentUnknownSizeAndCanceledOperations 验证未知大小可完整暂存且取消不会产生持久化副作用。
func TestSidecarIntentUnknownSizeAndCanceledOperations(t *testing.T) {
	ctx := t.Context()
	db := taskDatabase(t)
	initial := sidecarTestIntent(filepath.Join(t.TempDir(), "movie.nfo"))
	initial.ExpectedBytes = nil
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := db.ReserveSidecarIntent(canceled, initial); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled reserve = %v", err)
	}
	if _, err := db.SidecarIntentByTarget(ctx, initial.TargetPath); !errors.Is(err, sidecar.ErrNotFound) {
		t.Fatalf("canceled reserve wrote row: %v", err)
	}
	if err := db.ReserveSidecarIntent(ctx, initial); err != nil {
		t.Fatal(err)
	}
	staging := sidecarTestAdvance(initial, sidecar.StateStaging)
	if err := db.AdvanceSidecarIntent(canceled, initial, staging); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled advance = %v", err)
	}
	if err := db.AdvanceSidecarIntent(ctx, initial, staging); err != nil {
		t.Fatal(err)
	}
	staged := staging
	staged.State, staged.CompletedBytes, staged.Revision = sidecar.StateStaged, 7, staging.Revision+1
	if err := db.AdvanceSidecarIntent(ctx, staging, staged); err != nil {
		t.Fatal(err)
	}
	if err := db.DiscardSidecarIntent(canceled, staged); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled discard = %v", err)
	}
	rows, err := db.SidecarIntentsByLibrary(ctx, initial.LibraryID)
	if err != nil || len(rows) != 1 || rows[0].CompletedBytes != 7 || rows[0].ExpectedBytes != nil {
		t.Fatalf("unknown size snapshot: %+v %v", rows, err)
	}
}

// TestSidecarIntentDatabaseChecksAndReadValidation 验证数据库拒绝非法阶段/长度，读取也拒绝无法由 SQL 表达的路径损坏。
func TestSidecarIntentDatabaseChecksAndReadValidation(t *testing.T) {
	ctx := t.Context()
	db := taskDatabase(t)
	initial := sidecarTestIntent(filepath.Join(t.TempDir(), "movie.nfo"))
	if err := db.ReserveSidecarIntent(ctx, initial); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{
		"attempt = 0", "kind = 'subtitle'", "expected_bytes = -1", "expected_bytes = 8388609",
		"completed_bytes = 1", "state = 'staged'", "state = 'unknown'", "revision = 0",
		"state = 'staging', temporary_name = '.tmp', temporary_device = '1'",
	} {
		if _, err := db.sql.ExecContext(ctx, "UPDATE sidecar_download_intents SET "+mutation+" WHERE id = ?", initial.ID); err == nil {
			t.Fatalf("database accepted invalid state: %s", mutation)
		}
	}
	if _, err := db.sql.ExecContext(ctx, `UPDATE sidecar_download_intents SET relative_path = '../movie.nfo' WHERE id = ?`, initial.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SidecarIntentByTarget(ctx, initial.TargetPath); !errors.Is(err, sidecar.ErrConflict) {
		t.Fatalf("read accepted corrupted path: %v", err)
	}
	if _, err := db.SidecarIntentsByLibrary(ctx, initial.LibraryID); !errors.Is(err, sidecar.ErrConflict) {
		t.Fatalf("recovery list accepted corrupted path: %v", err)
	}
}
