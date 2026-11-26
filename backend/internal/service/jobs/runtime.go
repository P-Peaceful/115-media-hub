// runtime.go：管理有界工作池和调度循环，处理任务取消、重启核验及脱敏上报的生命周期。

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

// pollInterval 控制调度与空闲工作协程的轮询间隔，显式唤醒可提前触发抢占。
const pollInterval = time.Second

// Start 先核验遗留运行任务并推进停机期间的计划，再启动两个工作协程和一个调度协程。
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

// Stop 关闭订阅并取消后台工作，在调用方期限内等待协程释放资源。
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

// scheduler 按固定间隔处理到期计划，退出时释放定时器并停止产生新任务。
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

// worker 循环条件抢占任务并串行执行单个任务，空队列时等待轮询、唤醒或取消。
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

// wakeWorker 发送可合并的唤醒信号，不阻塞任务提交或状态迁移。
func (s *Service) wakeWorker() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// runAttempt 以任务 ID 和执行次数共同标识活动执行，隔离过期控制信号。
type runAttempt struct {
	jobID  string
	number int
}

// signalControl 只取消与任务执行次数匹配的活动上下文，旧执行控制不能影响恢复后的任务。
func (s *Service) signalControl(job domain.Job, action domain.Action) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	if stop, ok := s.active[runAttempt{job.ID, job.Attempt}]; ok {
		stop(controlCause(action))
	}
}

// controlCause 将暂停映射为专用原因，其余停止动作使用普通取消。
func controlCause(action domain.Action) error {
	if action == domain.Pause {
		return ErrPauseRequested
	}
	return context.Canceled
}

// run 登记活动执行并复读控制状态，解决控制先于取消句柄注册的竞态，再加载当前配置执行模块。
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
	// 控制命令可能先于工作协程的取消句柄完成持久化，因此登记后需要复读。
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

// runModule 调用业务模块，将 panic 转为固定执行错误，避免崩溃信息进入任务历史。
func runModule(ctx context.Context, module Module, execution Execution) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("task module panicked")
		}
	}()
	return module.Run(ctx, execution)
}

// execute 执行任务后使用独立短期限保存终态，优先处理已持久化控制；停机中断留待重启核验。
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

// reconcile 逐个核验数据库中遗留的运行任务，在工作池启动前保存恢复决策。
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

// reconcileJob 先由模块核验副作用和检查点，再按控制状态决定重排、暂停或取消；无法核验则失败。
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

// verifyModule 调用模块恢复核验，将 panic 转为固定错误而不泄露模块内部信息。
func verifyModule(ctx context.Context, module Module, execution Execution) (_ bool, err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("task recovery module panicked")
		}
	}()
	return module.Recover(ctx, execution)
}

// finish 提交执行状态并发布变更或完成通知，记录带分类计数的状态变更结果。
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

// sensitiveText 识别模块自由文本中常见的凭据和 URL 形式；仅作额外遮挡，模块仍须自行提供安全摘要。
var sensitiveText = regexp.MustCompile(`(?i)(https?://|\b(?:cookie|authorization|password|access[_-]?token|refresh[_-]?token|pickcode|uid|cid|seid|kid)\s*[:=]|\bbearer\s+)`)

// safeText 隐藏匹配常见凭据或 URL 形式的文本，替换控制字符并按 UTF-8 边界限制字节长度。
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

// execution 构造绑定任务执行次数的上报回调，校验并清理管理员摘要，返回需调用的更新资源释放函数。
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
		entry := domain.JobLog{ID: rand.Text(), Level: level, Message: safeText(message, 2048), CreatedAt: s.now().UTC()}
		if err := s.repo.InsertJobLog(ctx, job.ID, job.Attempt, entry); err != nil {
			return err
		}
		if s.observeLog != nil {
			s.observeLog(job, entry)
		}
		updates.report()
		return nil
	}
	if job.ParentID != nil && job.RetryScope == domain.RetryFailures {
		execution.RetryItems = func(ctx context.Context, page domain.Page) (domain.ItemPage, error) {
			return s.ListFailedItems(ctx, *job.ParentID, page)
		}
	}
	return execution, updates.close
}

// executionUpdates 将运行中变更提示合并到每秒最多一次，并用定时器补发静默前最后一次提示；终态单独发布。
type executionUpdates struct {
	mu      sync.Mutex
	publish func()
	next    time.Time
	timer   *time.Timer
	closed  bool
}

// report 在锁内发送或预约合并提示；已有待发定时器时复用该次提示。
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

// flush 在锁内清除已触发定时器，尚未关闭时发布尾部变更提示。
func (u *executionUpdates) flush() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.timer = nil
	if !u.closed {
		u.next = time.Now().Add(time.Second)
		u.publish()
	}
}

// close 标记上报资源关闭并停止未触发定时器，后续报告不再发送提示。
func (u *executionUpdates) close() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.closed = true
	if u.timer != nil {
		u.timer.Stop()
	}
}

// invalidProgress 拒绝非数值或越界的百分比，以及负速度和负条目总数。
func invalidProgress(progress domain.Progress) bool {
	return (progress.ProgressPercent != nil && (math.IsNaN(*progress.ProgressPercent) || *progress.ProgressPercent < 0 || *progress.ProgressPercent > 100)) ||
		(progress.SpeedBytesPerSecond != nil && *progress.SpeedBytesPerSecond < 0) ||
		(progress.TotalItems != nil && *progress.TotalItems < 0)
}
