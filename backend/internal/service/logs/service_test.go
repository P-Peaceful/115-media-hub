// service_test.go：覆盖采集批量写入、清理事件顺序、队列背压、订阅上限和终止行为。

package logs

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/logs"
)

// memoryRepository 由测试服务的单写入协程访问；测试在关闭同步点后可直接检查结果。
type memoryRepository struct {
	entries []domain.Entry
	audit   *auth.Audit
	nextID  int64
	insert  func(context.Context) error
	prune   func(context.Context) error
	remove  func(time.Time, auth.Audit) error
}

// InsertRuntimeLogs 模拟提交前的外部等待或故障，仅提交成功后分配可发布 ID。
func (r *memoryRepository) InsertRuntimeLogs(ctx context.Context, entries []domain.Entry) ([]domain.Entry, error) {
	if r.insert != nil {
		if err := r.insert(ctx); err != nil {
			return nil, err
		}
	}
	inserted := make([]domain.Entry, len(entries))
	for i, entry := range entries {
		r.nextID++
		entry.ID = r.nextID
		r.entries = append(r.entries, entry)
		inserted[i] = entry
	}
	return inserted, nil
}

func (*memoryRepository) ListRuntimeLogs(context.Context, domain.Filter) ([]domain.Entry, error) {
	return nil, nil
}

func (*memoryRepository) RuntimeLogExportBoundary(context.Context, domain.Filter, int) (int64, bool, error) {
	return 0, false, nil
}

// ClearRuntimeLogs 模拟原子清理和审计，故障时保留原有条目。
func (r *memoryRepository) ClearRuntimeLogs(_ context.Context, before time.Time, audit auth.Audit) (int64, error) {
	if r.remove != nil {
		if err := r.remove(before, audit); err != nil {
			return 0, err
		}
	}
	count := len(r.entries)
	r.entries = nil
	r.audit = &audit
	return int64(count), nil
}

func (r *memoryRepository) PruneRuntimeLogs(ctx context.Context, _ time.Time, _, _ int) (int64, error) {
	if r.prune != nil {
		return 0, r.prune(ctx)
	}
	return 0, nil
}

// TestServicePublishesCommittedTaskLogsAndClearsInOrder 验证任务记录先落库再广播，清理等待已有写入并通知所有订阅者。
func TestServicePublishesCommittedTaskLogsAndClearsInOrder(t *testing.T) {
	insertStarted, releaseInsert := make(chan struct{}), make(chan struct{})
	repo := &memoryRepository{insert: func(ctx context.Context) error {
		select {
		case <-insertStarted:
		default:
			close(insertStarted)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-releaseInsert:
			return nil
		}
	}}
	base := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	service := New(repo, base)
	t.Cleanup(func() { stopService(t, service) })
	events, release, err := service.Subscribe(domain.Filter{Module: "strm", JobID: "job-test", Query: "摘要"})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	now := time.Now().UTC()
	service.CaptureJobLog(jobs.Job{ID: "job-test", Type: jobs.STRM, RequestID: "req-1", Attempt: 1},
		jobs.JobLog{Level: "info", Message: "中文摘要", CreatedAt: now})
	await(t, insertStarted)
	select {
	case event := <-events:
		t.Fatalf("尚未提交时提前发布: %+v", event)
	default:
	}
	close(releaseInsert)
	if event := await(t, events); event.Entry == nil || event.Entry.ID != 1 || event.Entry.RequestID != "req-1" ||
		event.Entry.Message != "中文摘要" || !event.Entry.OccurredAt.Equal(now) {
		t.Fatalf("任务记录事件不符合预期: %+v", event.Entry)
	}
	service.CaptureJobLog(jobs.Job{ID: "ignored", Type: jobs.STRM},
		jobs.JobLog{Level: "warn", Message: "其他任务的摘要", CreatedAt: now})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	result, err := service.Clear(ctx, "all", "administrator-test", "req-clear")
	if err != nil || result.DeletedCount != 2 || result.Scope != "all" {
		t.Fatalf("清理未包含队列中已有记录: %+v, %v", result, err)
	}
	if event := await(t, events); event.Cleared == nil || *event.Cleared != result {
		t.Fatalf("清理事件有误: %+v", event)
	}
	stopService(t, service)
	if repo.audit == nil || repo.audit.ActorID != "administrator-test" || repo.audit.Action != "clear_runtime_logs" ||
		repo.audit.CorrelationID != "req-clear" || repo.audit.TargetID != "all" {
		t.Fatalf("审计记录不完整: %+v", repo.audit)
	}
	if len(repo.entries) != 1 || repo.entries[0].Operation != "clear" || repo.entries[0].Message != "运行日志清理完成" {
		t.Fatalf("清空后只应出现新的清理结果记录: %+v", repo.entries)
	}
}

