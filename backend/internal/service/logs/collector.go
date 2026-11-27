// collector.go：保留既有 slog 出口，并将符合筛选的脱敏结构化日志交给异步采集服务。

package logs

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"math"
	"reflect"
	"time"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/logs"
)

const maxDetailsBytes = 2048

// detailKeys 只放行既有业务事件使用的诊断字段；原始凭据、载荷、链接和任意嵌套对象均不采集。
var detailKeys = []string{
	"actor_id", "artifact_id", "attempt", "attempt_count", "bytes", "candidate_count", "candidate_files",
	"channel", "completed_directories", "component", "conflicting_files", "connection_id", "content_length",
	"cooldown_blocked", "deleted_count", "directory_id", "directory_limit", "dropped_count", "duration_ms", "exported_count",
	"failed_count", "failed_directories", "failed_files", "failed_items", "fallback_reason", "file_id",
	"file_name", "file_path", "file_size", "file_type", "filename", "format", "generated_files", "jobs_created",
	"library_id", "method", "next_channel", "outcome", "path", "pending_files", "phase", "reason",
	"request_limit", "route", "sample_limit", "scan_complete", "scan_mode", "scanned_videos", "schedule_id",
	"scheduled_for", "scope", "scope_complete", "size_bytes", "skipped_files", "skipped_items", "snapshot_mode",
	"source_directory_id", "source_file_id", "status", "storage_operation", "storage_reason", "succeeded_items",
	"suppressed_failure_logs", "task_type", "total_bytes", "total_items", "truncated", "upstream_status", "wait_ms",
}

var captureKeys = func() map[string]struct{} {
	keys := make(map[string]struct{}, len(detailKeys)+5)
	for _, key := range detailKeys {
		keys[key] = struct{}{}
	}
	for _, key := range []string{"module", "operation", "job_id", "request_id", "error_code"} {
		keys[key] = struct{}{}
	}
	return keys
}()

type collectorHandler struct {
	next    slog.Handler
	service *Service
	attrs   map[string]any
	grouped bool
}

// WrapLogger 添加运行日志采集，同时沿用原有输出、级别和派生上下文。
func (s *Service) WrapLogger(logger *slog.Logger) *slog.Logger {
	return slog.New(&collectorHandler{next: logger.Handler(), service: s})
}

// Enabled 沿用原有日志级别策略；运行日志采集不恢复被配置禁用的记录。
func (h *collectorHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle 原样转发到原输出，并将已脱敏且可安全序列化的记录投递到有界队列。
func (h *collectorHandler) Handle(ctx context.Context, record slog.Record) error {
	entry, collected := h.entry(record)
	if collected {
		h.service.enqueue(entry)
	}
	return h.next.Handle(ctx, record)
}

// WithAttrs 在新的 handler 上合并顶层白名单字段，不修改父 handler 的属性。
func (h *collectorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := h.next.WithAttrs(attrs)
	copy := *h
	copy.next = next
	if !h.grouped {
		copy.attrs = make(map[string]any, len(h.attrs)+len(attrs))
		maps.Copy(copy.attrs, h.attrs)
		for _, attr := range attrs {
			captureAttr(copy.attrs, attr)
		}
	}
	return &copy
}

// WithGroup 保持原始日志分组语义；具名分组中的属性不晋升为公共运行日志字段。
func (h *collectorHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	copy := *h
	copy.next = h.next.WithGroup(name)
	copy.grouped = true
	return &copy
}

// entry 规范化日志级别和模块，去掉健康检查、内部诊断与低价值访问记录。
func (h *collectorHandler) entry(record slog.Record) (domain.Entry, bool) {
	attrs := make(map[string]any, len(h.attrs)+record.NumAttrs())
	maps.Copy(attrs, h.attrs)
	if !h.grouped {
		record.Attrs(func(attr slog.Attr) bool {
			captureAttr(attrs, attr)
			return true
		})
	}
	if attrString(attrs, "outcome") == "canceled" {
		return domain.Entry{}, false
	}
	module, operation := attrString(attrs, "module"), attrString(attrs, "operation")
	level := record.Level
	switch module {
	case "health":
		return domain.Entry{}, false
	case "http":
		// 访问日志保留在 stdout/轮转文件；运行日志页只展示业务结果和未捕获 panic。
		if record.Message != "request panic" {
			return domain.Entry{}, false
		}
	case "logging":
		if operation != "export" && operation != "clear" {
			return domain.Entry{}, false
		}
	}
	occurredAt := record.Time.UTC()
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return domain.Entry{
		OccurredAt: occurredAt, Level: normalizeLevel(level), Module: normalizeModule(module),
		Operation: operation, JobID: attrString(attrs, "job_id"), RequestID: attrString(attrs, "request_id"),
		ErrorCode: attrString(attrs, "error_code"), Message: localizeMessage(record.Message, operation, attrs),
		Details: captureDetails(attrs),
	}, true
}

// captureAttr 仅提取顶层白名单标量，并展开 slog 的匿名内联分组。
func captureAttr(attrs map[string]any, attr slog.Attr) {
	if attr.Key == "" {
		if value := attr.Value.Resolve(); value.Kind() == slog.KindGroup {
			for _, child := range value.Group() {
				captureAttr(attrs, child)
			}
		}
		return
	}
	if _, allowed := captureKeys[attr.Key]; !allowed {
		return
	}
	if value := captureScalar(attr.Value.Resolve().Any()); value != nil {
		attrs[attr.Key] = value
	}
}

// captureScalar 仅复制安全标量及其命名类型，不调用任意对象的字符串或 JSON 方法。
func captureScalar(value any) any {
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.String:
		return sanitizeText(reflected.String(), maxAttributeRunes)
	case reflect.Bool:
		return reflected.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflected.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return reflected.Uint()
	case reflect.Float32, reflect.Float64:
		number := reflected.Float()
		if !math.IsNaN(number) && !math.IsInf(number, 0) {
			return number
		}
	}
	return nil
}

// captureDetails 在 JSON 字节预算内复制允许的属性，避免单条日志挤满队列和导出响应。
func captureDetails(attrs map[string]any) map[string]any {
	details := make(map[string]any)
	size := 2 // JSON 对象的左右括号。
	for _, key := range detailKeys {
		value, exists := attrs[key]
		if !exists {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			continue
		}
		if size+len(key)+4+len(encoded) > maxDetailsBytes {
			continue
		}
		details[key] = value
		size += len(key) + 4 + len(encoded) // 引号、冒号和逗号。
	}
	return details
}

func attrString(attrs map[string]any, key string) string {
	value, _ := attrs[key].(string)
	return value
}

// normalizeLevel 将自定义 slog 级别归入运行日志契约定义的四级。
func normalizeLevel(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "error"
	case level >= slog.LevelWarn:
		return "warn"
	case level >= slog.LevelInfo:
		return "info"
	default:
		return "debug"
	}
}

// normalizeModule 将启动、访问异常、运行日志维护等事件统一归到系统模块。
func normalizeModule(module string) string {
	switch module {
	case "sync", "scan", "strm", "upload", "jobs", "library", "provider_115", "auth":
		return module
	default:
		return "system"
	}
}
