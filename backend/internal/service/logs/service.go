// service.go：管理运行日志的有界采集、批量刷盘、保留策略及清理审计顺序。

package logs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/logs"
)

const (
	queueSize       = 2048
	batchSize       = 128
	flushInterval   = 200 * time.Millisecond
	storageTimeout  = 5 * time.Second
	retention       = 30 * 24 * time.Hour
	maxRows         = 50_000
	pruneBatch      = 1000
	pruneInterval   = 10 * time.Minute
	dropLogInterval = 30 * time.Second
)

var (
	// ErrInvalidScope 表示客户端请求了未定义的清理范围。
	ErrInvalidScope = errors.New("invalid runtime log clear scope")
	// ErrUnavailable 表示服务正在关闭或订阅已达上限。
	ErrUnavailable = errors.New("runtime logs unavailable")
)

// Repository 提供已脱敏日志的存储操作；清理必须和审计在同一个事务提交。
type Repository interface {
	InsertRuntimeLogs(context.Context, []domain.Entry) ([]domain.Entry, error)
	ListRuntimeLogs(context.Context, domain.Filter) ([]domain.Entry, error)
	RuntimeLogExportBoundary(context.Context, domain.Filter, int) (int64, bool, error)
	ClearRuntimeLogs(context.Context, time.Time, auth.Audit) (int64, error)
	PruneRuntimeLogs(context.Context, time.Time, int, int) (int64, error)
}

// ClearResult 描述一次已经提交的清理及其实际删除条数。
type ClearResult struct {
	DeletedCount int64
	Scope        string
}

type clearRequest struct {
	ctx       context.Context
	scope     string
	actorID   string
	requestID string
	reply     chan clearReply
}

type clearReply struct {
	result ClearResult
	err    error
}

// Service 拥有一个后台写入协程；业务日志投递不等待数据库或订阅者。
type Service struct {
	repo Repository
	base *slog.Logger

	admission sync.RWMutex
	stopping  bool
	queue     chan domain.Entry
	dropped   atomic.Uint64
	clears    chan clearRequest
	done      chan struct{}
	cancel    context.CancelFunc
	closeErr  error // 仅由写入协程写入；在 done 关闭后读取。

	streams broker
}

// New 启动日志写入协程；调用者须先 Close 再释放仓储。
func New(repo Repository, baseLogger *slog.Logger) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	service := &Service{
		repo: repo, base: baseLogger,
		queue: make(chan domain.Entry, queueSize), clears: make(chan clearRequest),
		done: make(chan struct{}), cancel: cancel,
		streams: broker{subscribers: make(map[*subscriber]struct{})},
	}
	go service.run(ctx)
	return service
}

// List 查询符合指定筛选和游标的已持久化日志。
func (s *Service) List(ctx context.Context, filter domain.Filter) ([]domain.Entry, error) {
	return s.repo.ListRuntimeLogs(ctx, filter)
}

// ExportBoundary 捕获导出的最大记录 ID 及该快照在指定筛选下是否需要截断。
func (s *Service) ExportBoundary(ctx context.Context, filter domain.Filter, limit int) (int64, bool, error) {
	return s.repo.RuntimeLogExportBoundary(ctx, filter, limit)
}

// CaptureJobLog 收集任务中心已接受的业务摘要，保留任务关联和原始事件时间。
func (s *Service) CaptureJobLog(job jobs.Job, entry jobs.JobLog) {
	module := "jobs"
	switch job.Type {
	case jobs.Scan, jobs.Sync, jobs.STRM, jobs.Upload:
		module = string(job.Type)
	}
	details := map[string]any{"task_type": sanitizeText(string(job.Type), maxAttributeRunes), "attempt": job.Attempt}
	if job.LibraryID != "" {
		details["library_id"] = sanitizeText(job.LibraryID, maxAttributeRunes)
	}
	s.enqueue(domain.Entry{
		OccurredAt: entry.CreatedAt.UTC(), Level: entry.Level,
		Module: module, Operation: "execute", JobID: sanitizeText(job.ID, maxAttributeRunes),
		RequestID: sanitizeText(job.RequestID, maxAttributeRunes), Message: sanitizeText(entry.Message, maxMessageRunes),
		Details: details,
	})
}