// TestClearFailureRetainsHistoryAndDoesNotPublish 验证审计失败时清理不发布变更，且只产生一次可诊断终态。
func TestClearFailureRetainsHistoryAndDoesNotPublish(t *testing.T) {
	fault := errors.New("injected failure")
	repo := &memoryRepository{remove: func(before time.Time, audit auth.Audit) error {
		if before != audit.OccurredAt.Add(-7*24*time.Hour) {
			return errors.New("unexpected cutoff")
		}
		return fault
	}}
	var output bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&output, nil))
	service := New(repo, base)
	t.Cleanup(func() { stopService(t, service) })
	events, release, err := service.Subscribe(domain.Filter{Module: "scan"})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	service.WrapLogger(base).Info("保留记录", "module", "scan")
	if _, err := service.Clear(t.Context(), "before_7d", "admin", "request"); !errors.Is(err, fault) {
		t.Fatalf("清理应传播事务故障: %v", err)
	}
	if event := await(t, events); event.Entry == nil || event.Entry.Message != "保留记录" {
		t.Fatalf("期望只收到已持久化记录: %+v", event)
	}
	select {
	case event := <-events:
		t.Fatalf("清理失败不应发布清理事件: %+v", event)
	default:
	}
	stopService(t, service)
	if repo.audit != nil || len(repo.entries) != 2 || repo.entries[1].ErrorCode != "runtime_log_clear_failed" ||
		strings.Count(output.String(), "runtime_log_clear_failed") != 1 || strings.Contains(output.String(), fault.Error()) {
		t.Fatalf("清理故障日志或事务结果不符合预期: %+v / %s", repo.entries, &output)
	}
}

// TestCollectorDropsWhenQueueFullWithoutWaitingOnStorage 验证数据库繁忙时业务调用不会等待日志刷盘，关闭后正常排空。
func TestCollectorDropsWhenQueueFullWithoutWaitingOnStorage(t *testing.T) {
	pruning, release := make(chan struct{}), make(chan struct{})
	repo := &memoryRepository{prune: func(ctx context.Context) error {
		select {
		case <-pruning:
		default:
			close(pruning)
		}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	var output bytes.Buffer
	service := New(repo, slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { stopService(t, service) })
	await(t, pruning)
	added := make(chan struct{})
	go func() {
		base := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
		logger := service.WrapLogger(base)
		for range queueSize + 16 {
			logger.Info("应用事件", "module", "app")
		}
		close(added)
	}()
	await(t, added)
	if got := service.dropped.Load(); got != 16 {
		t.Fatalf("队列应丢弃超出容量的事件: %d", got)
	}
	close(release)
	stopService(t, service)
	if len(repo.entries) != queueSize || !strings.Contains(output.String(), `"error_code":"log_queue_full","dropped_count":16`) {
		t.Fatalf("关闭刷盘或降载诊断不正确: %d, %s", len(repo.entries), &output)
	}
}

// TestSubscribeDisconnectsSlowReadersAndLimitsConnections 验证慢客户端断开且其他订阅者能继续消费事件。
func TestSubscribeDisconnectsSlowReadersAndLimitsConnections(t *testing.T) {
	service := New(&memoryRepository{}, slog.New(slog.DiscardHandler))
	t.Cleanup(func() { stopService(t, service) })
	slow, release, err := service.Subscribe(domain.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	fast, releaseFast, err := service.Subscribe(domain.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFast()
	for range maxSubscribers - 2 {
		if _, _, err := service.Subscribe(domain.Filter{JobID: "none"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := service.Subscribe(domain.Filter{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("达到连接上限后应拒绝订阅: %v", err)
	}
	for i := range streamBuffer + 1 {
		entry := domain.Entry{ID: int64(i + 1)}
		service.streams.publish(Event{Entry: &entry})
		if got := await(t, fast); got.Entry == nil || got.Entry.ID != entry.ID {
			t.Fatalf("正常客户端未收到事件 %d", entry.ID)
		}
	}
	if _, ok := <-slow; ok {
		t.Fatal("慢客户端应已关闭")
	}
	service.StopStreams()
	if _, ok := <-fast; ok {
		t.Fatal("停止实时流后所有连接应关闭")
	}
	if _, _, err := service.Subscribe(domain.Filter{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("停止实时流后应拒绝新订阅: %v", err)
	}
}

// stopService 用独立的有界上下文等待异步刷盘，不依赖测试退出前已取消的上下文。
func stopService(t *testing.T, service *Service) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := service.Close(ctx); err != nil {
		t.Fatalf("关闭日志服务: %v", err)
	}
}

// await 等待测试同步点并设置超时，避免依赖异步处理速度或固定睡眠。
func await[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("等待日志测试同步点超时")
		var zero T
		return zero
	}
}
