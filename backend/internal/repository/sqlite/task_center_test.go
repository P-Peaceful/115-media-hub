package sqlite

import (
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/migrations"
)

func TestTaskCenterMigrationPreservesExistingLibraries(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "previous.db")
	pool, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, migrations.Files)
	if err != nil {
		t.Error(err)
	} else if _, err := provider.UpTo(ctx, 4); err != nil {
		t.Error(err)
	} else if _, err := pool.ExecContext(ctx, `INSERT INTO media_libraries (id, config_json, revision, created_at, updated_at)
		VALUES ('preexisting', '{"name":"Old Library"}', 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Error(err)
	}
	if err := pool.Close(); err != nil {
		t.Error(err)
	}
	if t.Failed() {
		return
	}
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	record, err := db.GetLibrary(ctx, "preexisting")
	if err != nil || record.Name != "Old Library" || record.Revision != 3 {
		t.Fatalf("migration lost the previous library: %+v %v", record, err)
	}
	page, err := db.QueryJobs(ctx, jobs.JobFilter{Page: jobs.Page{Limit: 2}})
	if err != nil || page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("fresh task tables should be empty: %+v %v", page, err)
	}
}

func taskDatabase(t *testing.T) *Database {
	t.Helper()
	db, err := Open(t.Context(), filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func TestTaskCenterDowngradeRejectsHistoryLoss(t *testing.T) {
	ctx := t.Context()
	db := taskDatabase(t)
	job := jobs.Job{
		ID: "retained-job", LibraryID: "library", LibraryName: "Library", Type: jobs.Scan,
		Status: jobs.Queued, Source: jobs.Manual, CreatedAt: time.Now().UTC(),
	}
	if err := db.InsertJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 4); err == nil || !strings.Contains(err.Error(), "task_center_down_is_irreversible_restore_a_consistent_backup") {
		t.Fatalf("downgrade did not report the intentional rejection: %v", err)
	}
	version, err := provider.GetDBVersion(ctx)
	if err != nil || version != 5 {
		t.Fatalf("failed downgrade changed the migration version: %d %v", version, err)
	}
	if _, err := db.ReadJob(ctx, job.ID); err != nil {
		t.Fatalf("failed downgrade lost task history: %v", err)
	}
	logPage, err := db.QueryJobLogs(ctx, job.ID, jobs.Page{Limit: 1})
	if err != nil || len(logPage.Items) != 1 {
		t.Fatalf("failed downgrade lost task logs: %+v %v", logPage, err)
	}
}

func TestConcurrentClaimSerializesSameLibraryAndPagesHistory(t *testing.T) {
	ctx := t.Context()
	db := taskDatabase(t)
	now := time.Date(2026, 11, 20, 10, 0, 0, 12345, time.UTC)
	for i, entry := range []struct {
		id, library string
		kind        jobs.Type
	}{
		{"one", "library-a", jobs.Scan}, {"two", "library-a", jobs.Scan}, {"three", "library-b", jobs.Sync},
	} {
		if err := db.InsertJob(ctx, jobs.Job{
			ID: entry.id, LibraryID: entry.library, LibraryName: entry.library, Type: entry.kind,
			Status: jobs.Queued, Source: jobs.Manual, CreatedAt: now.Add(time.Duration(i) * time.Nanosecond),
		}); err != nil {
			t.Fatal(err)
		}
	}
	claims := make(chan string, 8)
	launch := make(chan struct{})
	var workers sync.WaitGroup
	for range cap(claims) {
		workers.Go(func() {
			<-launch
			claim, err := db.ClaimJob(ctx, now)
			if err == nil {
				claims <- claim.ID
			} else if !errors.Is(err, jobs.ErrNotFound) {
				t.Error(err)
			}
		})
	}
	close(launch)
	workers.Wait()
	close(claims)
	var claimed []string
	for claim := range claims {
		claimed = append(claimed, claim)
	}
	slices.Sort(claimed)
	if !slices.Equal(claimed, []string{"one", "three"}) {
		t.Fatalf("claim lock or FIFO failed: %v", claimed)
	}
	filter := jobs.JobFilter{Page: jobs.Page{Limit: 1}}
	for i, want := range []string{"three", "two", "one"} {
		page, err := db.QueryJobs(ctx, filter)
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != want || page.Total != 3 {
			t.Fatalf("history page %d: %+v %v", i+1, page, err)
		}
		if i == 2 {
			if page.NextCursor != nil {
				t.Fatalf("unexpected next cursor at end: %q", *page.NextCursor)
			}
		} else {
			if page.NextCursor == nil {
				t.Fatalf("missing cursor at page %d", i+1)
			}
			filter.Cursor = *page.NextCursor
		}
	}
	active, err := db.ReadJob(ctx, "one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.FinishJob(ctx, active, jobs.Completed, "", now); err != nil {
		t.Fatal(err)
	}
	next, err := db.ClaimJob(ctx, now)
	if err != nil || next.ID != "two" {
		t.Fatalf("claim did not release the library: %+v %v", next, err)
	}
}

func TestScheduleAdvanceRollsBackPartialSubmission(t *testing.T) {
	ctx := t.Context()
	db := taskDatabase(t)
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	next := now.Add(24 * time.Hour)
	plan := jobs.Schedule{
		ID: "plan", Revision: 1, CreatedAt: now, UpdatedAt: now, NextRunAt: &now,
		ScheduleInput: jobs.ScheduleInput{Name: "Scan", Type: jobs.Scan, Enabled: true, Period: jobs.Daily, Timezone: "Asia/Shanghai"},
	}
	if err := db.InsertSchedule(ctx, plan); err != nil {
		t.Fatal(err)
	}
	job := jobs.Job{
		ID: "good", Type: jobs.Scan, LibraryID: "library", LibraryName: "Library", Source: jobs.Scheduled,
		Status: jobs.Queued, ScheduleID: &plan.ID, ScheduledFor: &now, CreatedAt: now,
	}
	bad := job
	bad.ID, bad.LibraryID, bad.Type = "bad", "bad-library", "invalid"
	if err := db.AdvanceSchedule(ctx, plan, &next, "", []jobs.Job{job, bad}); err == nil {
		t.Fatal("invalid task unexpectedly accepted")
	}
	unchanged, err := db.ReadSchedule(ctx, plan.ID)
	if err != nil || unchanged.NextRunAt == nil || !unchanged.NextRunAt.Equal(now) {
		t.Fatalf("schedule advanced after failed submission: %+v %v", unchanged, err)
	}
	if _, err := db.ReadJob(ctx, job.ID); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("partial job outlived failed submission: %v", err)
	}
	if err := db.AdvanceSchedule(ctx, plan, &next, "", []jobs.Job{job}); err != nil {
		t.Fatal(err)
	}
	if err := db.AdvanceSchedule(ctx, plan, &next, "", []jobs.Job{job}); !errors.Is(err, jobs.ErrConflict) {
		t.Fatalf("stale trigger was accepted: %v", err)
	}
}
