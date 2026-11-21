package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/jobs"
)

type taskHandler struct {
	service *jobs.Service
	auth    *auth.Service
	logger  *slog.Logger
}

func (h *taskHandler) register(r *gin.Engine) {
	r.GET("/api/v1/task-types", h.capabilities)
	r.GET("/api/v1/tasks", h.listTasks)
	r.POST("/api/v1/tasks", h.submitTask)
	r.GET("/api/v1/tasks/:id", h.getTask)
	r.GET("/api/v1/tasks/:id/failures", h.listFailures)
	r.GET("/api/v1/tasks/:id/logs", h.listLogs)
	r.POST("/api/v1/tasks/:id/actions", h.applyAction)
	r.GET("/api/v1/schedules", h.listSchedules)
	r.POST("/api/v1/schedules", h.createSchedule)
	r.GET("/api/v1/schedules/:id", h.getSchedule)
	r.PUT("/api/v1/schedules/:id", h.updateSchedule)
	r.PATCH("/api/v1/schedules/:id/enabled", h.enableSchedule)
	r.DELETE("/api/v1/schedules/:id", h.deleteSchedule)
	r.GET("/api/v1/events", h.events)
}

func (h *taskHandler) failure(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "请检查任务或计划字段")
	case errors.Is(err, domain.ErrNotFound):
		writeError(c, http.StatusNotFound, generated.NotFound, "任务、计划或媒体库不存在")
	case errors.Is(err, domain.ErrUnavailable):
		writeError(c, http.StatusConflict, generated.TaskTypeUnavailable, "该任务类型暂不可用")
	case errors.Is(err, domain.ErrLibraryUnavailable):
		writeError(c, http.StatusConflict, generated.Conflict, "目标媒体库当前无法执行此任务")
	case errors.Is(err, domain.ErrConflict) && operation == "apply_task_action":
		writeError(c, http.StatusConflict, generated.TaskStateConflict, "当前任务状态不支持该操作，请刷新后重试")
	case errors.Is(err, domain.ErrConflict):
		writeError(c, http.StatusConflict, generated.Conflict, "计划状态已变化，请刷新后重试")
	case c.Request.Context().Err() != nil:
		c.Abort()
	default:
		h.logger.ErrorContext(c.Request.Context(), "task center request failed", "module", "jobs", "operation", operation, "request_id", c.GetString("request_id"), "outcome", "failed", "error_code", "task_center_unavailable")
		writeError(c, http.StatusServiceUnavailable, generated.ServiceUnavailable, "任务中心暂时不可用")
	}
}

func (h *taskHandler) capabilities(c *gin.Context) {
	capabilities := h.service.Capabilities()
	items := make([]generated.TaskTypeCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		items = append(items, generated.TaskTypeCapability{
			Type: generated.TaskType(capability.Type), Available: capability.Available, Reason: capability.Reason,
			CanPause: capability.CanPause, CanRetryFailed: capability.CanRetryFailed,
		})
	}
	c.JSON(http.StatusOK, generated.TaskTypeCapabilities{Items: items})
}

func (h *taskHandler) listTasks(c *gin.Context) {
	filter, ok := queryTaskFilter(c)
	if !ok {
		return
	}
	if status, exists := c.GetQuery("status"); exists {
		if !generated.TaskStatus(status).Valid() {
			writeError(c, http.StatusBadRequest, generated.ValidationFailed, "任务状态无效")
			return
		}
		filter.Status = domain.Status(status)
	}
	page, err := h.service.ListJobs(c.Request.Context(), filter)
	if err != nil {
		h.failure(c, "list_tasks", err)
		return
	}
	response := generated.TaskPage{
		Items: make([]generated.Task, 0, len(page.Items)), NextCursor: page.NextCursor, Total: page.Total,
		ByType: make(map[string]int64, len(page.ByType)), ByStatus: make(map[string]int64, len(page.ByStatus)),
	}
	for _, item := range page.Items {
		response.Items = append(response.Items, taskResponse(item))
	}
	for kind, count := range page.ByType {
		response.ByType[string(kind)] = count
	}
	for status, count := range page.ByStatus {
		response.ByStatus[string(status)] = count
	}
	c.JSON(http.StatusOK, response)
}

func (h *taskHandler) submitTask(c *gin.Context) {
	var request generated.SubmitTaskInput
	if !decodeAuthBody(c, &request, []string{"type", "library_id"}) {
		return
	}
	if !request.Type.Valid() || !validTaskID(request.LibraryId) {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "任务类型或媒体库无效")
		return
	}
	item, err := h.service.Submit(c.Request.Context(), domain.Type(request.Type), request.LibraryId, c.GetString("request_id"))
	if err != nil {
		h.failure(c, "submit_task", err)
		return
	}
	c.JSON(http.StatusAccepted, taskResponse(item))
}

func (h *taskHandler) getTask(c *gin.Context) {
	id, ok := taskPathID(c)
	if !ok {
		return
	}
	item, err := h.service.GetJob(c.Request.Context(), id)
	if err != nil {
		h.failure(c, "get_task", err)
		return
	}
	c.JSON(http.StatusOK, taskResponse(item))
}

