// task_events.go：将任务中心变更转发为 SSE 通知，维护连接内序号、会话复核、心跳及有界写入，退出时释放订阅。

package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
)

// taskEventHeartbeat 控制空闲连接保活及会话有效性复核的间隔。
const taskEventHeartbeat = 15 * time.Second

// taskEventWriteTimeout 限制单帧发送时间，避免慢客户端持续占用连接。
const taskEventWriteTimeout = 5 * time.Second

// taskEventPayload 只传资源失效提示和可选结果通知，客户端须重读持久状态。
type taskEventPayload struct {
	ID           string                 `json:"id"`
	Notification *generated.TaskOutcome `json:"notification,omitempty"`
}

// events 为已认证请求订阅变更并发送 SSE；每次通知或心跳前复核会话，取消、撤销或写入失败即退出。
// 事件序号只在本连接内递增，不根据 Last-Event-ID 重放持久历史。
func (h *taskHandler) events(c *gin.Context) {
	events, unsubscribe := h.service.Subscribe()
	if events == nil {
		h.logger.WarnContext(c.Request.Context(), "task event subscription unavailable", "module", "jobs", "operation", "subscribe", "request_id", c.GetString("request_id"), "outcome", "failed", "error_code", "event_subscription_unavailable")
		writeError(c, http.StatusServiceUnavailable, generated.ServiceUnavailable, "实时任务事件暂时不可用")
		return
	}
	defer unsubscribe()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-store, no-transform")
	c.Header("X-Accel-Buffering", "no")
	// Gin 的 Flush 不返回底层错误，因此解包后由服务器写入器刷新并检查连接状态。
	writer := http.ResponseWriter(c.Writer)
	if wrapper, ok := writer.(interface{ Unwrap() http.ResponseWriter }); ok {
		writer = wrapper.Unwrap()
	}
	controller := http.NewResponseController(writer)
	if !writeTaskEvent(c.Writer, controller, ": connected\n\n") {
		return
	}

	ctx := c.Request.Context()
	token, _ := c.Cookie(sessionCookie)
	ticker := time.NewTicker(taskEventHeartbeat)
	defer ticker.Stop()
	var sequence uint64
	for {
		var frame string
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			frame = ": keep-alive\n\n"
		case event, ok := <-events:
			if !ok {
				return
			}
			name, id := "task.changed", event.JobID
			if id == "" {
				name, id = "schedule.changed", event.ScheduleID
			}
			if id == "" {
				continue
			}
			payload, err := json.Marshal(taskEventPayload{ID: id, Notification: (*generated.TaskOutcome)(event.Notification)})
			if err != nil {
				return
			}
			sequence++
			frame = fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", sequence, name, payload)
		}
		// 流打开期间会话仍可能因注销等操作失效，发送前必须重新校验。
		if _, err := h.auth.Authenticate(ctx, token); err != nil {
			if !errors.Is(err, auth.ErrUnauthenticated) && ctx.Err() == nil {
				h.logger.WarnContext(ctx, "task event authorization failed", "module", "jobs", "operation", "authorize_subscription", "request_id", c.GetString("request_id"), "error_code", "authentication_unavailable")
			}
			return
		}
		if !writeTaskEvent(c.Writer, controller, frame) {
			return
		}
	}
}

// writeTaskEvent 在单帧写入期限内写出并刷新 SSE，完成后清除写入期限；不支持期限控制的写入器仍可使用。
func writeTaskEvent(w http.ResponseWriter, controller *http.ResponseController, frame string) bool {
	// 每帧单独设置期限，避免将服务器的一次性写超时套用于整个长连接。
	if err := controller.SetWriteDeadline(time.Now().Add(taskEventWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return false
	}
	if _, err := fmt.Fprint(w, frame); err != nil || controller.Flush() != nil {
		return false
	}
	err := controller.SetWriteDeadline(time.Time{})
	return err == nil || errors.Is(err, http.ErrNotSupported)
}
