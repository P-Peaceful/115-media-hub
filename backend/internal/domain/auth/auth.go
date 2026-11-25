// auth.go：定义管理员、设置、会话与审计的领域数据，隔离传输和 SQL 实现并标明凭据存储边界。

// Package auth 定义独立于传输和 SQL 的管理员认证领域数据。
package auth

import "time"

// Administrator 表示唯一的本地管理员，密码哈希不得跨越传输边界。
type Administrator struct {
	ID           string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

// Settings 保存首次初始化的业务设置。
type Settings struct {
	ServiceURL          string
	OutputDirectory     string
	SessionLifetimeDays int
}

// Session 只保存浏览器凭据和 CSRF 证明的不可逆摘要绑定。
type Session struct {
	TokenHash       string
	AdministratorID string
	CSRFHash        string
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

// Identity 组合已认证会话与所属管理员身份，传输层仍需筛选可公开字段。
type Identity struct {
	Session       Session
	Administrator Administrator
}

// Audit 保存不含凭据的安全操作持久化记录。
type Audit struct {
	ActorID       string
	Action        string
	TargetID      string
	Outcome       string
	CorrelationID string
	OccurredAt    time.Time
}
