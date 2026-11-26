// types.go：定义 115 协议传输的受控操作、请求预算和脱敏错误；不包含账号存储或业务投影。

package provider115

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Operation 声明协议操作，重试和期限由能力决定，不能由 HTTP 方法猜测。
type Operation string

const (
	QR           Operation = "qr"
	Login        Operation = "login"
	Metadata     Operation = "metadata"
	Files        Operation = "files"
	Playback     Operation = "playback"
	DownloadLink Operation = "download_link"
	FastFiles    Operation = "fast_files"
	Directories  Operation = "directories"
)

// Priority 区分交互与后台分页；后台在前台连续四次后获得一次机会。
type Priority uint8

const (
	Foreground Priority = iota
	Background
)

// Kind 是可供服务映射的安全上游错误类别。
type Kind string

const (
	Canceled             Kind = "canceled"
	Deadline             Kind = "deadline"
	InvalidAuthorization Kind = "invalid_authorization"
	NotFound             Kind = "not_found"
	Throttled            Kind = "throttled"
	Temporary            Kind = "temporary"
	Protocol             Kind = "protocol"
)

// Error 仅暴露受控诊断，不把 URL、响应体或底层网络错误放进文本。
type Error struct {
	Operation  Operation
	Kind       Kind
	StatusCode int
	RetryAt    time.Time
	cause      error
}

// Error 返回固定字段构成的脱敏描述。
func (e *Error) Error() string {
	return fmt.Sprintf("115 %s: %s (status %d)", e.Operation, e.Kind, e.StatusCode)
}

// Unwrap 保留 context 和调用方凭据校验错误的身份。
func (e *Error) Unwrap() error { return e.cause }

// ErrBudgetExhausted 表示本次抽样已无发送配额，不代表上游故障。
var ErrBudgetExhausted = errors.New("115 request budget exhausted")

// ErrClosed 表示适配器已关闭，所有等待者应停止。
var ErrClosed = errors.New("115 client closed")

// Config 注入传输、模块日志和账号间隔；TestEndpoints 仅由可信测试构造配置使用。
// 生产不配置测试端点，所有操作只允许固定 HTTPS 主机与路径。
type Config struct {
	Transport     http.RoundTripper
	Interval      time.Duration
	TestEndpoints []string
	Logger        *slog.Logger
}

// Request 保存一次可重建的协议调用；Scope 是账号的不透明稳定键，不能使用 Cookie。
// Check 在每次发送前及采用响应前验证凭据快照；Body 在重试时从原始字节重建。
// Inspect 在账号资格尚未释放时检查完整成功响应，可登记业务限流；回调应有界且不执行外部 I/O。
type Request struct {
	Scope     string
	Priority  Priority
	Operation Operation
	Method    string
	URL       string
	Header    http.Header
	Body      []byte
	MaxBytes  int64
	Check     func(context.Context) error
	Inspect   func(context.Context, []byte) error
	Budget    *Budget
}

// Budget 为一轮预览共享实际发送次数；等待、无效输入及凭据失效不消耗次数。
type Budget struct {
	mu        sync.Mutex
	remaining int
}

// NewBudget 构造非负发送预算；负值视为零。
func NewBudget(limit int) *Budget {
	if limit < 0 {
		limit = 0
	}
	return &Budget{remaining: limit}
}

// Remaining 返回尚可发送的次数，可用于预览停止原因判定。
func (b *Budget) Remaining() int { b.mu.Lock(); defer b.mu.Unlock(); return b.remaining }

// Take 只在实际交给传输之前扣减，所有并发调用共享同一计数；协议测试替身也使用该入口。
func (b *Budget) Take() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.remaining == 0 {
		return false
	}
	b.remaining--
	return true
}

type budgetKey struct{}

// WithBudget 把抽样预算传递到嵌套服务调用；显式 Request.Budget 优先。
func WithBudget(ctx context.Context, budget *Budget) context.Context {
	return context.WithValue(ctx, budgetKey{}, budget)
}

// contextError 分类取消与期限，并保留 errors.Is 身份。
func contextError(op Operation, err error) error {
	kind := Canceled
	if errors.Is(err, context.DeadlineExceeded) {
		kind = Deadline
	}
	return &Error{Operation: op, Kind: kind, cause: err}
}

// checkError 保留凭据校验错误身份，并避免把仓储故障或账号变更误判为上游授权失效。
func checkError(op Operation, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return contextError(op, err)
	}
	return &Error{Operation: op, Kind: Protocol, cause: err}
}

// BudgetFromContext 提取传入的发送预算，供服务包装器和协议测试替身共享同一实际发送计数。
func BudgetFromContext(ctx context.Context) *Budget {
	budget, _ := ctx.Value(budgetKey{}).(*Budget)
	return budget
}
