// schedule.go：校验日历与 Cron 定时规则，管理计划配置，并将到期触发解析为合格媒体库任务。

package jobs

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	"github.com/robfig/cron/v3"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
)

// defaultTimezone 是未指定计划时区时使用的产品默认值，不使用主机本地时区。
const defaultTimezone = "Asia/Shanghai"

// scheduleRule 要求明确可加载的 IANA 时区，将合法配置解析为五字段 Cron 规则。
func scheduleRule(input domain.ScheduleInput) (cron.Schedule, error) {
	if input.Timezone == "" || input.Timezone == "Local" || len(input.Timezone) > 128 {
		return nil, domain.ErrInvalid
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return nil, domain.ErrInvalid
	}
	expression, err := scheduleExpression(input)
	if err != nil {
		return nil, err
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	rule, err := parser.Parse("CRON_TZ=" + input.Timezone + " " + expression)
	if err != nil {
		return nil, domain.ErrInvalid
	}
	return rule, nil
}

// scheduleExpression 校验周期互斥字段并生成五字段表达式，拒绝重复星期和超界日期。
func scheduleExpression(input domain.ScheduleInput) (string, error) {
	if input.Period == domain.Custom {
		if input.Cron == nil || len(*input.Cron) > 160 || len(strings.Fields(*input.Cron)) != 5 ||
			strings.ContainsAny(*input.Cron, "=\r\n") || input.Time != nil || len(input.Weekdays) > 0 || input.MonthDay != nil {
			return "", domain.ErrInvalid
		}
		return *input.Cron, nil
	}
	if input.Time == nil || len(*input.Time) != 5 || input.Cron != nil {
		return "", domain.ErrInvalid
	}
	clock, err := time.Parse("15:04", *input.Time)
	if err != nil {
		return "", domain.ErrInvalid
	}
	prefix := fmt.Sprintf("%d %d", clock.Minute(), clock.Hour())
	switch input.Period {
	case domain.Daily:
		if len(input.Weekdays) > 0 || input.MonthDay != nil {
			return "", domain.ErrInvalid
		}
		return prefix + " * * *", nil
	case domain.Weekly:
		if len(input.Weekdays) == 0 || len(input.Weekdays) > 7 || input.MonthDay != nil {
			return "", domain.ErrInvalid
		}
		days := make([]string, 0, len(input.Weekdays))
		for _, day := range input.Weekdays {
			value := strconv.Itoa(day)
			if day < 0 || day > 6 || slices.Contains(days, value) {
				return "", domain.ErrInvalid
			}
			days = append(days, value)
		}
		return prefix + " * * " + strings.Join(days, ","), nil
	case domain.Monthly:
		if len(input.Weekdays) > 0 || input.MonthDay == nil || *input.MonthDay < 1 || *input.MonthDay > 31 {
			return "", domain.ErrInvalid
		}
		return prefix + " " + strconv.Itoa(*input.MonthDay) + " * *", nil
	default:
		return "", domain.ErrInvalid
	}
}

// ListSchedules 校验分页和筛选条件后读取定时计划。
func (s *Service) ListSchedules(ctx context.Context, filter domain.ScheduleFilter) (domain.SchedulePage, error) {
	page, err := normalizePage(filter.Page)
	if err != nil || (filter.Type != "" && !slices.Contains(domain.Types(), filter.Type)) ||
		(filter.LibraryID != "" && !validID(filter.LibraryID)) {
		return domain.SchedulePage{}, domain.ErrInvalid
	}
	filter.Page = page
	return s.repo.QuerySchedules(ctx, filter)
}

// GetSchedule 校验计划 ID 并读取当前保存的配置。
func (s *Service) GetSchedule(ctx context.Context, id string) (domain.Schedule, error) {
	if !validID(id) {
		return domain.Schedule{}, domain.ErrInvalid
	}
	return s.repo.ReadSchedule(ctx, id)
}

// configureSchedule 规范计划名称并验证规则、模块及目标资格，启用时计算 UTC 下次触发时间。
func (s *Service) configureSchedule(ctx context.Context, schedule *domain.Schedule, requestID string) error {
	schedule.Name = strings.TrimSpace(schedule.Name)
	if schedule.Name == "" || !utf8.ValidString(schedule.Name) || utf8.RuneCountInString(schedule.Name) > 100 ||
		strings.ContainsRune(schedule.Name, 0) || !slices.Contains(domain.Types(), schedule.Type) {
		return domain.ErrInvalid
	}
	unique := map[domain.Outcome]bool{}
	for _, outcome := range schedule.NotifyOn {
		if unique[outcome] || (outcome != domain.Success && outcome != domain.PartialFailure && outcome != domain.Failure) {
			return domain.ErrInvalid
		}
		unique[outcome] = true
	}
	if schedule.Timezone == "" {
		schedule.Timezone = defaultTimezone
	}
	rule, err := scheduleRule(schedule.ScheduleInput)
	if err != nil {
		return err
	}
	next := rule.Next(schedule.UpdatedAt).UTC()
	if next.IsZero() {
		return domain.ErrInvalid
	}
	module, available := s.module(schedule.Type)
	if !available || module.ManualOnly {
		return domain.ErrUnavailable
	}
	if schedule.LibraryID != nil {
		if _, err := s.target(ctx, module, *schedule.LibraryID, requestID); err != nil {
			return err
		}
	}
	schedule.NextRunAt = nil
	if schedule.Enabled {
		schedule.NextRunAt = &next
	}
	return nil
}

// CreateSchedule 生成计划身份和初始修订号，校验配置后保存并发布结果。
func (s *Service) CreateSchedule(ctx context.Context, input domain.ScheduleInput, requestID string) (domain.Schedule, error) {
	now := s.now().UTC()
	schedule := domain.Schedule{ScheduleInput: input, ID: rand.Text(), CreatedAt: now, UpdatedAt: now, Revision: 1}
	if err := s.configureSchedule(ctx, &schedule, requestID); err != nil {
		return domain.Schedule{}, err
	}
	err := s.repo.InsertSchedule(ctx, schedule)
	s.scheduleChange(ctx, schedule.ID, "create_schedule", requestID, now, err)
	return schedule, err
}

// UpdateSchedule 读取当前计划并核对提供的修订号，重新计算规则后条件保存。
func (s *Service) UpdateSchedule(ctx context.Context, id string, input domain.ScheduleInput, revision int64, requestID string) (domain.Schedule, error) {
	schedule, err := s.GetSchedule(ctx, id)
	if err != nil {
		return domain.Schedule{}, err
	}
	if revision != 0 && revision != schedule.Revision {
		return domain.Schedule{}, domain.ErrConflict
	}
	schedule.ScheduleInput = input
	schedule.UpdatedAt = s.now().UTC()
	if err := s.configureSchedule(ctx, &schedule, requestID); err != nil {
		return domain.Schedule{}, err
	}
	return s.saveSchedule(ctx, schedule, "update_schedule", requestID)
}

// SetScheduleEnabled 切换计划启用状态；禁用清空触发时间，重新启用时复核资格和规则。
func (s *Service) SetScheduleEnabled(ctx context.Context, id string, enabled bool, requestID string) (domain.Schedule, error) {
	schedule, err := s.GetSchedule(ctx, id)
	if err != nil {
		return domain.Schedule{}, err
	}
	if schedule.Enabled == enabled {
		return schedule, nil
	}
	schedule.Enabled = enabled
	schedule.UpdatedAt = s.now().UTC()
	schedule.NextRunAt = nil
	if enabled {
		if err := s.configureSchedule(ctx, &schedule, requestID); err != nil {
			return domain.Schedule{}, err
		}
	}
	return s.saveSchedule(ctx, schedule, "set_schedule_enabled", requestID)
}

// saveSchedule 条件保存计划并记录结果，成功后反映递增的修订号。
func (s *Service) saveSchedule(ctx context.Context, schedule domain.Schedule, operation, requestID string) (domain.Schedule, error) {
	err := s.repo.SaveSchedule(ctx, schedule)
	s.scheduleChange(ctx, schedule.ID, operation, requestID, schedule.UpdatedAt, err)
	if err == nil {
		schedule.Revision++
	}
	return schedule, err
}

// DeleteSchedule 删除计划并记录操作结果，不影响已提交的任务历史。
func (s *Service) DeleteSchedule(ctx context.Context, id, actorID, requestID string) error {
	if !validID(id) {
		return domain.ErrInvalid
	}
	start := time.Now()
	err := s.repo.RemoveSchedule(ctx, id, actorID, requestID)
	s.scheduleChange(ctx, id, "delete_schedule", requestID, start, err)
	return err
}

// scheduleChange 记录计划操作结果，成功时向订阅者发布需回读状态的提示。
func (s *Service) scheduleChange(ctx context.Context, id, operation, requestID string, start time.Time, err error) {
	attrs := []any{"schedule_id", id, "operation", operation, "duration_ms", max(int64(0), time.Since(start).Milliseconds())}
	if requestID != "" {
		attrs = append(attrs, "request_id", requestID)
	}
	if err == nil {
		s.logger.InfoContext(ctx, "schedule operation completed", append(attrs, "outcome", "succeeded")...)
		s.publish(domain.Event{ScheduleID: id})
		return
	}
	level, code := slog.LevelError, "storage_unavailable"
	if errors.Is(err, domain.ErrConflict) {
		level, code = slog.LevelWarn, "conflict"
	} else if errors.Is(err, domain.ErrNotFound) {
		level, code = slog.LevelWarn, "not_found"
	}
	s.logger.Log(ctx, level, "schedule operation failed", append(attrs, "outcome", "failed", "error_code", code)...)
}

// tickSchedules 分批推进所有到期计划，遵守取消并忽略已被其他触发者处理的冲突。
func (s *Service) tickSchedules(ctx context.Context, now time.Time, stopped bool) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		schedules, err := s.repo.DueSchedules(ctx, now)
		if err != nil || len(schedules) == 0 {
			return err
		}
		var batchErr error
		for _, plan := range schedules {
			err := s.advance(ctx, plan, now, stopped)
			if err != nil && !errors.Is(err, domain.ErrConflict) {
				batchErr = errors.Join(batchErr, err)
			}
		}
		if batchErr != nil {
			return batchErr
		}
	}
}

