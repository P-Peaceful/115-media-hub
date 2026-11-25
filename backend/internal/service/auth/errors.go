// errors.go：定义认证错误分类和安全的字段校验、限流错误，供传输层稳定映射响应。

package auth

import (
	"errors"
	"time"
)

var (
	// ErrSetupRequired 表示管理员尚未初始化。
	ErrSetupRequired = errors.New("administrator setup required")
	// ErrAlreadyInitialized 表示初始化已完成，包含并发初始化的落败请求。
	ErrAlreadyInitialized = errors.New("administrator already initialized")
	// ErrInvalidCredentials 统一表示用户名、密码或已验证的凭据代次无效。
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUnauthenticated 表示会话缺失、过期或已撤销。
	ErrUnauthenticated = errors.New("authentication required")
	// ErrCSRF 表示 CSRF 绑定证明无效。
	ErrCSRF = errors.New("invalid CSRF proof")
	// ErrAuditUnavailable 表示必须保存的安全审计失败，相关变更不能提交。
	ErrAuditUnavailable = errors.New("security audit unavailable")
	// ErrUnavailable 标记非预期认证基础设施失败，保留具体错误链供内部分类。
	ErrUnavailable = errors.New("authentication unavailable")
)

// ValidationError 包含可安全返回的字段校验信息，不携带原始凭据。
type ValidationError struct{ Fields map[string]string }

// Error 返回稳定的认证错误摘要，不包含用户提交的凭据。
func (e *ValidationError) Error() string { return "invalid authentication input" }

// ThrottledError 指明再次进行昂贵认证尝试前应等待的最短时间。
type ThrottledError struct{ RetryAfter time.Duration }

// Error 返回稳定的认证错误摘要，不包含用户提交的凭据。
func (e *ThrottledError) Error() string { return "authentication attempts throttled" }
