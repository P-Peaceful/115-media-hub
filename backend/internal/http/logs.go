// logs.go：注册运行日志管理接口，统一校验筛选条件并转换持久日志和清理结果。

package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/logs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/logs"
)

type runtimeLogHandler struct {
	service *logs.Service
	auth    *auth.Service
	logger  *slog.Logger
}

// register 将运行日志路由挂到统一管理边界下，共用管理员会话与写请求 CSRF 保护。
func (h *runtimeLogHandler) register(router *gin.Engine) {
	router.GET("/api/v1/logs", h.list)
	router.GET("/api/v1/logs/stream", h.stream)
	router.GET("/api/v1/logs/export", h.export)
	router.POST("/api/v1/logs/clear", h.clear)
}

// list 多取一条以确定是否存在下一页，游标指向当前页最后一条记录。
func (h *runtimeLogHandler) list(c *gin.Context) {
	query, ok := runtimeLogQuery(c)
	if !ok {
		return
	}
	filter, ok := runtimeLogFilter(c, query, false)
	if !ok {
		return
	}
	limit := 50
	if query.Has("limit") {
		var err error
		limit, err = strconv.Atoi(query.Get("limit"))
		if err != nil || limit < 1 || limit > 200 || strconv.Itoa(limit) != query.Get("limit") {
			writeError(c, http.StatusBadRequest, generated.ValidationFailed, "日志分页条数无效")
			return
		}
	}
	if query.Has("cursor") {
		filter.Before, ok = decodeRuntimeLogCursor(query.Get("cursor"))
		if !ok {
			writeError(c, http.StatusBadRequest, generated.ValidationFailed, "日志分页游标无效")
			return
		}
	}
	filter.Limit = limit + 1
	entries, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		if ctx := c.Request.Context(); ctx.Err() == nil {
			h.logger.WarnContext(ctx, "runtime log list unavailable", "module", "logging", "operation", "list",
				"request_id", c.GetString("request_id"), "outcome", "failed", "error_code", "runtime_log_read_failed")
		}
		h.unavailable(c)
		return
	}
	var next *string
	if len(entries) > limit {
		entries = entries[:limit]
		next = new(encodeRuntimeLogCursor(entries[limit-1]))
	}
	items := make([]generated.RuntimeLogEntry, 0, len(entries))
	for _, entry := range entries {
		items = append(items, runtimeLogResponse(entry))
	}
	c.JSON(http.StatusOK, generated.RuntimeLogPage{Items: items, NextCursor: next})
}

// clear 使用管理员身份执行日志清理，成功响应只能在删除和审计提交后发送。
func (h *runtimeLogHandler) clear(c *gin.Context) {
	var input generated.ClearRuntimeLogsRequest
	if !decodeAuthBody(c, &input, []string{"scope"}) {
		return
	}
	if !input.Scope.Valid() {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "日志清理范围无效")
		return
	}
	identity := providerIdentity(c)
	result, err := h.service.Clear(c.Request.Context(), string(input.Scope), identity.Administrator.ID, c.GetString("request_id"))
	if errors.Is(err, logs.ErrInvalidScope) {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "日志清理范围无效")
		return
	}
	if err != nil {
		h.unavailable(c)
		return
	}
	c.JSON(http.StatusOK, runtimeLogClearResponse(result))
}

// unavailable 在请求仍有效时返回脱敏的服务错误，存储细节留在后端。
func (h *runtimeLogHandler) unavailable(c *gin.Context) {
	if c.Request.Context().Err() == nil {
		writeError(c, http.StatusServiceUnavailable, generated.ServiceUnavailable, "运行日志暂时不可用")
	}
}

// runtimeLogQuery 拒绝损坏的查询编码和重复的筛选项。
func runtimeLogQuery(c *gin.Context) (url.Values, bool) {
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err == nil {
		for _, key := range []string{"range", "level", "module", "job_id", "q", "limit", "cursor", "format"} {
			if len(query[key]) > 1 {
				err = errors.New("duplicate runtime log parameter")
				break
			}
		}
	}
	if err != nil {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "日志查询参数无效")
		return nil, false
	}
	return query, true
}

