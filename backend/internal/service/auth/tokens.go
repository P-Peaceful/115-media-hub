// tokens.go：实现会话令牌摘要、CSRF 绑定及进程内匿名 Cookie 的签名和有效期校验。

package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// randomToken 将 32 字节密码学随机数编码为固定 64 字符的十六进制令牌。
func randomToken() string {
	var value [32]byte
	_, _ = rand.Read(value[:]) // Go 1.26 保证填满缓冲区并返回 nil 错误。
	return hex.EncodeToString(value[:])
}

// tokenHash 计算令牌的 SHA-256 摘要，供持久化和绑定比较使用。
func tokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// csrfToken 使用固定用途前缀从 Cookie 派生 CSRF 证明。
func csrfToken(value string) string { return tokenHash("mediahub-csrf:" + value) }

// tokenEqual 对等长凭证进行常量时间比较；长度不同直接判为不匹配。
func tokenEqual(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// validSessionToken 检查会话令牌长度并拒绝分隔符和常见空白；不承担持久化认证。
func validSessionToken(token string) bool {
	return len(token) == 64 && !strings.ContainsAny(token, ". \t\r\n")
}

// AnonymousBinding 签发仅当前进程可验证、有效期 20 分钟的匿名 Cookie 及其 CSRF 证明。
func (s *Service) AnonymousBinding() (cookie, csrf string) {
	payload := randomToken() + "." + strconv.FormatInt(s.now().Unix(), 10)
	cookie = payload + "." + s.signAnonymous(payload)
	return cookie, csrfToken(cookie)
}

// signAnonymous 使用本进程密钥计算匿名 Cookie 载荷的 HMAC 签名。
func (s *Service) signAnonymous(value string) string {
	mac := hmac.New(sha256.New, s.anonymousKey)
	_, _ = mac.Write([]byte(value)) // hash.Hash.Write 不会返回错误。
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidateAnonymous 校验匿名 Cookie 的结构、签发时间、20 分钟有效期、签名及对应 CSRF 证明。
func (s *Service) ValidateAnonymous(cookie, csrf string) bool {
	if len(cookie) > 180 {
		return false
	}
	parts := strings.Split(cookie, ".")
	if len(parts) != 3 || len(parts[0]) != 64 {
		return false
	}
	issued, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}
	now := s.now()
	created := time.Unix(issued, 0)
	if created.After(now) || !now.Before(created.Add(20*time.Minute)) {
		return false
	}
	return tokenEqual(parts[2], s.signAnonymous(parts[0]+"."+parts[1])) && tokenEqual(csrf, csrfToken(cookie))
}

// AnonymousCSRF 复用仍有效的匿名 Cookie，使重复查询和多个标签页共享同一证明。
func (s *Service) AnonymousCSRF(cookie string) (string, bool) {
	proof := csrfToken(cookie)
	if !s.ValidateAnonymous(cookie, proof) {
		return "", false
	}
	return proof, true
}
