package jobs

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
)

const pollInterval = time.Second

// Start reconciles persisted running jobs before starting the scheduler and two workers.
func (s *Service) Start(ctx context.Context) error {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if s.done != nil {
		select {
		case <-s.done:
		default:
			return domain.ErrConflict
		}
	}
	started := s.now().UTC()
	if err := s.reconcile(ctx); err != nil {
		return fmt.Errorf("recover task center: %w", err)
	}
	if err := s.tickSchedules(ctx, started, true); err != nil {
		return fmt.Errorf("advance missed schedules: %w", err)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	s.stop, s.done = stop, done
	s.setEventsClosed(false)
	go func() {
		defer close(done)
		var running sync.WaitGroup
		for range 2 {
			running.Go(func() { s.worker(runCtx) })
		}
		running.Go(func() { s.scheduler(runCtx) })
		running.Wait()
	}()
	return nil
}

// Stop cancels running work and waits until the workers release their resources.
func (s *Service) Stop(ctx context.Context) error {
	s.runtimeMu.Lock()
	done := s.done
	s.setEventsClosed(true)
	if s.stop != nil {
		s.stop()
	}
	s.runtimeMu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) scheduler(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		if err := s.tickSchedules(ctx, s.now().UTC(), false); err != nil && ctx.Err() == nil {
			s.logger.ErrorContext(ctx, "scheduler tick failed", "operation", "trigger_schedule",
				"outcome", "failed", "error_code", "schedule_tick_failed", "duration_ms", 0)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) worker(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		started := time.Now()
		job, err := s.repo.ClaimJob(ctx, s.now().UTC())
		if err == nil {
			s.log(ctx, "claim", job, started, nil)
			s.publish(domain.Event{JobID: job.ID})
			s.wakeWorker()
			s.execute(ctx, job)
			continue
		}
		if !errors.Is(err, domain.ErrNotFound) && ctx.Err() == nil {
			s.logger.ErrorContext(ctx, "task claim failed", "operation", "claim",
				"outcome", "failed", "error_code", "storage_unavailable", "duration_ms", time.Since(started).Milliseconds())
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
	}
}

func (s *Service) wakeWorker() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

type runAttempt struct {
	jobID  string
	number int
}

func (s *Service) signalControl(job domain.Job, action domain.Action) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	if stop, ok := s.active[runAttempt{job.ID, job.Attempt}]; ok {
		stop(controlCause(action))
	}
}

func controlCause(action domain.Action) error {
	if action == domain.Pause {
		return ErrPauseRequested
	}
	return context.Canceled
}

func (s *Service) run(ctx context.Context, job domain.Job) (string, error) {
	runCtx, cancel := context.WithCancelCause(ctx)
	attempt := runAttempt{job.ID, job.Attempt}
	s.activeMu.Lock()
	s.active[attempt] = cancel
	s.activeMu.Unlock()
	defer func() {
		s.activeMu.Lock()
		delete(s.active, attempt)
		s.activeMu.Unlock()
		cancel(context.Canceled)
	}()
	// A control can persist before the worker registers its cancellation hook.
	current, err := s.repo.ReadJob(ctx, job.ID)
	if err != nil {
		return "storage_unavailable", err
	}
	if current.PendingAction != "" {
		cancel(controlCause(current.PendingAction))
	}
	if runCtx.Err() != nil {
		if current.PendingAction == domain.Pause {
			return "", ErrPaused
		}
		return "", runCtx.Err()
	}
	module, available := s.module(job.Type)
	if !available {
		return "task_type_unavailable", domain.ErrUnavailable
	}
	target, err := s.target(runCtx, module, job.LibraryID, job.RequestID)
	if errors.Is(context.Cause(runCtx), ErrPauseRequested) {
		return "", ErrPaused
	}
	if err != nil {
		if errors.Is(err, library.ErrNotFound) || errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrLibraryUnavailable) {
			return "library_unavailable", err
		}
		return "load_library_failed", err
	}
	execution, closeUpdates := s.execution(job, target)
	defer closeUpdates()
	err = runModule(runCtx, module, execution)
	if err != nil {
		return "module_execution_failed", err
	}
	return "", nil
}

