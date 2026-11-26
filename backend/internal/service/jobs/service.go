// service.go：编排任务提交、控制、重试和历史查询，维护业务模块能力及有界变更订阅。

// Package jobs 管理持久化任务和计划，并调度已注册的业务模块。
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

// Repository 定义任务和计划的持久化合同，条件状态变更及执行次数隔离由实现负责。
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

// Libraries 提供当前保存的媒体库配置，供入队和每次执行时重新核对资格。
type Libraries interface {
	Get(ctx context.Context, id, requestID string) (library.Library, error)
	List(ctx context.Context, cursor string, limit int, requestID string) ([]library.Library, error)
}

// ErrPauseRequested 是用户请求暂停时传递给业务上下文的取消原因。
var ErrPauseRequested = errors.New("task pause requested")

// ErrPaused 由支持暂停的模块在安全保存恢复检查点后返回。
var ErrPaused = errors.New("task paused with a checkpoint")

// Module 声明业务执行及能力；Run 必须响应取消，暂停需先保存检查点再返回 ErrPaused，Recover 核验后才可恢复。
type Module struct {
	Run            func(context.Context, Execution) error
	Recover        func(context.Context, Execution) (bool, error)
	Eligible       func(library.Library) bool
	CanPause       bool
	CanRetryFailed bool
	ManualOnly     bool
}

// Execution 提供单次执行的媒体库配置快照与上报回调；条目键须跨执行次数稳定，摘要须适合管理员查看。
type Execution struct {
	Job     domain.Job
	Library library.Library
	Report  func(context.Context, domain.Progress) error
	Item    func(context.Context, domain.Item) error
	Log     func(context.Context, string, string) error

	// RetryItems 分页读取原任务的失败条目及稳定业务键，仅失败条目重试时提供。
	RetryItems func(context.Context, domain.Page) (domain.ItemPage, error)
}

// Options 配置任务日志的可选观察者，在任务日志持久化成功后调用。
type Options struct {
	ObserveLog func(domain.Job, domain.JobLog)
}

// Service 持有业务模块、运行生命周期、活动执行取消句柄和受锁保护的事件订阅。
type Service struct {
	repo       Repository
	library    Libraries
	modules    map[domain.Type]Module
	logger     *slog.Logger
	now        func() time.Time
	observeLog func(domain.Job, domain.JobLog)

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

// New 复制模块注册表并创建任务服务，后台循环由 Start 显式启动。
func New(repo Repository, libraries Libraries, modules map[domain.Type]Module, logger *slog.Logger, options ...Options) *Service {
	service := &Service{
		repo: repo, library: libraries, modules: maps.Clone(modules), logger: logger.With("module", "jobs"),
		now: time.Now, wake: make(chan struct{}, 1), active: make(map[runAttempt]context.CancelCauseFunc),
		events: make(map[chan domain.Event]struct{}),
	}
	if len(options) > 0 {
		service.observeLog = options[0].ObserveLog
	}
	return service
}

// Capabilities 列出所有任务类型，仅为已注册且可执行的模块开放其声明能力。
func (s *Service) Capabilities() []domain.Capability {
	result := make([]domain.Capability, 0, len(domain.Types()))
	for _, kind := range domain.Types() {
		module, available := s.module(kind)
		capability := domain.Capability{Type: kind, Available: available, CanPause: available && module.CanPause, CanRetryFailed: available && module.CanRetryFailed, CanSchedule: available && !module.ManualOnly}
		if !available {
			reason := "业务模块尚未接入任务中心"
			capability.Reason = &reason
		}
		result = append(result, capability)
	}
	return result
}

// module 查找具有 Run 回调的已注册业务模块。
func (s *Service) module(kind domain.Type) (Module, bool) {
	module, exists := s.modules[kind]
	return module, exists && module.Run != nil
}

// target 校验媒体库 ID 并读取当前配置，拒绝缺失、停用或不满足模块资格的目标。
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

// eligible 要求媒体库启用且通过模块可选的资格条件。
func eligible(module Module, target library.Library) bool {
	return target.Enabled && (module.Eligible == nil || module.Eligible(target))
}

// validID 约束内部资源 ID 的长度并拒绝 NUL 字符。
func validID(id string) bool { return len(id) > 0 && len(id) <= 128 && !strings.ContainsRune(id, 0) }

// normalizePage 校验分页大小与游标，未指定大小时默认每页 50 条。
func normalizePage(page domain.Page) (domain.Page, error) {
	if page.Limit < 0 || page.Limit > 200 || (page.Cursor != "" && !validID(page.Cursor)) {
		return domain.Page{}, domain.ErrInvalid
	}
	if page.Limit == 0 {
		page.Limit = 50
	}
	return page, nil
}

// ListJobs 验证筛选条件并查询任务，为结果补充可用操作及耗时。
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

// GetJob 验证任务 ID，读取状态并计算当前允许操作和耗时。
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

// decorate 按任务状态补充操作列表，使用结束时间或当前时间计算非负耗时。
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

// actions 根据任务状态、待处理控制和模块能力计算当前允许的操作。
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

// ListFailedItems 确认任务存在并校验分页后读取失败条目。
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

// ListLogs 确认任务存在并校验分页后读取管理员可见日志。
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

// newJob 以当前媒体库名称、提交来源和 UTC 时刻构造新的排队任务。
func (s *Service) newJob(kind domain.Type, lib library.Library, source domain.Source, requestID string) domain.Job {
	return domain.Job{
		ID: rand.Text(), Type: kind, LibraryID: lib.ID, LibraryName: lib.Name,
		Source: source, Status: domain.Queued, CreatedAt: s.now().UTC(), RequestID: requestID,
	}
}

// Submit 校验类型、模块和媒体库资格，再持久化手动任务。
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

// submit 保存任务并记录结果；成功后发布变更提示并唤醒工作协程。
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

// Control 验证暂停、恢复或取消动作，先持久化条件变更，再通知匹配的运行轮次。
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

// Retry 按模块能力创建关联原任务的新运行，支持整轮或仅失败条目重试。
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

// log 记录带任务、媒体库、执行次数及关联标识的操作结果，不输出原始底层错误。
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

// Subscribe 创建有界变更提示订阅并返回释放函数；达到上限或停止服务时拒绝，慢订阅需重连回读历史。
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

// setEventsClosed 在锁内切换事件接收状态，关闭时释放全部订阅通道。
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

// publish 非阻塞发布变更提示，关闭缓冲已满的慢订阅以避免阻塞工作协程。
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

// closeSubscription 在调用方持有事件锁时幂等移除并关闭订阅。
func (s *Service) closeSubscription(subscription chan domain.Event) {
	if _, exists := s.events[subscription]; exists {
		delete(s.events, subscription)
		close(subscription)
	}
}