func (h *taskHandler) listFailures(c *gin.Context) {
	id, ok := taskPathID(c)
	if !ok {
		return
	}
	pagination, ok := queryTaskPage(c)
	if !ok {
		return
	}
	page, err := h.service.ListFailedItems(c.Request.Context(), id, pagination)
	if err != nil {
		h.failure(c, "list_task_failures", err)
		return
	}
	items := make([]generated.TaskFailure, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, generated.TaskFailure{Id: item.ID, Path: item.Path, Reason: item.Reason})
	}
	c.JSON(http.StatusOK, generated.TaskFailurePage{Items: items, NextCursor: page.NextCursor})
}

func (h *taskHandler) listLogs(c *gin.Context) {
	id, ok := taskPathID(c)
	if !ok {
		return
	}
	pagination, ok := queryTaskPage(c)
	if !ok {
		return
	}
	page, err := h.service.ListLogs(c.Request.Context(), id, pagination)
	if err != nil {
		h.failure(c, "list_task_logs", err)
		return
	}
	items := make([]generated.TaskLog, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, generated.TaskLog{Id: item.ID, Level: item.Level, Message: item.Message, CreatedAt: item.CreatedAt.UTC()})
	}
	c.JSON(http.StatusOK, generated.TaskLogPage{Items: items, NextCursor: page.NextCursor})
}

func (h *taskHandler) applyAction(c *gin.Context) {
	id, ok := taskPathID(c)
	if !ok {
		return
	}
	var request generated.TaskActionInput
	if !decodeAuthBody(c, &request, []string{"action"}) {
		return
	}
	if !request.Action.Valid() {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "任务操作无效")
		return
	}
	ctx, requestID := c.Request.Context(), c.GetString("request_id")
	var item domain.Job
	var err error
	switch domain.Action(request.Action) {
	case domain.Retry:
		item, err = h.service.Retry(ctx, id, domain.RetryWhole, requestID)
	case domain.RetryFailed:
		item, err = h.service.Retry(ctx, id, domain.RetryFailures, requestID)
	default:
		item, err = h.service.Control(ctx, id, domain.Action(request.Action), requestID)
	}
	if err != nil {
		h.failure(c, "apply_task_action", err)
		return
	}
	c.JSON(http.StatusAccepted, taskResponse(item))
}

func queryTaskFilter(c *gin.Context) (domain.JobFilter, bool) {
	page, ok := queryTaskPage(c)
	filter := domain.JobFilter{Page: page}
	if !ok {
		return filter, false
	}
	if value, exists := c.GetQuery("type"); exists {
		if !generated.TaskType(value).Valid() {
			writeError(c, http.StatusBadRequest, generated.ValidationFailed, "任务类型无效")
			return filter, false
		}
		filter.Type = domain.Type(value)
	}
	if id, exists := c.GetQuery("library_id"); exists {
		if !validTaskID(id) {
			writeError(c, http.StatusBadRequest, generated.ValidationFailed, "媒体库标识无效")
			return filter, false
		}
		filter.LibraryID = id
	}
	return filter, true
}

func queryTaskPage(c *gin.Context) (domain.Page, bool) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	cursor := c.Query("cursor")
	if err != nil || limit < 1 || limit > 200 || len(cursor) > 256 || strings.ContainsAny(cursor, " \t\r\n") {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "分页参数无效")
		return domain.Page{}, false
	}
	return domain.Page{Limit: limit, Cursor: cursor}, true
}

func taskPathID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if !validTaskID(id) {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "任务或计划标识无效")
		return "", false
	}
	return id, true
}

func validTaskID(id string) bool {
	return len(id) > 0 && len(id) <= 128 && strings.TrimSpace(id) == id
}

func taskResponse(item domain.Job) generated.Task {
	actions := make([]generated.TaskAction, 0, len(item.Actions))
	for _, action := range item.Actions {
		actions = append(actions, generated.TaskAction(action))
	}
	return generated.Task{
		Id: item.ID, Type: generated.TaskType(item.Type), LibraryId: item.LibraryID, LibraryName: item.LibraryName,
		Source: generated.TaskSource(item.Source), ScheduleId: item.ScheduleID, ParentId: item.ParentID,
		Status: generated.TaskStatus(item.Status), Outcome: (*generated.TaskOutcome)(item.Outcome),
		CreatedAt: item.CreatedAt.UTC(), StartedAt: item.StartedAt, FinishedAt: item.FinishedAt,
		DurationMs: item.DurationMS, CurrentFile: item.CurrentFile, ProgressPercent: item.ProgressPercent,
		SpeedBytesPerSecond: item.SpeedBytesPerSecond, TotalItems: item.TotalItems, SucceededItems: item.SucceededItems,
		FailedItems: item.FailedItems, SkippedItems: item.SkippedItems, Actions: actions,
	}
}
