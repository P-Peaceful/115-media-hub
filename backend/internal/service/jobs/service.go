// Package jobs manages persistent task and schedule use cases and dispatches registered modules.
package jobs

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
)

type Repository interface {
	QueryJobs(context.Context, domain.JobFilter) (domain.JobPage, error)
	ReadJob(context.Context, string) (domain.Job, error)
	InsertJob(context.Context, domain.Job) error
	QueryFailedItems(context.Context, string, domain.Page) (domain.ItemPage, error)
	QueryJobLogs(context.Context, string, domain.Page) (domain.LogPage, error)
	ChangeJobControl(context.Context, domain.Job, domain.Action, time.Time) error
	ClaimJob(context.Context, time.Time) (domain.Job, error)
	RunningJobs(context.Context) ([]domain.Job, error)
	FinishJob(context.Context, domain.Job, domain.Status, string, time.Time) (domain.Job, error)
	UpdateJobProgress(context.Context, string, int, domain.Progress) error
	UpsertJobItem(context.Context, string, int, domain.Item) error
	InsertJobLog(context.Context, string, int, domain.JobLog) error

	QuerySchedules(context.Context, domain.ScheduleFilter) (domain.SchedulePage, error)
	ReadSchedule(context.Context, string) (domain.Schedule, error)
	InsertSchedule(context.Context, domain.Schedule) error
	SaveSchedule(context.Context, domain.Schedule) error
	RemoveSchedule(context.Context, string, string, string) error
	DueSchedules(context.Context, time.Time) ([]domain.Schedule, error)
	AdvanceSchedule(context.Context, domain.Schedule, *time.Time, string, []domain.Job) error
}

type Libraries interface {
	Get(ctx context.Context, id, requestID string) (library.Library, error)
	List(ctx context.Context, cursor string, limit int, requestID string) ([]library.Library, error)
}

// ErrPauseRequested is the cancellation cause when a pause has been requested.
var ErrPauseRequested = errors.New("task pause requested")

// ErrPaused is returned by a module after safely persisting a resume checkpoint.
var ErrPaused = errors.New("task paused with a checkpoint")

// Module delegates execution to a business module. Run must stop on cancellation.
// A pause-capable module must persist its own checkpoint before returning ErrPaused.
// Recover may authorize resumption after inspecting its own effects and checkpoint.
type Module struct {
	Run            func(context.Context, Execution) error
	Recover        func(context.Context, Execution) (bool, error)
	Eligible       func(library.Library) bool
	CanPause       bool
	CanRetryFailed bool
}

// Execution contains a single snapshot of saved media-library configuration.
// Reports and item keys must be safe for administrator-facing history. Item keys
// must be stable across attempts so a resumed run can update prior results.
type Execution struct {
	Job     domain.Job
	Library library.Library
	Report  func(context.Context, domain.Progress) error
	Item    func(context.Context, domain.Item) error
	Log     func(context.Context, string, string) error

	// RetryItems pages through the original failed items, including module item keys.
	RetryItems func(context.Context, domain.Page) (domain.ItemPage, error)
}

type Service struct {
	repo    Repository
	library Libraries
	modules map[domain.Type]Module
	logger  *slog.Logger
	now     func() time.Time

	runtimeMu sync.Mutex
	stop      context.CancelFunc
	done      chan struct{}
	wake      chan struct{}

	activeMu sync.Mutex
	active   map[runAttempt]context.CancelCauseFunc

	eventMu     sync.Mutex
	events      map[chan domain.Event]struct{}
	eventClosed bool
}

func New(repo Repository, libraries Libraries, modules map[domain.Type]Module, logger *slog.Logger) *Service {
	return &Service{
		repo: repo, library: libraries, modules: maps.Clone(modules), logger: logger.With("module", "jobs"),
		now: time.Now, wake: make(chan struct{}, 1), active: make(map[runAttempt]context.CancelCauseFunc),
		events: make(map[chan domain.Event]struct{}),
	}
}

func (s *Service) Capabilities() []domain.Capability {
	result := make([]domain.Capability, 0, len(domain.Types()))
	for _, kind := range domain.Types() {
		module, available := s.module(kind)
		capability := domain.Capability{Type: kind, Available: available, CanPause: available && module.CanPause, CanRetryFailed: available && module.CanRetryFailed}
		if !available {
			reason := "业务模块尚未接入任务中心"
			capability.Reason = &reason
		}
		result = append(result, capability)
	}
	return result
}

func (s *Service) module(kind domain.Type) (Module, bool) {
	module, exists := s.modules[kind]
	return module, exists && module.Run != nil
}

