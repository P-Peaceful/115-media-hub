// schedules.go：实现任务计划的 HTTP 管理和传输转换，校验可空媒体库、通知枚举及乐观更新版本。

package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
)

// listSchedules 组合分页、类型、媒体库和严格布尔启用筛选，返回计划列表及下一游标。
func (h *taskHandler) listSchedules(c *gin.Context) {
	target, ok := queryTaskFilter(c)
	if !ok {
		return
	}
	filter := domain.ScheduleFilter{Page: target.Page, Type: target.Type, LibraryID: target.LibraryID}
	if enabled, exists := c.GetQuery("enabled"); exists {
		if enabled != "true" && enabled != "false" {
			writeError(c, http.StatusBadRequest, generated.ValidationFailed, "计划状态无效")
			return
		}
		value := enabled == "true"
		filter.Enabled = &value
	}
	page, err := h.service.ListSchedules(c.Request.Context(), filter)
	if err != nil {
		h.failure(c, "list_schedules", err)
		return
	}
	items := make([]generated.Schedule, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, scheduleResponse(item))
	}
	c.JSON(http.StatusOK, generated.SchedulePage{Items: items, NextCursor: page.NextCursor})
}

// createSchedule 校验计划输入并创建持久计划，返回其版本与下次运行信息。
func (h *taskHandler) createSchedule(c *gin.Context) {
	input, ok := readScheduleInput(c)
	if !ok {
		return
	}
	item, err := h.service.CreateSchedule(c.Request.Context(), input, c.GetString("request_id"))
	if err != nil {
		h.failure(c, "create_schedule", err)
		return
	}
	c.JSON(http.StatusCreated, scheduleResponse(item))
}

// getSchedule 校验计划标识并读取保存的计划配置。
func (h *taskHandler) getSchedule(c *gin.Context) {
	id, ok := taskPathID(c)
	if !ok {
		return
	}
	item, err := h.service.GetSchedule(c.Request.Context(), id)
	if err != nil {
		h.failure(c, "get_schedule", err)
		return
	}
	c.JSON(http.StatusOK, scheduleResponse(item))
}

// updateSchedule 要求规范的正整数版本并更新计划，过期版本由服务返回冲突。
func (h *taskHandler) updateSchedule(c *gin.Context) {
	id, ok := taskPathID(c)
	if !ok {
		return
	}
	value := c.GetHeader("X-Schedule-Revision")
	revision, err := strconv.ParseInt(value, 10, 64)
	if err != nil || revision < 1 || strconv.FormatInt(revision, 10) != value {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "计划版本无效，请刷新后重试")
		return
	}
	input, ok := readScheduleInput(c)
	if !ok {
		return
	}
	item, err := h.service.UpdateSchedule(c.Request.Context(), id, input, revision, c.GetString("request_id"))
	if err != nil {
		h.failure(c, "update_schedule", err)
		return
	}
	c.JSON(http.StatusOK, scheduleResponse(item))
}

// enableSchedule 仅接收 enabled 字段更新计划启停状态，返回刷新后的计划。
func (h *taskHandler) enableSchedule(c *gin.Context) {
	id, ok := taskPathID(c)
	if !ok {
		return
	}
	var input generated.ScheduleEnabledInput
	if !decodeAuthBody(c, &input, []string{"enabled"}) {
		return
	}
	item, err := h.service.SetScheduleEnabled(c.Request.Context(), id, input.Enabled, c.GetString("request_id"))
	if err != nil {
		h.failure(c, "set_schedule_enabled", err)
		return
	}
	c.JSON(http.StatusOK, scheduleResponse(item))
}

// deleteSchedule 携带管理员身份删除指定计划，保留任务历史的处理由服务负责。
func (h *taskHandler) deleteSchedule(c *gin.Context) {
	id, ok := taskPathID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteSchedule(c.Request.Context(), id, providerIdentity(c).Administrator.ID, c.GetString("request_id")); err != nil {
		h.failure(c, "delete_schedule", err)
		return
	}
	c.Status(http.StatusNoContent)
}

// readScheduleInput 严格解码计划必填字段及可空媒体库，校验枚举后构造业务输入，时间规则由服务校验。
func readScheduleInput(c *gin.Context) (domain.ScheduleInput, bool) {
	var request generated.ScheduleInput
	if !decodeAuthBody(c, &request, []string{"name", "type", "enabled", "period", "timezone", "notify_on"}, "library_id") {
		return domain.ScheduleInput{}, false
	}
	if !request.Type.Valid() || !request.Period.Valid() || request.LibraryId != nil && !validTaskID(*request.LibraryId) {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "执行类型、媒体库或触发周期无效")
		return domain.ScheduleInput{}, false
	}
	input := domain.ScheduleInput{
		Name: request.Name, Type: domain.Type(request.Type), LibraryID: request.LibraryId, Enabled: request.Enabled,
		Period: domain.Period(request.Period), Timezone: request.Timezone, Time: request.Time,
		MonthDay: request.MonthDay, Cron: request.Cron, NotifyOn: make([]domain.Outcome, 0, len(request.NotifyOn)),
	}
	if request.Weekdays != nil {
		input.Weekdays = *request.Weekdays
	}
	for _, outcome := range request.NotifyOn {
		if !outcome.Valid() {
			writeError(c, http.StatusBadRequest, generated.ValidationFailed, "任务结果通知类型无效")
			return domain.ScheduleInput{}, false
		}
		input.NotifyOn = append(input.NotifyOn, domain.Outcome(outcome))
	}
	return input, true
}

// scheduleResponse 映射计划配置、通知和时间字段，按传输合同保留可空星期与下次执行时间。
func scheduleResponse(schedule domain.Schedule) generated.Schedule {
	notifyOn := make([]generated.TaskOutcome, 0, len(schedule.NotifyOn))
	for _, outcome := range schedule.NotifyOn {
		notifyOn = append(notifyOn, generated.TaskOutcome(outcome))
	}
	var weekdays *[]int
	if len(schedule.Weekdays) > 0 {
		weekdays = &schedule.Weekdays
	}
	return generated.Schedule{
		Id: schedule.ID, Name: schedule.Name, Type: generated.TaskType(schedule.Type), LibraryId: schedule.LibraryID,
		Enabled: schedule.Enabled, Period: generated.SchedulePeriod(schedule.Period), Timezone: schedule.Timezone,
		Time: schedule.Time, Weekdays: weekdays, MonthDay: schedule.MonthDay, Cron: schedule.Cron, NotifyOn: notifyOn,
		NextRunAt: schedule.NextRunAt, CreatedAt: schedule.CreatedAt.UTC(), UpdatedAt: schedule.UpdatedAt.UTC(), Revision: schedule.Revision,
	}
}
