package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
)

func newFixture(t *testing.T, modules map[domain.Type]Module) (*Service, *sqlite.Database, *library.Service) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "task.db"))
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	libraries := library.New(db, logger)
	service := New(db, libraries, modules, logger)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := service.Stop(ctx); err != nil {
			t.Error(err)
		}
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return service, db, libraries
}

func testLibrary(t *testing.T, libraries *library.Service, name string) library.Library {
	t.Helper()
	target, err := libraries.Create(t.Context(), library.Config{
		Name: name, DirectoryID: "10", DirectoryPath: "/media", MediaType: "movie", OutputPath: t.TempDir(),
		LanguagePreference: "zh-CN", ScrapeSource: "none", FileEncoding: "UTF-8",
		LinkType: "proxy", STRMLinkFormat: "file_id", STRMExistingFilePolicy: "skip",
	}, "create-library")
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func waitTask(t *testing.T, service *Service, events <-chan domain.Event, id string, want domain.Status) domain.Job {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		job, err := service.GetJob(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status == want {
			return job
		}
		select {
		case _, ok := <-events:
			if !ok {
				t.Fatal("subscription closed")
			}
		case <-timer.C:
			t.Fatalf("job %s status = %s, want %s", id, job.Status, want)
		}
	}
}

func TestScheduleAndRetryPersistModuleResults(t *testing.T) {
	var seenRetry atomic.Bool
	module := Module{CanRetryFailed: true, Run: func(ctx context.Context, x Execution) error {
		if x.Job.Source == domain.Retried {
			page, err := x.RetryItems(ctx, domain.Page{Limit: 1})
			if err != nil || len(page.Items) != 1 || page.Items[0].Key != "bad-file" || page.NextCursor != nil {
				return errors.New("failed item scope was not passed to the module")
			}
			seenRetry.Store(true)
			item := page.Items[0]
			item.Status, item.Reason = domain.ItemSucceeded, ""
			return x.Item(ctx, item)
		}
		percent, bytes, total, path := 100.0, int64(200), int64(2), "/media/good.mkv"
		return errors.Join(
			x.Report(ctx, domain.Progress{ProgressPercent: &percent, SpeedBytesPerSecond: &bytes, TotalItems: &total, CurrentFile: &path}),
			x.Item(ctx, domain.Item{Key: "good-file", Path: path, Status: domain.ItemSucceeded}),
			x.Item(ctx, domain.Item{Key: "bad-file", Path: "/media/bad.mkv", Status: domain.ItemFailed, Reason: "文件不存在"}),
			x.Log(ctx, "warn", "provider request failed: Cookie: UID=private-example"),
		)
	}}
	service, db, libraries := newFixture(t, map[domain.Type]Module{domain.Scan: module})
	target := testLibrary(t, libraries, "Active")
	disabled := testLibrary(t, libraries, "Disabled")
	if _, err := libraries.SetEnabled(t.Context(), disabled.ID, false, "disable-library"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 20, 17, 59, 59, 0, time.UTC)
	service.now = func() time.Time { return now }
	clock := "02:00"
	plan, err := service.CreateSchedule(t.Context(), domain.ScheduleInput{
		Name: " All media ", Type: domain.Scan, Enabled: true, Period: domain.Daily, Time: &clock,
		NotifyOn: []domain.Outcome{domain.PartialFailure},
	}, "create-plan")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Name != "All media" || plan.NextRunAt == nil || !plan.NextRunAt.Equal(now.Add(time.Second)) || plan.Timezone != "Asia/Shanghai" {
		t.Fatalf("unexpected plan fields: %+v", plan)
	}
	stored, err := db.ReadSchedule(t.Context(), plan.ID)
	if err != nil || len(stored.NotifyOn) != 1 || stored.NotifyOn[0] != domain.PartialFailure {
		t.Fatalf("persisted schedule: %+v %v", stored, err)
	}
	now = now.Add(2 * time.Second)
	var ticks sync.WaitGroup
	for range 2 {
		ticks.Go(func() {
			if err := service.tickSchedules(t.Context(), now, false); err != nil {
				t.Error(err)
			}
		})
	}
	ticks.Wait()
	pending, err := service.ListJobs(t.Context(), domain.JobFilter{})
	if err != nil || pending.Total != 1 || pending.ByType[domain.Scan] != 1 || pending.ByStatus[domain.Queued] != 1 {
		t.Fatalf("trigger duplication or wrong target: %+v %v", pending, err)
	}
	job := pending.Items[0]
	if job.LibraryID != target.ID || job.ScheduleID == nil || *job.ScheduleID != plan.ID || job.Source != domain.Scheduled {
		t.Fatalf("wrong scheduled task: %+v", job)
	}
	plan, err = service.SetScheduleEnabled(t.Context(), plan.ID, false, "pause-plan")
	if err != nil || plan.NextRunAt != nil || plan.Enabled {
		t.Fatalf("disable schedule: %+v %v", plan, err)
	}
	disabledPlan, err := service.GetSchedule(t.Context(), plan.ID)
	if err != nil || disabledPlan.Enabled || disabledPlan.NextRunAt != nil || disabledPlan.Revision != plan.Revision {
		t.Fatalf("persisted disabled schedule: %+v %v", disabledPlan, err)
	}
	events, closeEvents := service.Subscribe()
	defer closeEvents()
	notifications, closeNotifications := service.Subscribe()
	defer closeNotifications()
	if err := service.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	job = waitTask(t, service, events, job.ID, domain.Completed)
	if job.Outcome == nil || *job.Outcome != domain.PartialFailure || job.SucceededItems != 1 || job.FailedItems != 1 ||
		job.ProgressPercent == nil || *job.ProgressPercent != 100 || job.TotalItems == nil || *job.TotalItems != 2 {
		t.Fatalf("unexpected execution result: %+v", job)
	}
	select {
	case <-time.After(time.Second):
		t.Fatal("completion notification missing")
	default:
		found := false
		for len(notifications) > 0 {
			event := <-notifications
			found = found || (event.JobID == job.ID && event.Notification != nil && *event.Notification == domain.PartialFailure)
		}
		// The commit may be observed before the notification is published.
		if !found {
			select {
			case event := <-notifications:
				if event.JobID != job.ID || event.Notification == nil || *event.Notification != domain.PartialFailure {
					t.Fatalf("unexpected event: %+v", event)
				}
			case <-time.After(time.Second):
				t.Fatal("completion notification missing")
			}
		}
	}
	failures, err := service.ListFailedItems(t.Context(), job.ID, domain.Page{})
	if err != nil || len(failures.Items) != 1 || failures.Items[0].Reason != "文件不存在" {
		t.Fatalf("missing failure: %+v %v", failures, err)
	}
	logs, err := service.ListLogs(t.Context(), job.ID, domain.Page{})
	if err != nil || len(logs.Items) < 4 {
		t.Fatalf("missing task logs: %+v %v", logs, err)
	}
	for _, message := range logs.Items {
		if strings.Contains(message.Message, "private-example") {
			t.Fatal("module credential leaked into task logs")
		}
	}
	child, err := service.Retry(t.Context(), job.ID, domain.RetryFailures, "retry-request")
	if err != nil {
		t.Fatal(err)
	}
	child = waitTask(t, service, events, child.ID, domain.Completed)
	if !seenRetry.Load() || child.ParentID == nil || *child.ParentID != job.ID || child.Outcome == nil || *child.Outcome != domain.Success {
		t.Fatalf("unexpected failed item retry result: %+v", child)
	}
	if err := service.DeleteSchedule(t.Context(), plan.ID, "administrator", "delete-plan"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetJob(t.Context(), job.ID); err != nil {
		t.Fatalf("deleting a plan lost its job: %v", err)
	}
	if err := service.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	for range events {
	}
	if subscription, close := service.Subscribe(); subscription != nil {
		close()
		t.Fatal("subscription accepted during shutdown")
	}
}

func TestPauseResumeAndCancel(t *testing.T) {
	started := make(chan Execution, 3)
	runContexts := make(chan context.Context, 3)
	service, _, libraries := newFixture(t, map[domain.Type]Module{domain.Sync: {
		CanPause: true,
		Run: func(ctx context.Context, x Execution) error {
			runContexts <- ctx
			started <- x
			<-ctx.Done()
			if errors.Is(context.Cause(ctx), ErrPauseRequested) {
				return ErrPaused
			}
			return ctx.Err()
		},
	}})
	target := testLibrary(t, libraries, "Sync")
	events, closeEvents := service.Subscribe()
	defer closeEvents()
	if err := service.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	job, err := service.Submit(t.Context(), domain.Sync, target.ID, "sub")
	if err != nil {
		t.Fatal(err)
	}
	first := awaitExecution(t, started)
	<-runContexts
	if first.Job.NeedsResume {
		t.Fatal("new job should start without resume")
	}
	if _, err := service.Control(t.Context(), job.ID, domain.Pause, "pause"); err != nil {
		t.Fatal(err)
	}
	job = waitTask(t, service, events, job.ID, domain.Paused)
	if _, err := service.Control(t.Context(), job.ID, domain.Resume, "resume"); err != nil {
		t.Fatal(err)
	}
	second := awaitExecution(t, started)
	secondCtx := <-runContexts
	if !second.Job.NeedsResume || second.Job.Attempt != 2 || second.Job.ID != first.Job.ID {
		t.Fatalf("resume metadata: %+v", second.Job)
	}
	service.signalControl(first.Job, domain.Pause)
	if secondCtx.Err() != nil {
		t.Fatal("a stale control canceled the resumed attempt")
	}
	percent := 25.0
	progress := domain.Progress{ProgressPercent: &percent}
	item := domain.Item{Key: "stale-item", Status: domain.ItemFailed}
	for operation, err := range map[string]error{
		"progress": first.Report(t.Context(), progress),
		"item":     first.Item(t.Context(), item),
		"log":      first.Log(t.Context(), "info", "stale message"),
	} {
		if !errors.Is(err, domain.ErrConflict) {
			t.Errorf("a stale attempt wrote %s: %v", operation, err)
		}
	}
	if err := second.Report(secondCtx, progress); err != nil {
		t.Fatalf("the resumed attempt could not report progress: %v", err)
	}
	if _, err := service.Control(t.Context(), job.ID, domain.Cancel, "cancel"); err != nil {
		t.Fatal(err)
	}
	job = waitTask(t, service, events, job.ID, domain.Canceled)
	if job.FinishedAt == nil || job.Outcome != nil {
		t.Fatalf("cancellation result: %+v", job)
	}
}

func TestProgressPublishesTrailingChangeWhileRunning(t *testing.T) {
	started := make(chan Execution, 1)
	service, _, libraries := newFixture(t, map[domain.Type]Module{domain.Scan: {
		Run: func(ctx context.Context, x Execution) error {
			started <- x
			<-ctx.Done()
			return ctx.Err()
		},
	}})
	target := testLibrary(t, libraries, "Scan")
	if err := service.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	job, err := service.Submit(t.Context(), domain.Scan, target.ID, "submit")
	if err != nil {
		t.Fatal(err)
	}
	execution := awaitExecution(t, started)
	events, closeEvents := service.Subscribe()
	defer closeEvents()
	for _, percent := range []float64{10, 20} {
		if err := execution.Report(t.Context(), domain.Progress{ProgressPercent: &percent}); err != nil {
			t.Fatal(err)
		}
		select {
		case event := <-events:
			if event.JobID != job.ID {
				t.Fatalf("unexpected task change: %+v", event)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("live progress change missing at %.0f%%", percent)
		}
	}
	latest, err := service.GetJob(t.Context(), job.ID)
	if err != nil || latest.Status != domain.Running || latest.ProgressPercent == nil || *latest.ProgressPercent != 20 {
		t.Fatalf("unexpected live state: %+v %v", latest, err)
	}
}

func awaitExecution(t *testing.T, executed <-chan Execution) Execution {
	t.Helper()
	select {
	case value := <-executed:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not call module")
		return Execution{}
	}
}

func TestRestartReconcilesAndSkipsMissedPlans(t *testing.T) {
	verified := make(chan string, 1)
	executed := make(chan Execution, 2)
	service, db, libraries := newFixture(t, map[domain.Type]Module{
		domain.Scan: {
			Run: func(_ context.Context, x Execution) error { executed <- x; return nil },
			Recover: func(_ context.Context, x Execution) (bool, error) {
				verified <- x.Job.ID
				return true, nil
			},
		},
		domain.Check: {Run: func(_ context.Context, x Execution) error { executed <- x; return nil }},
	})
	target := testLibrary(t, libraries, "Recover")
	other := testLibrary(t, libraries, "No verification")
	firstTime := time.Date(2026, 11, 20, 17, 0, 0, 0, time.UTC)
	var clock atomic.Int64
	clock.Store(firstTime.UnixNano())
	service.now = func() time.Time { return time.Unix(0, clock.Load()) }
	job, err := service.Submit(t.Context(), domain.Scan, target.ID, "run")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimJob(t.Context(), firstTime); err != nil {
		t.Fatal(err)
	}
	uncertain, err := service.Submit(t.Context(), domain.Check, other.ID, "uncertain")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimJob(t.Context(), firstTime); err != nil {
		t.Fatal(err)
	}
	timeOfDay := "02:30"
	plan, err := service.CreateSchedule(t.Context(), domain.ScheduleInput{
		Name: "Schedule", Type: domain.Scan, LibraryID: &target.ID, Enabled: true, Period: domain.Daily, Time: &timeOfDay,
	}, "create")
	if err != nil {
		t.Fatal(err)
	}
	oldRun := *plan.NextRunAt
	clock.Store(firstTime.Add(48 * time.Hour).UnixNano())
	events, closeEvents := service.Subscribe()
	defer closeEvents()
	if err := service.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-verified:
		if id != job.ID {
			t.Fatalf("verified wrong job %s", id)
		}
	default:
		t.Fatal("running job was not verified")
	}
	run := awaitExecution(t, executed)
	if run.Job.ID != job.ID || !run.Job.NeedsResume || run.Job.Attempt != 2 {
		t.Fatalf("resume arguments: %+v", run.Job)
	}
	failed := waitTask(t, service, events, uncertain.ID, domain.Failed)
	if failed.Outcome == nil || *failed.Outcome != domain.Failure {
		t.Fatalf("interrupted result: %+v", failed)
	}
	pages, err := service.ListJobs(t.Context(), domain.JobFilter{})
	if err != nil || pages.Total != 2 {
		t.Fatalf("missed plans should not create jobs: %+v %v", pages, err)
	}
	next, err := service.GetSchedule(t.Context(), plan.ID)
	if err != nil || next.NextRunAt == nil || !next.NextRunAt.Equal(oldRun.Add(48*time.Hour)) {
		t.Fatalf("incorrect next run after downtime: %+v %v", next, err)
	}
}