// advance 原子推进计划并入队当前触发的任务，提交成功后发布提示及唤醒工作池。
func (s *Service) advance(ctx context.Context, plan domain.Schedule, now time.Time, stopped bool) error {
	start := time.Now()
	next, reason, runs, err := s.planOccurrence(ctx, plan, now, stopped)
	if err != nil {
		return err
	}
	if err := s.repo.AdvanceSchedule(ctx, plan, next, reason, runs); err != nil {
		return err
	}
	s.publish(domain.Event{ScheduleID: plan.ID})
	attrs := []any{"operation", "trigger_schedule", "schedule_id", plan.ID, "task_type", plan.Type,
		"scheduled_for", plan.NextRunAt, "jobs_created", len(runs), "duration_ms", time.Since(start).Milliseconds()}
	if reason != "" {
		s.logger.InfoContext(ctx, "schedule occurrence skipped", append(attrs, "outcome", "skipped", "reason", reason)...)
	} else {
		s.logger.InfoContext(ctx, "schedule occurrence submitted", append(attrs, "outcome", "succeeded")...)
	}
	for _, run := range runs {
		s.log(ctx, "submit_scheduled", run, start, nil)
		s.publish(domain.Event{JobID: run.ID})
	}
	if len(runs) > 0 {
		s.wakeWorker()
	}
	return nil
}