// enqueue 在队列满时只增加丢弃计数；锁仅保护关闭与发送的交接。
func (s *Service) enqueue(entry domain.Entry) {
	s.admission.RLock()
	defer s.admission.RUnlock()
	if s.stopping {
		return
	}
	select {
	case s.queue <- entry:
	default:
		s.dropped.Add(1)
	}
}

// Clear 由采集协程顺序处理，先刷写已接收日志，再提交清理事务并通知订阅者。
func (s *Service) Clear(ctx context.Context, scope, actorID, requestID string) (ClearResult, error) {
	switch scope {
	case "before_7d", "before_30d", "all":
	default:
		return ClearResult{}, ErrInvalidScope
	}
	request := clearRequest{ctx: ctx, scope: scope, actorID: actorID, requestID: requestID, reply: make(chan clearReply, 1)}
	select {
	case <-ctx.Done():
		return ClearResult{}, ctx.Err()
	case <-s.done:
		return ClearResult{}, ErrUnavailable
	case s.clears <- request:
	}

	select {
	case <-ctx.Done():
		return ClearResult{}, ctx.Err()
	case reply := <-request.reply:
		return reply.result, reply.err
	}
}

// Close 停止接收新日志、关闭实时流并等待所有已接收批次写回仓储。
func (s *Service) Close(ctx context.Context) error {
	s.StopStreams()
	s.admission.Lock()
	if !s.stopping {
		s.stopping = true
		close(s.queue)
	}
	s.admission.Unlock()
	select {
	case <-s.done:
		return s.closeErr
	default:
	}
	select {
	case <-s.done:
		return s.closeErr
	case <-ctx.Done():
		s.cancel()
		return ctx.Err()
	}
}

// run 串行执行批量写入、清理和裁剪，确保先持久化再投递且清理事件顺序一致。
func (s *Service) run(ctx context.Context) {
	defer close(s.done)
	defer s.cancel()
	flushTicker := time.NewTicker(flushInterval)
	pruneTicker := time.NewTicker(pruneInterval)
	diagnosticTicker := time.NewTicker(dropLogInterval)
	defer flushTicker.Stop()
	defer pruneTicker.Stop()
	defer diagnosticTicker.Stop()
	prunePending := s.prune(ctx)
	persisted := 0
	batch := make([]domain.Entry, 0, batchSize)
	var failedEntries uint64
	var lastWriteReport time.Time
	reportWriteFailures := func(now time.Time) {
		if failedEntries == 0 || now.Sub(lastWriteReport) < dropLogInterval {
			return
		}
		s.base.Error("运行日志批次写入失败", "module", "logging", "operation", "flush",
			"outcome", "failed", "error_code", "runtime_log_write_failed", "dropped_count", failedEntries)
		failedEntries = 0
		lastWriteReport = now
	}
	flush := func() {
		count, err := s.flush(ctx, batch)
		if err != nil {
			failedEntries += uint64(len(batch))
			reportWriteFailures(time.Now())
			if s.isStopping() {
				s.closeErr = err
			}
		}
		clear(batch)
		batch = batch[:0]
		persisted += count
	}
	var lastDropReport time.Time
	for {
		select {
		case <-ctx.Done():
			s.closeErr = ctx.Err()
			return
		case entry, ok := <-s.queue:
			if !ok {
				flush()
				reportWriteFailures(time.Now())
				if time.Since(lastDropReport) >= dropLogInterval {
					s.reportDrops()
				}
				return
			}
			batch = append(batch, entry)
			if len(batch) == batchSize {
				flush()
			}
		case <-flushTicker.C:
			flush()
			if prunePending {
				prunePending = s.prune(ctx)
			}
		case <-pruneTicker.C:
			prunePending = s.prune(ctx)
			persisted = 0
		case <-diagnosticTicker.C:
			reportWriteFailures(time.Now())
			if s.reportDrops() {
				lastDropReport = time.Now()
			}
		case request := <-s.clears:
			if s.isStopping() {
				request.reply <- clearReply{err: ErrUnavailable}
				continue
			}
			if err := request.ctx.Err(); err != nil {
				request.reply <- clearReply{err: err}
				continue
			}
			// 固定此次清理前的队列长度，使等待中的采集不在清空之后重新出现。
			pending := len(s.queue)
			for range pending {
				batch = append(batch, <-s.queue)
				if len(batch) == batchSize {
					flush()
				}
			}
			flush()
			result, err := s.clear(ctx, request)
			request.reply <- clearReply{result: result, err: err}
		}
		if persisted >= pruneBatch {
			prunePending = s.prune(ctx)
			persisted = 0
		}
	}
}