func runModule(ctx context.Context, module Module, execution Execution) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("task module panicked")
		}
	}()
	return module.Run(ctx, execution)
}

func (s *Service) execute(ctx context.Context, job domain.Job) {
	started := time.Now()
	code, runErr := s.run(ctx, job)
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	for {
		current, err := s.repo.ReadJob(saveCtx, job.ID)
		if err != nil {
			s.log(saveCtx, "read_execution_result", job, started, err)
			return
		}
		if current.Status != domain.Running || current.Attempt != job.Attempt {
			return
		}
		status := domain.Completed
		switch {
		case current.PendingAction == domain.Cancel:
			status, code = domain.Canceled, "canceled_by_user"
		case errors.Is(runErr, ErrPaused) && current.PendingAction == domain.Pause:
			status, code = domain.Paused, "paused_by_user"
		case ctx.Err() != nil && runErr != nil:
			s.logger.InfoContext(saveCtx, "task execution stopped for restart recovery", "operation", "execute",
				"job_id", job.ID, "library_id", job.LibraryID, "attempt", job.Attempt, "outcome", "canceled",
				"duration_ms", time.Since(started).Milliseconds())
			return
		case runErr != nil:
			status = domain.Failed
		}
		err = s.finish(saveCtx, current, status, code, "execute", started)
		if !errors.Is(err, domain.ErrConflict) {
			return
		}
	}
}

func (s *Service) reconcile(ctx context.Context) error {
	running, err := s.repo.RunningJobs(ctx)
	if err != nil {
		return err
	}
	for _, job := range running {
		start := time.Now()
		status, code := s.reconcileJob(ctx, job)
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.finish(ctx, job, status, code, "reconcile", start); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) reconcileJob(ctx context.Context, job domain.Job) (domain.Status, string) {
	module, available := s.module(job.Type)
	if !available {
		return domain.Failed, "task_type_unavailable"
	}
	if module.Recover == nil {
		return domain.Failed, "restart_verification_unavailable"
	}
	target, err := s.target(ctx, module, job.LibraryID, job.RequestID)
	if err != nil {
		return domain.Failed, "restart_library_unavailable"
	}
	execution, closeUpdates := s.execution(job, target)
	defer closeUpdates()
	verified, err := verifyModule(ctx, module, execution)
	if err != nil || !verified {
		return domain.Failed, "restart_verification_failed"
	}
	switch job.PendingAction {
	case domain.Cancel:
		return domain.Canceled, "restart_canceled"
	case domain.Pause:
		return domain.Paused, "restart_paused"
	default:
		return domain.Queued, "restart_requeued"
	}
}

func verifyModule(ctx context.Context, module Module, execution Execution) (_ bool, err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("task recovery module panicked")
		}
	}()
	return module.Recover(ctx, execution)
}

func (s *Service) finish(ctx context.Context, job domain.Job, status domain.Status, code, operation string, started time.Time) error {
	finished, err := s.repo.FinishJob(ctx, job, status, code, s.now().UTC())
	if err != nil {
		if !errors.Is(err, domain.ErrConflict) {
			s.log(ctx, operation, job, started, err)
		}
		return err
	}
	event := domain.Event{JobID: job.ID}
	if finished.Outcome != nil && slices.Contains(finished.NotifyOn, *finished.Outcome) {
		event.Notification = finished.Outcome
	}
	s.publish(event)
	s.wakeWorker()

	attrs := []any{"operation", operation, "job_id", job.ID, "library_id", job.LibraryID,
		"task_type", job.Type, "attempt", job.Attempt, "status", finished.Status,
		"succeeded_items", finished.SucceededItems, "failed_items", finished.FailedItems, "skipped_items", finished.SkippedItems,
		"duration_ms", time.Since(started).Milliseconds()}
	if job.RequestID != "" {
		attrs = append(attrs, "request_id", job.RequestID)
	}
	if code == "" && finished.FailedItems > 0 {
		code = "item_failures"
	}
	level, outcome := slog.LevelInfo, "succeeded"
	switch finished.Status {
	case domain.Failed:
		level, outcome = slog.LevelWarn, "failed"
		attrs = append(attrs, "error_code", code)
	case domain.Canceled:
		outcome = "canceled"
	case domain.Completed:
		if finished.FailedItems > 0 {
			level, outcome = slog.LevelWarn, "failed"
			attrs = append(attrs, "error_code", code)
		}
	}
	if code != "" {
		attrs = append(attrs, "reason", code)
	}
	s.logger.Log(ctx, level, "task execution state changed", append(attrs, "outcome", outcome)...)
	return nil
}