// planOccurrence 计算下次触发及本次目标，停机错过的触发只推进时间，不补执行历史任务。
func (s *Service) planOccurrence(ctx context.Context, plan domain.Schedule, now time.Time, stopped bool) (*time.Time, string, []domain.Job, error) {
	rule, err := scheduleRule(plan.ScheduleInput)
	if err != nil {
		return nil, "invalid_schedule", nil, nil
	}
	next := rule.Next(now).UTC()
	if next.IsZero() {
		return nil, "no_future_occurrence", nil, nil
	}
	if stopped {
		return &next, "missed_while_stopped", nil, nil
	}
	module, available := s.module(plan.Type)
	if !available || module.ManualOnly {
		return &next, "task_type_unavailable", nil, nil
	}
	targets, err := s.scheduleTargets(ctx, plan, module)
	if err != nil {
		return nil, "", nil, err
	}
	if len(targets) == 0 {
		return &next, "no_eligible_libraries", nil, nil
	}
	runs := make([]domain.Job, 0, len(targets))
	for _, target := range targets {
		run := s.newJob(plan.Type, target, domain.Scheduled, "")
		run.ScheduleID = &plan.ID
		run.ScheduledFor = plan.NextRunAt
		run.NotifyOn = plan.NotifyOn
		runs = append(runs, run)
	}
	return &next, "", runs, nil
}

// scheduleTargets 读取指定或分页遍历全部媒体库，仅返回启用且符合模块资格的目标。
func (s *Service) scheduleTargets(ctx context.Context, plan domain.Schedule, module Module) ([]library.Library, error) {
	if plan.LibraryID != nil {
		lib, err := s.target(ctx, module, *plan.LibraryID, "")
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrLibraryUnavailable) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return []library.Library{lib}, nil
	}
	var targets []library.Library
	const pageSize = 200
	for cursor := ""; ; {
		found, err := s.library.List(ctx, cursor, pageSize, "")
		if err != nil {
			return nil, err
		}
		for _, lib := range found {
			if eligible(module, lib) {
				targets = append(targets, lib)
			}
		}
		if len(found) < pageSize {
			return targets, nil
		}
		cursor = found[len(found)-1].ID
	}
}