func (s *Service) target(ctx context.Context, module Module, id, requestID string) (library.Library, error) {
	if !validID(id) {
		return library.Library{}, domain.ErrInvalid
	}
	target, err := s.library.Get(ctx, id, requestID)
	if errors.Is(err, library.ErrNotFound) {
		return library.Library{}, domain.ErrNotFound
	}
	if err != nil {
		return library.Library{}, err
	}
	if !eligible(module, target) {
		return library.Library{}, domain.ErrLibraryUnavailable
	}
	return target, nil
}

func eligible(module Module, target library.Library) bool {
	return target.Enabled && (module.Eligible == nil || module.Eligible(target))
}

func validID(id string) bool { return len(id) > 0 && len(id) <= 128 && !strings.ContainsRune(id, 0) }

func normalizePage(page domain.Page) (domain.Page, error) {
	if page.Limit < 0 || page.Limit > 200 || (page.Cursor != "" && !validID(page.Cursor)) {
		return domain.Page{}, domain.ErrInvalid
	}
	if page.Limit == 0 {
		page.Limit = 50
	}
	return page, nil
}

func (s *Service) ListJobs(ctx context.Context, filter domain.JobFilter) (domain.JobPage, error) {
	page, err := normalizePage(filter.Page)
	if err != nil || (filter.Type != "" && !slices.Contains(domain.Types(), filter.Type)) ||
		(filter.Status != "" && !slices.Contains(domain.Statuses(), filter.Status)) ||
		(filter.LibraryID != "" && !validID(filter.LibraryID)) {
		return domain.JobPage{}, domain.ErrInvalid
	}
	filter.Page = page
	result, err := s.repo.QueryJobs(ctx, filter)
	for i := range result.Items {
		s.decorate(&result.Items[i])
	}
	return result, err
}

func (s *Service) GetJob(ctx context.Context, id string) (domain.Job, error) {
	if !validID(id) {
		return domain.Job{}, domain.ErrInvalid
	}
	job, err := s.repo.ReadJob(ctx, id)
	if err == nil {
		s.decorate(&job)
	}
	return job, err
}

func (s *Service) decorate(job *domain.Job) {
	job.Actions = s.actions(*job)
	if job.StartedAt != nil {
		end := s.now()
		if job.FinishedAt != nil {
			end = *job.FinishedAt
		}
		duration := max(int64(0), end.Sub(*job.StartedAt).Milliseconds())
		job.DurationMS = &duration
	}
}

func (s *Service) actions(job domain.Job) []domain.Action {
	actions := []domain.Action{}
	module, available := s.module(job.Type)
	switch job.Status {
	case domain.Queued:
		actions = append(actions, domain.Cancel)
	case domain.Running:
		if job.PendingAction != domain.Cancel {
			actions = append(actions, domain.Cancel)
			if available && module.CanPause && job.PendingAction == "" {
				actions = append(actions, domain.Pause)
			}
		}
	case domain.Paused:
		actions = append(actions, domain.Cancel)
		if available && module.CanPause {
			actions = append(actions, domain.Resume)
		}
	case domain.Failed, domain.Canceled, domain.Completed:
		if available && (job.Status != domain.Completed || job.FailedItems > 0) {
			actions = append(actions, domain.Retry)
			if module.CanRetryFailed && job.FailedItems > 0 {
				actions = append(actions, domain.RetryFailed)
			}
		}
	}
	return actions
}

func (s *Service) ListFailedItems(ctx context.Context, id string, page domain.Page) (domain.ItemPage, error) {
	page, err := normalizePage(page)
	if err != nil {
		return domain.ItemPage{}, err
	}
	if _, err := s.GetJob(ctx, id); err != nil {
		return domain.ItemPage{}, err
	}
	return s.repo.QueryFailedItems(ctx, id, page)
}

func (s *Service) ListLogs(ctx context.Context, id string, page domain.Page) (domain.LogPage, error) {
	page, err := normalizePage(page)
	if err != nil {
		return domain.LogPage{}, err
	}
	if _, err := s.GetJob(ctx, id); err != nil {
		return domain.LogPage{}, err
	}
	return s.repo.QueryJobLogs(ctx, id, page)
}

func (s *Service) newJob(kind domain.Type, lib library.Library, source domain.Source, requestID string) domain.Job {
	return domain.Job{
		ID: rand.Text(), Type: kind, LibraryID: lib.ID, LibraryName: lib.Name,
		Source: source, Status: domain.Queued, CreatedAt: s.now().UTC(), RequestID: requestID,
	}
}

func (s *Service) Submit(ctx context.Context, kind domain.Type, libraryID, requestID string) (domain.Job, error) {
	if !slices.Contains(domain.Types(), kind) || !validID(libraryID) {
		return domain.Job{}, domain.ErrInvalid
	}
	module, available := s.module(kind)
	if !available {
		return domain.Job{}, domain.ErrUnavailable
	}
	lib, err := s.target(ctx, module, libraryID, requestID)
	if err != nil {
		return domain.Job{}, err
	}
	job := s.newJob(kind, lib, domain.Manual, requestID)
	return s.submit(ctx, job, "submit")
}

