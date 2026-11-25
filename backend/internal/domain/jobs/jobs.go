// jobs.go：定义任务中心的任务、计划、动作、进度和分页数据，统一持久化状态与模块能力合同。

// Package jobs 定义任务中心管理的持久化任务与计划领域类型。
package jobs

import (
	"errors"
	"time"
)

// Type 表示业务任务类别；是否可执行取决于当前模块注册。
type Type string

const (
	Scan    Type = "scan"
	STRM    Type = "strm"
	Sync    Type = "sync"
	Check   Type = "check"
	Cleanup Type = "cleanup"
	Scrape  Type = "scrape"
	Upload  Type = "upload"
)

// Types 返回任务中心支持识别的全部任务类型，顺序用于列表展示。
func Types() []Type { return []Type{Scan, STRM, Sync, Check, Cleanup, Scrape, Upload} }

// Status 表示任务持久化生命周期状态。
type Status string

const (
	Queued    Status = "queued"
	Running   Status = "running"
	Paused    Status = "paused"
	Completed Status = "completed"
	Failed    Status = "failed"
	Canceled  Status = "canceled"
)

// Statuses 返回任务状态枚举，供过滤校验和统计使用。
func Statuses() []Status { return []Status{Queued, Running, Paused, Completed, Failed, Canceled} }

// Outcome 表示已结束任务的成功、部分失败或失败结果。
type Outcome string

const (
	Success        Outcome = "success"
	PartialFailure Outcome = "partial_failure"
	Failure        Outcome = "failure"
)

// Source 表示任务来自手动、计划还是重试。
type Source string

const (
	Manual    Source = "manual"
	Scheduled Source = "schedule"
	Retried   Source = "retry"
)

// Action 表示客户端可请求的任务控制动作。
type Action string

const (
	Pause       Action = "pause"
	Resume      Action = "resume"
	Cancel      Action = "cancel"
	Retry       Action = "retry"
	RetryFailed Action = "retry_failed"
)

// RetryScope 区分整轮重试与仅重试原失败条目。
type RetryScope string

const (
	RetryWhole    RetryScope = "whole"
	RetryFailures RetryScope = "failed"
)

// Period 表示计划规则的日、周、月或自定义周期。
type Period string

const (
	Daily   Period = "daily"
	Weekly  Period = "weekly"
	Monthly Period = "monthly"
	Custom  Period = "custom"
)

var (
	ErrInvalid            = errors.New("invalid task center request")
	ErrNotFound           = errors.New("task center resource not found")
	ErrConflict           = errors.New("task center resource state conflict")
	ErrUnavailable        = errors.New("task type is unavailable")
	ErrLibraryUnavailable = errors.New("media library is unavailable for this task")
)

// Capability 描述当前注册模块的可用性和暂停、失败项重试、计划能力。
type Capability struct {
	Type           Type
	Available      bool
	Reason         *string
	CanPause       bool
	CanRetryFailed bool
	CanSchedule    bool
}

// Job 保存任务身份、执行状态、统计和尝试元数据；业务副作用由模块负责。
type Job struct {
	ID                  string
	Type                Type
	LibraryID           string
	LibraryName         string
	Source              Source
	ScheduleID          *string
	ScheduledFor        *time.Time
	ParentID            *string
	Status              Status
	Outcome             *Outcome
	CreatedAt           time.Time
	StartedAt           *time.Time
	FinishedAt          *time.Time
	DurationMS          *int64
	ProgressPercent     *float64
	SpeedBytesPerSecond *int64
	CurrentFile         *string
	TotalItems          *int64
	SucceededItems      int64
	FailedItems         int64
	SkippedItems        int64
	Actions             []Action

	// 执行元数据仅在任务中心内部流转。
	RequestID     string
	PendingAction Action
	NeedsResume   bool
	Attempt       int
	RetryScope    RetryScope
	NotifyOn      []Outcome
}

// ItemStatus 表示单条任务处理结果。
type ItemStatus string

const (
	ItemSucceeded ItemStatus = "succeeded"
	ItemFailed    ItemStatus = "failed"
	ItemSkipped   ItemStatus = "skipped"
)

// Item 保存稳定条目键、路径与安全失败原因，供统计和失败项重试使用。
type Item struct {
	ID     string
	Key    string
	Path   string
	Reason string
	Status ItemStatus
}

// Progress 描述模块报告的最新进度，空指针表示未提供该字段。
type Progress struct {
	ProgressPercent     *float64
	SpeedBytesPerSecond *int64
	CurrentFile         *string
	TotalItems          *int64
}

// JobLog 保存任务历史中的可展示操作记录。
type JobLog struct {
	ID        string
	Level     string
	Message   string
	CreatedAt time.Time
}

// ScheduleInput 描述计划规则和通知选项；空媒体库表示所有符合条件的库。
type ScheduleInput struct {
	Name      string
	Type      Type
	LibraryID *string
	Enabled   bool
	Period    Period
	Timezone  string
	Time      *string
	Weekdays  []int
	MonthDay  *int
	Cron      *string
	NotifyOn  []Outcome
}

// Schedule 为计划配置附加身份、下次触发时间及乐观并发版本。
type Schedule struct {
	ScheduleInput
	ID        string
	NextRunAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	Revision  int64
}

// Page 描述游标分页的起点和条数。
type Page struct {
	Cursor string
	Limit  int
}

// JobFilter 组合任务列表的分页、类型、状态及媒体库条件。
type JobFilter struct {
	Page
	Type      Type
	Status    Status
	LibraryID string
}

// JobPage 包含任务分页、总数及按类型和状态聚合的计数。
type JobPage struct {
	Items      []Job
	NextCursor *string
	Total      int64
	ByType     map[Type]int64
	ByStatus   map[Status]int64
}

// ItemPage 描述任务条目分页及可空的后续游标。
type ItemPage struct {
	Items      []Item
	NextCursor *string
}

// LogPage 描述任务日志分页及可空的后续游标。
type LogPage struct {
	Items      []JobLog
	NextCursor *string
}

// ScheduleFilter 组合计划列表的分页、类型、媒体库及可选启用条件。
type ScheduleFilter struct {
	Page
	Type      Type
	LibraryID string
	Enabled   *bool
}

// SchedulePage 描述计划分页及可空的后续游标。
type SchedulePage struct {
	Items      []Schedule
	NextCursor *string
}

// Event 提示任务或计划已变化，并按配置附带终态通知结果。
type Event struct {
	JobID        string
	ScheduleID   string
	Notification *Outcome
}
