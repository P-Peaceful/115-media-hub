// Package jobs defines the persisted tasks and schedules managed by the task center.
package jobs

import (
	"errors"
	"time"
)

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

func Types() []Type { return []Type{Scan, STRM, Sync, Check, Cleanup, Scrape, Upload} }

type Status string

const (
	Queued    Status = "queued"
	Running   Status = "running"
	Paused    Status = "paused"
	Completed Status = "completed"
	Failed    Status = "failed"
	Canceled  Status = "canceled"
)

func Statuses() []Status { return []Status{Queued, Running, Paused, Completed, Failed, Canceled} }

type Outcome string

const (
	Success        Outcome = "success"
	PartialFailure Outcome = "partial_failure"
	Failure        Outcome = "failure"
)

type Source string

const (
	Manual    Source = "manual"
	Scheduled Source = "schedule"
	Retried   Source = "retry"
)

type Action string

const (
	Pause       Action = "pause"
	Resume      Action = "resume"
	Cancel      Action = "cancel"
	Retry       Action = "retry"
	RetryFailed Action = "retry_failed"
)

type RetryScope string

const (
	RetryWhole    RetryScope = "whole"
	RetryFailures RetryScope = "failed"
)

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

type Capability struct {
	Type           Type
	Available      bool
	Reason         *string
	CanPause       bool
	CanRetryFailed bool
}

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

	// Execution metadata stays inside the job center.
	RequestID     string
	PendingAction Action
	NeedsResume   bool
	Attempt       int
	RetryScope    RetryScope
	NotifyOn      []Outcome
}

type ItemStatus string

const (
	ItemSucceeded ItemStatus = "succeeded"
	ItemFailed    ItemStatus = "failed"
	ItemSkipped   ItemStatus = "skipped"
)

type Item struct {
	ID     string
	Key    string
	Path   string
	Reason string
	Status ItemStatus
}

type Progress struct {
	ProgressPercent     *float64
	SpeedBytesPerSecond *int64
	CurrentFile         *string
	TotalItems          *int64
}

type JobLog struct {
	ID        string
	Level     string
	Message   string
	CreatedAt time.Time
}

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

type Schedule struct {
	ScheduleInput
	ID        string
	NextRunAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	Revision  int64
}

type Page struct {
	Cursor string
	Limit  int
}

type JobFilter struct {
	Page
	Type      Type
	Status    Status
	LibraryID string
}

type JobPage struct {
	Items      []Job
	NextCursor *string
	Total      int64
	ByType     map[Type]int64
	ByStatus   map[Status]int64
}

type ItemPage struct {
	Items      []Item
	NextCursor *string
}

type LogPage struct {
	Items      []JobLog
	NextCursor *string
}

type ScheduleFilter struct {
	Page
	Type      Type
	LibraryID string
	Enabled   *bool
}

type SchedulePage struct {
	Items      []Schedule
	NextCursor *string
}

type Event struct {
	JobID        string
	ScheduleID   string
	Notification *Outcome
}