func (s *Service) submit(ctx context.Context, job domain.Job, operation string) (domain.Job, error) {
	started := time.Now()
	err := s.repo.InsertJob(ctx, job)
	s.log(ctx, operation, job, started, err)
	if err != nil {
		return domain.Job{}, err
	}
	s.publish(domain.Event{JobID: job.ID})
	s.wakeWorker()
	s.decorate(&job)
	return job, nil
}

func (s *Service) Control(ctx context.Context, id string, action domain.Action, requestID string) (domain.Job, error) {
	if action != domain.Pause && action != domain.Resume && action != domain.Cancel {
		return domain.Job{}, domain.ErrInvalid
	}
	started := time.Now()
	job, err := s.GetJob(ctx, id)
	if err != nil {
		return domain.Job{}, err
	}
	if !slices.Contains(job.Actions, action) {
		return domain.Job{}, domain.ErrConflict
	}
	err = s.repo.ChangeJobControl(ctx, job, action, s.now().UTC())
	audit := job
	audit.RequestID = requestID
	s.log(ctx, string(action), audit, started, err)
	if err != nil {
		return domain.Job{}, err
	}
	s.publish(domain.Event{JobID: id})
	if job.Status == domain.Running {
		s.signalControl(job, action)
	}
	if action == domain.Resume {
		s.wakeWorker()
	}
	return s.GetJob(ctx, id)
}

func (s *Service) Retry(ctx context.Context, id string, scope domain.RetryScope, requestID string) (domain.Job, error) {
	if scope != domain.RetryWhole && scope != domain.RetryFailures {
		return domain.Job{}, domain.ErrInvalid
	}
	parent, err := s.GetJob(ctx, id)
	if err != nil {
		return domain.Job{}, err
	}
	module, available := s.module(parent.Type)
	if !available {
		return domain.Job{}, domain.ErrUnavailable
	}
	action := domain.Retry
	if scope == domain.RetryFailures {
		action = domain.RetryFailed
	}
	if !slices.Contains(parent.Actions, action) {
		return domain.Job{}, domain.ErrConflict
	}
	lib, err := s.target(ctx, module, parent.LibraryID, requestID)
	if err != nil {
		return domain.Job{}, err
	}
	job := s.newJob(parent.Type, lib, domain.Retried, requestID)
	job.ParentID = &id
	job.RetryScope = scope
	return s.submit(ctx, job, string(action))
}

func (s *Service) log(ctx context.Context, operation string, job domain.Job, started time.Time, err error) {
	attrs := []any{"operation", operation, "job_id", job.ID, "library_id", job.LibraryID,
		"task_type", job.Type, "attempt", job.Attempt, "duration_ms", time.Since(started).Milliseconds()}
	if job.RequestID != "" {
		attrs = append(attrs, "request_id", job.RequestID)
	}
	if job.ParentID != nil {
		attrs = append(attrs, "parent_job_id", *job.ParentID)
	}
	if job.RetryScope != "" {
		attrs = append(attrs, "retry_scope", job.RetryScope)
	}
	if err == nil {
		s.logger.InfoContext(ctx, "task operation completed", append(attrs, "outcome", "succeeded")...)
		return
	}
	level := slog.LevelError
	outcome, code := "failed", "storage_unavailable"
	if errors.Is(err, domain.ErrConflict) {
		level, code = slog.LevelWarn, "conflict"
	} else if errors.Is(err, context.Canceled) {
		level, outcome, code = slog.LevelInfo, "canceled", "operation_canceled"
	}
	s.logger.Log(ctx, level, "task operation failed", append(attrs, "outcome", outcome, "error_code", code)...)
}

// Subscribe delivers bounded change hints. A slow subscriber reconnects and re-reads history.
func (s *Service) Subscribe() (<-chan domain.Event, func()) {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	if s.eventClosed || len(s.events) >= 64 {
		return nil, func() {}
	}
	subscription := make(chan domain.Event, 32)
	s.events[subscription] = struct{}{}
	return subscription, func() {
		s.eventMu.Lock()
		defer s.eventMu.Unlock()
		s.closeSubscription(subscription)
	}
}

func (s *Service) setEventsClosed(closed bool) {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	s.eventClosed = closed
	if closed {
		for subscription := range s.events {
			s.closeSubscription(subscription)
		}
	}
}

func (s *Service) publish(event domain.Event) {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	for subscription := range s.events {
		select {
		case subscription <- event:
		default:
			s.closeSubscription(subscription)
		}
	}
}

func (s *Service) closeSubscription(subscription chan domain.Event) {
	if _, exists := s.events[subscription]; exists {
		delete(s.events, subscription)
		close(subscription)
	}
}
