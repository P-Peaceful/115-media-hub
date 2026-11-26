// logs.go：定义运行日志记录、分页边界及历史与实时共用的筛选语义。

// Package logs 定义运行日志领域值，不依赖数据库或 HTTP。
package logs

import (
	"strings"
	"time"
)

// Entry 表示经过采集层脱敏的运行日志；ID 由持久化层分配。
type Entry struct {
	ID         int64
	OccurredAt time.Time
	Level      string
	Module     string
	Operation  string
	JobID      string
	RequestID  string
	ErrorCode  string
	Message    string
	Details    map[string]any
}

// Cursor 用发生时间与持久化 ID 唯一标记倒序分页的边界。
type Cursor struct {
	OccurredAt time.Time
	ID         int64
}

// Filter 描述历史查询和实时订阅的筛选；MaxID 可固定导出的写入上界。
type Filter struct {
	Since  time.Time
	Level  string
	Module string
	JobID  string
	Query  string
	Before Cursor
	Limit  int
	MaxID  int64
}

// Matches 判断单条记录是否满足筛选；关键词按 SQLite 相同的 ASCII 折叠规则比较。
func (f Filter) Matches(entry Entry) bool {
	if !f.Since.IsZero() && entry.OccurredAt.Before(f.Since) ||
		f.Level != "" && entry.Level != f.Level ||
		f.Module != "" && entry.Module != f.Module ||
		f.JobID != "" && entry.JobID != f.JobID ||
		f.MaxID > 0 && entry.ID > f.MaxID {
		return false
	}
	if f.Before.ID > 0 && (entry.OccurredAt.After(f.Before.OccurredAt) ||
		entry.OccurredAt.Equal(f.Before.OccurredAt) && entry.ID >= f.Before.ID) {
		return false
	}
	if f.Query == "" {
		return true
	}
	needle := foldASCII(f.Query)
	for _, field := range []string{entry.Message, entry.Operation, entry.ErrorCode, entry.JobID, entry.RequestID} {
		if strings.Contains(foldASCII(field), needle) {
			return true
		}
	}
	return false
}

// foldASCII 保留非 ASCII 字符，匹配 SQLite 内置 lower 的大小写行为。
func foldASCII(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, value)
}
