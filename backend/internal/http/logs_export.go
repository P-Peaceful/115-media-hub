// logs_export.go：以有界 SQLite 批次生成运行日志附件，并在响应开始前确定截断标记。

package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/logs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
)

const (
	runtimeLogExportLimit = 10_000
	runtimeLogExportBatch = 500
)

// export 固定导出的最大行号，然后逐批读取和写出；响应完成与失败都记录一次终态。
func (h *runtimeLogHandler) export(c *gin.Context) {
	query, ok := runtimeLogQuery(c)
	if !ok {
		return
	}
	format := generated.RuntimeLogExportFormat(query.Get("format"))
	if !format.Valid() {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "日志导出格式无效")
		return
	}
	filter, ok := runtimeLogFilter(c, query, false)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	started := time.Now()
	count, truncated := 0, false
	outcome, errorCode := "failed", "runtime_log_export_failed"
	defer func() {
		level := slog.LevelWarn
		if outcome == "succeeded" || ctx.Err() != nil {
			level = slog.LevelInfo
			if outcome != "succeeded" {
				outcome, errorCode = "canceled", "request_canceled"
			}
		}
		h.logger.Log(ctx, level, "runtime log export finished",
			"module", "logging", "operation", "export", "request_id", c.GetString("request_id"),
			"outcome", outcome, "error_code", errorCode, "duration_ms", time.Since(started).Milliseconds(),
			"exported_count", count, "format", string(format), "truncated", truncated)
	}()

	maxID, truncated, err := h.service.ExportBoundary(ctx, filter, runtimeLogExportLimit)
	if err != nil {
		h.unavailable(c)
		return
	}
	filter.MaxID = maxID
	contentType := "text/plain; charset=utf-8"
	if format == generated.RuntimeLogExportFormatJsonl {
		contentType = "application/x-ndjson; charset=utf-8"
	}
	controller := streamingResponseController(c)
	encoder := json.NewEncoder(c.Writer)
	for maxID > 0 && count < runtimeLogExportLimit {
		filter.Limit = min(runtimeLogExportBatch, runtimeLogExportLimit-count)
		batch, err := h.service.List(ctx, filter)
		if err != nil {
			if c.Writer.Written() {
				// 断开已开始的附件响应，使客户端能识别不完整的下载。
				panic(http.ErrAbortHandler)
			}
			h.unavailable(c)
			return
		}
		if len(batch) == 0 {
			break
		}
		writeRuntimeLogExportHeaders(c, started, contentType, format, truncated)
		for _, entry := range batch {
			if err := writeRuntimeLogExportEntry(c, encoder, format, entry); err != nil {
				panic(http.ErrAbortHandler)
			}
			count++
		}
		if err := controller.Flush(); err != nil {
			panic(http.ErrAbortHandler)
		}
		if len(batch) < filter.Limit {
			break
		}
		last := batch[len(batch)-1]
		filter.Before = domain.Cursor{OccurredAt: last.OccurredAt, ID: last.ID}
	}
	writeRuntimeLogExportHeaders(c, started, contentType, format, truncated)
	c.Status(http.StatusOK)
	outcome, errorCode = "succeeded", ""
}

// writeRuntimeLogExportHeaders 在首字节发出前设置安全附件文件名和截断状态，也适用于空结果。
func writeRuntimeLogExportHeaders(c *gin.Context, started time.Time, contentType string, format generated.RuntimeLogExportFormat, truncated bool) {
	if c.Writer.Written() {
		return
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"mediahub-logs-%s.%s\"", started.UTC().Format("20060102-150405"), format))
	c.Header("X-Export-Truncated", strconv.FormatBool(truncated))
}

// writeRuntimeLogExportEntry 复用公开 JSON 对象，纯文本格式则保持每条记录一行。
func writeRuntimeLogExportEntry(c *gin.Context, encoder *json.Encoder, format generated.RuntimeLogExportFormat, entry domain.Entry) error {
	if format == generated.RuntimeLogExportFormatJsonl {
		return encoder.Encode(runtimeLogResponse(entry))
	}
	message := strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\u0085', '\u2028', '\u2029':
			return ' '
		default:
			return r
		}
	}, entry.Message)
	_, err := fmt.Fprintf(c.Writer, "%s [%s] [%s] %s\n",
		entry.OccurredAt.UTC().Format(time.RFC3339Nano), strings.ToUpper(entry.Level), entry.Module, message)
	return err
}