// runtimeLogFilter 让历史查询、导出和实时订阅使用同一组字段校验，实时订阅只接收新记录。
func runtimeLogFilter(c *gin.Context, query url.Values, live bool) (domain.Filter, bool) {
	filter := domain.Filter{Level: query.Get("level"), Module: query.Get("module"), JobID: query.Get("job_id"), Query: query.Get("q")}
	if query.Has("level") && !generated.RuntimeLogLevel(filter.Level).Valid() ||
		query.Has("module") && !generated.RuntimeLogModule(filter.Module).Valid() ||
		query.Has("job_id") && (!validTaskID(filter.JobID) || !utf8.ValidString(filter.JobID) || strings.ContainsRune(filter.JobID, 0)) ||
		!utf8.ValidString(filter.Query) || utf8.RuneCountInString(filter.Query) > 128 || strings.ContainsRune(filter.Query, 0) {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "日志筛选条件无效")
		return domain.Filter{}, false
	}
	if live {
		return filter, true
	}
	value := generated.RuntimeLogRange(query.Get("range"))
	if !query.Has("range") {
		value = generated.RuntimeLogRangeN24h
	}
	var duration time.Duration
	switch value {
	case generated.RuntimeLogRangeN1h:
		duration = time.Hour
	case generated.RuntimeLogRangeN24h:
		duration = 24 * time.Hour
	case generated.RuntimeLogRangeN7d:
		duration = 7 * 24 * time.Hour
	case generated.RuntimeLogRangeN30d:
		duration = 30 * 24 * time.Hour
	case generated.RuntimeLogRangeAll:
		return filter, true
	default:
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "日志时间范围无效")
		return domain.Filter{}, false
	}
	filter.Since = time.Now().UTC().Add(-duration)
	return filter, true
}

// runtimeLogCursor 保存稳定排序所需的时间戳和唯一行号。
type runtimeLogCursor struct {
	Time string `json:"t"`
	ID   int64  `json:"id"`
}

// encodeRuntimeLogCursor 将最后一条记录的稳定排序键编码为不透明游标。
func encodeRuntimeLogCursor(entry domain.Entry) string {
	value := runtimeLogCursor{Time: entry.OccurredAt.UTC().Format(time.RFC3339Nano), ID: entry.ID}
	// 固定字符串和整数结构不包含可能导致 Marshal 失败的字段。
	data, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(data)
}

// decodeRuntimeLogCursor 校验有界游标、UTC 纳秒时间和正整数行号。
func decodeRuntimeLogCursor(cursor string) (domain.Cursor, bool) {
	if len(cursor) == 0 || len(cursor) > 256 {
		return domain.Cursor{}, false
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil {
		return domain.Cursor{}, false
	}
	var value runtimeLogCursor
	if err := json.Unmarshal(data, &value); err != nil || value.ID < 1 {
		return domain.Cursor{}, false
	}
	occurred, err := time.Parse(time.RFC3339Nano, value.Time)
	if err != nil || occurred.IsZero() || occurred.UTC().Format(time.RFC3339Nano) != value.Time {
		return domain.Cursor{}, false
	}
	// 只接受本接口生成的格式，拒绝重复字段、额外字段与非规范编码。
	entry := domain.Entry{OccurredAt: occurred, ID: value.ID}
	if encodeRuntimeLogCursor(entry) != cursor {
		return domain.Cursor{}, false
	}
	return domain.Cursor{OccurredAt: occurred, ID: value.ID}, true
}

// runtimeLogResponse 保持查询、实时流及 JSONL 使用相同公开字段与不透明字符串 ID。
func runtimeLogResponse(entry domain.Entry) generated.RuntimeLogEntry {
	var details *map[string]any
	if len(entry.Details) > 0 {
		details = &entry.Details
	}
	return generated.RuntimeLogEntry{
		Id: strconv.FormatInt(entry.ID, 10), OccurredAt: entry.OccurredAt.UTC(), Level: generated.RuntimeLogLevel(entry.Level),
		Module: generated.RuntimeLogModule(entry.Module), Operation: entry.Operation, JobId: runtimeLogOptional(entry.JobID),
		RequestId: runtimeLogOptional(entry.RequestID), ErrorCode: runtimeLogOptional(entry.ErrorCode),
		Message: entry.Message, Details: details,
	}
}

func runtimeLogOptional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// runtimeLogClearResponse 对齐清理响应与 SSE 清理事件的公开字段。
func runtimeLogClearResponse(result logs.ClearResult) generated.ClearRuntimeLogsResponse {
	return generated.ClearRuntimeLogsResponse{DeletedCount: result.DeletedCount, Scope: generated.ClearRuntimeLogsScope(result.Scope)}
}
