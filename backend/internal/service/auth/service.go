// service.go：编排管理员初始化、登录、会话校验和密码恢复，统一认证策略与操作结果日志。

// Package auth 管理管理员认证策略与原子安全用例。
package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
)

// Repository 封装认证持久化；安全变更必须与对应审计在同一事务提交。
type Repository interface {
	Administrator(context.Context) (domain.Administrator, error)
	Settings(context.Context) (domain.Settings, error)
	FindSession(context.Context, string, time.Time) (domain.Identity, error)
	Initialize(context.Context, domain.Administrator, domain.Settings, domain.Session, domain.Audit) error
	CreateSession(context.Context, domain.Session, string, string, domain.Audit) error
	RevokeSession(context.Context, string, domain.Audit) error
	ResetPassword(context.Context, string, domain.Audit) error
	RecordAudit(context.Context, domain.Audit) error
}

// Options 提供可替换的时钟和密码哈希成本；生产默认使用成本 12。
type Options struct {
	Now          func() time.Time
	PasswordCost int
}

// Service 持有认证策略、匿名签名密钥与尝试限额，不依赖 HTTP 或 SQL 类型。
type Service struct {
	repo         Repository
	logger       *slog.Logger
	now          func() time.Time
	cost         int
	anonymousKey []byte
	dummyHash    []byte
	limiter      attemptLimiter
}

// New 构造认证服务，准备密码比较占位哈希和仅本次进程有效的匿名 Cookie 签名密钥。
func New(repo Repository, logger *slog.Logger, options Options) (*Service, error) {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.PasswordCost == 0 {
		options.PasswordCost = 12
	}
	if options.PasswordCost < bcrypt.MinCost || options.PasswordCost > bcrypt.MaxCost {
		return nil, fmt.Errorf("invalid bcrypt cost")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate anonymous signing key: %w", err)
	}
	dummyHash, err := bcrypt.GenerateFromPassword([]byte(rand.Text()), options.PasswordCost)
	if err != nil {
		return nil, fmt.Errorf("prepare credential comparison: %w", err)
	}
	return &Service{repo: repo, logger: logger.With("module", "auth"), now: options.Now, cost: options.PasswordCost, anonymousKey: key, dummyHash: dummyHash}, nil
}

// SessionResult 承载会话身份及供传输层下发的原始凭证；原始 Token 和 CSRF 值不持久化或记录日志。
type SessionResult struct {
	Identity  domain.Identity
	Token     string
	CSRFToken string
}

// Initialized 仅查询管理员是否存在，将首次启动的缺失状态转换为 false。
func (s *Service) Initialized(ctx context.Context) (bool, error) {
	_, err := s.repo.Administrator(ctx)
	if errors.Is(err, ErrSetupRequired) {
		return false, nil
	}
	return err == nil, unavailable(err)
}

// Authenticate 按令牌摘要查询未过期会话；非法格式或失效凭证返回未认证错误。
func (s *Service) Authenticate(ctx context.Context, token string) (domain.Identity, error) {
	if !validSessionToken(token) {
		return domain.Identity{}, ErrUnauthenticated
	}
	identity, err := s.repo.FindSession(ctx, tokenHash(token), s.now().UTC())
	if errors.Is(err, ErrUnauthenticated) {
		return domain.Identity{}, err
	}
	return identity, unavailable(err)
}

// CSRFForSession 从会话 Cookie 重建绑定的 CSRF 证明，不存储原始证明。
func (s *Service) CSRFForSession(token string) string { return csrfToken(token) }

// ValidateSessionCSRF 以常量时间比较提交证明及其持久化摘要，验证与当前会话的绑定。
func (s *Service) ValidateSessionCSRF(identity domain.Identity, token, proof string) bool {
	return tokenEqual(proof, csrfToken(token)) && tokenEqual(tokenHash(proof), identity.Session.CSRFHash)
}

// Initialize 校验首次配置并限制密码哈希开销，原子创建管理员、设置、首个会话和审计。
func (s *Service) Initialize(ctx context.Context, username, password string, settings domain.Settings, peer, correlation string) (result SessionResult, resultErr error) {
	started := time.Now()
	defer func() { s.logOutcome(ctx, "initialize", correlation, started, resultErr) }()
	if err := validateCredentials(username, password); err != nil {
		return result, err
	}
	if err := validateSettings(settings); err != nil {
		return result, err
	}
	release, err := s.limiter.admit(peer, s.now())
	if err != nil {
		return result, err
	}
	defer release()
	initialized, err := s.Initialized(ctx)
	if err != nil {
		return result, err
	}
	if initialized {
		return result, ErrAlreadyInitialized
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return result, unavailable(err)
	}
	now := s.now().UTC()
	administrator := domain.Administrator{ID: rand.Text(), Username: username, PasswordHash: string(hash), CreatedAt: now}
	result = s.newSession(administrator, settings, now)
	err = s.repo.Initialize(ctx, administrator, settings, result.Identity.Session, s.audit(administrator.ID, "initialize", administrator.ID, "succeeded", correlation))
	if errors.Is(err, ErrAlreadyInitialized) {
		return SessionResult{}, err
	}
	if err != nil {
		return SessionResult{}, unavailable(err)
	}
	return result, nil
}

