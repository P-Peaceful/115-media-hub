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

const taskEventHeartbeat = 15 * time.Second
const taskEventWriteTimeout = 5 * time.Second

type taskEventPayload struct {
	ID           string                 `json:"id"`
	Notification *generated.TaskOutcome `json:"notification,omitempty"`
}

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
	// Gin's writer commits response headers, but its Flush method drops errors
	// from the underlying connection. Flush through the server writer instead.
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
		// Session validity can change while the stream is open, including on logout.
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

func writeTaskEvent(w http.ResponseWriter, controller *http.ResponseController, frame string) bool {
	// Bound each flush without applying the server's one-shot timeout to the stream.
	if err := controller.SetWriteDeadline(time.Now().Add(taskEventWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return false
	}
	if _, err := fmt.Fprint(w, frame); err != nil || controller.Flush() != nil {
		return false
	}
	err := controller.SetWriteDeadline(time.Time{})
	return err == nil || errors.Is(err, http.ErrNotSupported)
}