// Only administrator-safe summaries belong in module callbacks. Suppress common
// accidental credential and URL forms before persisting any free-form fields.
var sensitiveText = regexp.MustCompile(`(?i)(https?://|\b(?:cookie|authorization|password|access[_-]?token|refresh[_-]?token|pickcode|uid|cid|seid|kid)\s*[:=]|\bbearer\s+)`)

func safeText(value string, limit int) string {
	if sensitiveText.MatchString(value) {
		return "内容已隐藏"
	}
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	for len(value) > limit {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value
}

func (s *Service) execution(job domain.Job, target library.Library) (Execution, func()) {
	execution := Execution{Job: job, Library: target}
	updates := &executionUpdates{publish: func() { s.publish(domain.Event{JobID: job.ID}) }}
	execution.Report = func(ctx context.Context, progress domain.Progress) error {
		if invalidProgress(progress) {
			return domain.ErrInvalid
		}
		if progress.CurrentFile != nil {
			path := safeText(*progress.CurrentFile, 4096)
			progress.CurrentFile = &path
		}
		if err := s.repo.UpdateJobProgress(ctx, job.ID, job.Attempt, progress); err != nil {
			return err
		}
		updates.report()
		return nil
	}
	execution.Item = func(ctx context.Context, item domain.Item) error {
		if item.Key == "" || len(item.Key) > 4096 ||
			(item.Status != domain.ItemSucceeded && item.Status != domain.ItemFailed && item.Status != domain.ItemSkipped) {
			return domain.ErrInvalid
		}
		item.ID = rand.Text()
		item.Path = safeText(item.Path, 4096)
		item.Reason = safeText(item.Reason, 1024)
		if err := s.repo.UpsertJobItem(ctx, job.ID, job.Attempt, item); err != nil {
			return err
		}
		updates.report()
		return nil
	}
	execution.Log = func(ctx context.Context, level, message string) error {
		if level != "info" && level != "warn" && level != "error" {
			return domain.ErrInvalid
		}
		err := s.repo.InsertJobLog(ctx, job.ID, job.Attempt, domain.JobLog{
			ID: rand.Text(), Level: level, Message: safeText(message, 2048), CreatedAt: s.now().UTC(),
		})
		if err == nil {
			updates.report()
		}
		return err
	}
	if job.ParentID != nil && job.RetryScope == domain.RetryFailures {
		execution.RetryItems = func(ctx context.Context, page domain.Page) (domain.ItemPage, error) {
			return s.ListFailedItems(ctx, *job.ParentID, page)
		}
	}
	return execution, updates.close
}

// executionUpdates sends at most one change hint per second, including the last
// report when a quiet period follows. The terminal transition sends its own hint.
type executionUpdates struct {
	mu      sync.Mutex
	publish func()
	next    time.Time
	timer   *time.Timer
	closed  bool
}

func (u *executionUpdates) report() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed || u.timer != nil {
		return
	}
	if delay := time.Until(u.next); delay > 0 {
		u.timer = time.AfterFunc(delay, u.flush)
		return
	}
	u.next = time.Now().Add(time.Second)
	u.publish()
}

func (u *executionUpdates) flush() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.timer = nil
	if !u.closed {
		u.next = time.Now().Add(time.Second)
		u.publish()
	}
}

func (u *executionUpdates) close() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.closed = true
	if u.timer != nil {
		u.timer.Stop()
	}
}

func invalidProgress(progress domain.Progress) bool {
	return (progress.ProgressPercent != nil && (math.IsNaN(*progress.ProgressPercent) || *progress.ProgressPercent < 0 || *progress.ProgressPercent > 100)) ||
		(progress.SpeedBytesPerSecond != nil && *progress.SpeedBytesPerSecond < 0) ||
		(progress.TotalItems != nil && *progress.TotalItems < 0)
}