// Login 统一用户名或密码错误的外部结果；成功时轮换旧会话，并由仓储复核密码代次。
func (s *Service) Login(ctx context.Context, username, password, oldToken, peer, correlation string) (result SessionResult, resultErr error) {
	started := time.Now()
	defer func() { s.logOutcome(ctx, "login", correlation, started, resultErr) }()
	if err := validateCredentials(username, password); err != nil {
		return result, err
	}
	release, err := s.limiter.admit(peer, s.now())
	if err != nil {
		return result, err
	}
	defer release()
	administrator, err := s.repo.Administrator(ctx)
	if errors.Is(err, ErrSetupRequired) {
		return result, err
	}
	if err != nil {
		return result, unavailable(err)
	}
	comparison := []byte(administrator.PasswordHash)
	if administrator.Username != username {
		comparison = s.dummyHash
	}
	passwordErr := bcrypt.CompareHashAndPassword(comparison, []byte(password))
	if passwordErr != nil || administrator.Username != username {
		actor := ""
		if administrator.Username == username {
			actor = administrator.ID
		}
		if err := s.repo.RecordAudit(ctx, s.audit(actor, "login", administrator.ID, "failed", correlation)); err != nil {
			return result, unavailable(err)
		}
		return result, ErrInvalidCredentials
	}
	settings, err := s.repo.Settings(ctx)
	if err != nil {
		return result, unavailable(err)
	}
	result = s.newSession(administrator, settings, s.now().UTC())
	oldHash := ""
	if validSessionToken(oldToken) {
		oldHash = tokenHash(oldToken)
	}
	if err := s.repo.CreateSession(ctx, result.Identity.Session, oldHash, administrator.PasswordHash, s.audit(administrator.ID, "login", administrator.ID, "succeeded", correlation)); err != nil {
		return SessionResult{}, unavailable(err)
	}
	return result, nil
}

// Logout 原子撤销当前会话并保存必须成功的安全审计。
func (s *Service) Logout(ctx context.Context, identity domain.Identity, correlation string) (resultErr error) {
	started := time.Now()
	defer func() { s.logOutcome(ctx, "logout", correlation, started, resultErr) }()
	return unavailable(s.repo.RevokeSession(ctx, identity.Session.TokenHash, s.audit(identity.Administrator.ID, "logout", identity.Administrator.ID, "succeeded", correlation)))
}

// ResetPassword 仅修改已存在管理员的密码，原子撤销全部会话并保留业务设置。
func (s *Service) ResetPassword(ctx context.Context, password, correlation string) (resultErr error) {
	started := time.Now()
	defer func() { s.logOutcome(ctx, "reset_password", correlation, started, resultErr) }()
	if err := ValidatePassword(password); err != nil {
		return err
	}
	administrator, err := s.repo.Administrator(ctx)
	if errors.Is(err, ErrSetupRequired) {
		return err
	}
	if err != nil {
		return unavailable(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return unavailable(err)
	}
	return unavailable(s.repo.ResetPassword(ctx, string(hash), s.audit("local_maintenance", "reset_password", administrator.ID, "succeeded", correlation)))
}

// newSession 生成固定期限会话与 CSRF 证明，存储摘要并从返回身份中移除密码哈希。
func (s *Service) newSession(administrator domain.Administrator, settings domain.Settings, now time.Time) SessionResult {
	token := randomToken()
	csrf := csrfToken(token)
	session := domain.Session{TokenHash: tokenHash(token), AdministratorID: administrator.ID, CSRFHash: tokenHash(csrf), CreatedAt: now, ExpiresAt: now.Add(time.Duration(settings.SessionLifetimeDays) * 24 * time.Hour)}
	administrator.PasswordHash = ""
	return SessionResult{Identity: domain.Identity{Session: session, Administrator: administrator}, Token: token, CSRFToken: csrf}
}

// audit 组装使用注入时钟和调用关联标识的安全审计记录。
func (s *Service) audit(actor, action, target, outcome, correlation string) domain.Audit {
	return domain.Audit{ActorID: actor, Action: action, TargetID: target, Outcome: outcome, CorrelationID: correlation, OccurredAt: s.now().UTC()}
}

// unavailable 保留预期认证错误的身份，为其余失败附加服务不可用分类。
func unavailable(err error) error {
	if err == nil {
		return nil
	}
	for _, expected := range []error{ErrSetupRequired, ErrAlreadyInitialized, ErrInvalidCredentials, ErrUnauthenticated, ErrCSRF} {
		if errors.Is(err, expected) {
			return err
		}
	}
	return errors.Join(ErrUnavailable, err)
}