// isStopping 判断刷盘失败是否发生在调用者要求关闭后。
func (s *Service) isStopping() bool {
	s.admission.RLock()
	defer s.admission.RUnlock()
	return s.stopping
}

// flush 批量写入带持久化 ID 的日志，然后投递给当前订阅者。
func (s *Service) flush(ctx context.Context, batch []domain.Entry) (int, error) {
	if len(batch) == 0 {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(ctx, storageTimeout)
	defer cancel()
	entries, err := s.repo.InsertRuntimeLogs(ctx, batch)
	if err != nil {
		return 0, fmt.Errorf("flush runtime logs: %w", err)
	}
	for _, entry := range entries {
		s.streams.publish(Event{Entry: &entry})
	}
	return len(entries), nil
}

// prune 执行一次有界裁剪，达到批量上限时请写入循环继续清理积压。
func (s *Service) prune(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, storageTimeout)
	defer cancel()
	count, err := s.repo.PruneRuntimeLogs(ctx, time.Now().UTC().Add(-retention), maxRows, pruneBatch)
	if err != nil {
		s.base.Warn("运行日志保留裁剪失败", "module", "logging", "operation", "prune",
			"outcome", "failed", "error_code", "runtime_log_prune_failed")
		return false
	}
	return count >= pruneBatch
}

// reportDrops 汇总队列满时丢弃的记录；诊断仅交给原始日志出口。
func (s *Service) reportDrops() bool {
	count := s.dropped.Swap(0)
	if count == 0 {
		return false
	}
	s.base.Warn("运行日志采集队列已满", "module", "logging", "operation", "ingest",
		"outcome", "failed", "error_code", "log_queue_full", "dropped_count", count)
	return true
}

// clear 计算删除边界，将变更和审计一起提交后发布清理事件，并记录一次终态。
func (s *Service) clear(workerCtx context.Context, request clearRequest) (ClearResult, error) {
	ctx, cancel := context.WithCancel(request.ctx)
	stop := context.AfterFunc(workerCtx, cancel)
	defer stop()
	defer cancel()
	started := time.Now()
	now := started.UTC()
	var before time.Time
	switch request.scope {
	case "before_7d":
		before = now.Add(-7 * 24 * time.Hour)
	case "before_30d":
		before = now.Add(-retention)
	}
	attrs := []any{"module", "logging", "operation", "clear", "scope", request.scope, "request_id", request.requestID}
	s.base.InfoContext(ctx, "运行日志清理开始", attrs...)
	audit := auth.Audit{
		ActorID: request.actorID, Action: "clear_runtime_logs", TargetID: request.scope,
		Outcome: "succeeded", CorrelationID: request.requestID, OccurredAt: now,
	}
	count, err := s.repo.ClearRuntimeLogs(ctx, before, audit)
	attrs = append(attrs, "duration_ms", time.Since(started).Milliseconds())
	logger := s.WrapLogger(s.base)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			logger.InfoContext(ctx, "运行日志清理已取消", append(attrs, "outcome", "canceled")...)
		} else {
			logger.WarnContext(ctx, "运行日志清理失败", append(attrs, "outcome", "failed", "error_code", "runtime_log_clear_failed")...)
		}
		return ClearResult{}, fmt.Errorf("clear runtime logs: %w", err)
	}
	result := ClearResult{DeletedCount: count, Scope: request.scope}
	s.streams.publish(Event{Cleared: &result})
	logger.InfoContext(ctx, "运行日志清理完成", append(attrs, "outcome", "succeeded", "deleted_count", count)...)
	return result, nil
}
