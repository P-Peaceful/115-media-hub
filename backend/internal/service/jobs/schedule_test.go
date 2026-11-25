// schedule_test.go：验证日历周期与 IANA 时区的下次触发计算，以及非法 Cron 和重复周期字段的拒绝。

package jobs

import (
	"errors"
	"testing"
	"time"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
)

// TestScheduleRecurrenceUsesCalendarAndTimezone 验证星期、月底跳月和夏令时变化按指定时区计算下次触发。
func TestScheduleRecurrenceUsesCalendarAndTimezone(t *testing.T) {
	clock := "02:00"
	monthDay := 31
	cron := "30 6 * * MON-FRI"
	for _, tt := range []struct {
		name  string
		input domain.ScheduleInput
		after time.Time
		want  time.Time
	}{
		{
			name:  "weekly Sunday in China",
			input: domain.ScheduleInput{Period: domain.Weekly, Timezone: "Asia/Shanghai", Time: &clock, Weekdays: []int{0}},
			after: time.Date(2026, 11, 20, 16, 0, 0, 0, time.UTC),
			want:  time.Date(2026, 11, 21, 18, 0, 0, 0, time.UTC),
		},
		{
			name:  "monthly 31 skips February",
			input: domain.ScheduleInput{Period: domain.Monthly, Timezone: "Asia/Shanghai", Time: &clock, MonthDay: &monthDay},
			after: time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC),
			want:  time.Date(2027, 3, 30, 18, 0, 0, 0, time.UTC),
		},
		{
			name:  "custom cron uses daylight-saving offset",
			input: domain.ScheduleInput{Period: domain.Custom, Timezone: "America/New_York", Cron: &cron},
			after: time.Date(2027, 3, 12, 12, 0, 0, 0, time.UTC),
			want:  time.Date(2027, 3, 15, 10, 30, 0, 0, time.UTC),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := scheduleRule(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got := rule.Next(tt.after); !got.Equal(tt.want) {
				t.Fatalf("next occurrence = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestScheduleRecurrenceRejectsInvalidFields 验证重复或超界星期、六字段 Cron 和无效时区不能形成计划。
func TestScheduleRecurrenceRejectsInvalidFields(t *testing.T) {
	clock := "02:00"
	sixFields := "0 0 3 * * *"
	for _, input := range []domain.ScheduleInput{
		{Period: domain.Weekly, Timezone: "UTC", Time: &clock, Weekdays: []int{0, 0}},
		{Period: domain.Weekly, Timezone: "UTC", Time: &clock, Weekdays: []int{7}},
		{Period: domain.Custom, Timezone: "UTC", Cron: &sixFields},
		{Period: domain.Daily, Timezone: "Local", Time: &clock},
		{Period: domain.Daily, Timezone: "Unknown/Region", Time: &clock},
	} {
		if _, err := scheduleRule(input); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("accepted invalid recurrence %+v: %v", input, err)
		}
	}
}
