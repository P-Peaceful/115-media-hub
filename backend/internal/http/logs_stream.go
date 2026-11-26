// logs_stream.go：将已持久化的运行日志变更发送至管理员 SSE 连接，并处理有界订阅与会话撤销。

package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/logs"
)

// stream 在发送心跳或日志前复核会话，依次设置连接内事件序号，并在退出后释放订阅。
func (h *runtimeLogHandler) stream(c *gin.Context) {
	query, ok := runtimeLogQuery(c)
	if !ok {
		return
	}
	filter, ok := runtimeLogFilter(c, query, true)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	events, unsubscribe, err := h.service.Subscribe(filter)
	if err != nil {
		h.logger.WarnContext(ctx, "runtime log subscription unavailable", "module", "logging", "operation", "stream_deliver", "request_id", c.GetString("request_id"), "outcome", "failed", "error_code", "event_subscription_unavailable")
		h.unavailable(c)
		return
	}
	defer unsubscribe()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-store, no-transform")
	c.Header("X-Accel-Buffering", "no")
	controller := streamingResponseController(c)
	if !writeEventFrame(c.Writer, controller, ": connected\n\n") {
		return
	}

	token, _ := c.Cookie(sessionCookie)
	ticker := time.NewTicker(eventHeartbeat)
	defer ticker.Stop()
	var sequence uint64
	for {
		frame := ": keep-alive\n\n"
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case event, open := <-events:
			if !open {
				return
			}
			sequence++
			frame, err = runtimeLogFrame(sequence, event)
			if err != nil {
				h.logger.ErrorContext(ctx, "runtime log event encoding failed", "module", "logging", "operation", "stream_deliver", "request_id", c.GetString("request_id"), "error_code", "event_encoding_failed")
				return
			}
		}
		if _, err := h.auth.Authenticate(ctx, token); err != nil {
			if !errors.Is(err, auth.ErrUnauthenticated) && ctx.Err() == nil {
				h.logger.WarnContext(ctx, "runtime log stream authorization failed", "module", "logging", "operation", "stream_deliver", "request_id", c.GetString("request_id"), "error_code", "authentication_unavailable")
			}
			return
		}
		if !writeEventFrame(c.Writer, controller, frame) {
			return
		}
	}
}

// runtimeLogFrame 以具名 SSE 帧包装与历史列表一致的日志对象或清理结果。
func runtimeLogFrame(sequence uint64, event logs.Event) (string, error) {
	var name string
	var payload any
	switch {
	case event.Entry != nil:
		name, payload = "log.created", runtimeLogResponse(*event.Entry)
	case event.Cleared != nil:
		name, payload = "log.cleared", runtimeLogClearResponse(*event.Cleared)
	default:
		return "", errors.New("empty runtime log event")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", sequence, name, data), nil
}
