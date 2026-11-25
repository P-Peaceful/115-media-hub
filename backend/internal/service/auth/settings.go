// settings.go：实现管理员凭据与会话期限设置，验证当前密码并委托仓储完成原子安全变更。

package auth

import (
	"context"
	"time"

	"golang.org/x/crypto/bcrypt"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
)

// settingsRepository 补充安全设置写入合同，变更需同时校验凭据状态并持久化审计。
type settingsRepository interface {
	UpdateCredentials(context.Context, string, string, string, string, domain.Audit) error
	UpdateSessionLifetime(context.Context, int, domain.Audit) error
}

// SecuritySettings 读取管理员信息与当前会话期限设置，将持久化失败归类为认证服务不可用。
func (s *Service) SecuritySettings(ctx context.Context) (domain.Administrator, domain.Settings, error) {
	administrator, err := s.repo.Administrator(ctx)
	if err != nil {
		return domain.Administrator{}, domain.Settings{}, unavailable(err)
	}
	settings, err := s.repo.Settings(ctx)
	return administrator, settings, unavailable(err)
}

// UpdateCredentials 验证当前密码并生成新哈希；仓储再次检查密码代次，只保留发起变更的会话。
func (s *Service) UpdateCredentials(ctx context.Context, identity domain.Identity, username, currentPassword, newPassword, correlation string) (resultErr error) {
	started := time.Now()
	defer func() { s.logOutcome(ctx, "update_credentials", correlation, started, resultErr) }()
	if !usernamePattern.MatchString(username) {
		return &ValidationError{Fields: map[string]string{"username": "用户名需为 3–32 位英文字母、数字、下划线、点或连字符"}}
	}
	if newPassword != "" {
		if err := ValidatePassword(newPassword); err != nil {
			return err
		}
	}
	administrator, err := s.repo.Administrator(ctx)
	if err != nil {
		return unavailable(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(administrator.PasswordHash), []byte(currentPassword)) != nil {
		return ErrInvalidCredentials
	}
	if newPassword == "" {
		newPassword = currentPassword
	}
	generated, err := bcrypt.GenerateFromPassword([]byte(newPassword), s.cost)
	if err != nil {
		return unavailable(err)
	}
	hash := string(generated)
	repo, ok := s.repo.(settingsRepository)
	if !ok {
		return ErrUnavailable
	}
	return unavailable(repo.UpdateCredentials(ctx, username, hash, administrator.PasswordHash, identity.Session.TokenHash, s.audit(identity.Administrator.ID, "update_credentials", identity.Administrator.ID, "succeeded", correlation)))
}

// UpdateSessionLifetime 保存 1、7 或 30 天的会话期限，仅影响此后创建的会话。
func (s *Service) UpdateSessionLifetime(ctx context.Context, identity domain.Identity, days int, correlation string) (resultErr error) {
	started := time.Now()
	defer func() { s.logOutcome(ctx, "update_session_lifetime", correlation, started, resultErr) }()
	if days != 1 && days != 7 && days != 30 {
		return &ValidationError{Fields: map[string]string{"session_lifetime_days": "登录有效期仅支持 1、7 或 30 天"}}
	}
	repo, ok := s.repo.(settingsRepository)
	if !ok {
		return ErrUnavailable
	}
	return unavailable(repo.UpdateSessionLifetime(ctx, days, s.audit(identity.Administrator.ID, "update_session_lifetime", identity.Administrator.ID, "succeeded", correlation)))
}
