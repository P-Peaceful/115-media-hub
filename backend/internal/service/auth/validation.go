// validation.go：集中校验认证输入、服务地址和服务器输出路径，返回可向管理员展示的字段错误。

package auth

import (
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)

// 密码仅允许单字节 ASCII，字符数与字节数一致。
const (
	// MinPasswordLength 是初始化、登录与恢复统一使用的密码最小长度。
	MinPasswordLength = 8
	// MaxPasswordLength 是密码最大长度，不因入口不同而改变。
	MaxPasswordLength         = 32
	passwordValidationMessage = "密码需为 8–32 位英文字母、数字或 ASCII 符号，不能包含空格"
)

// ValidatePassword 要求密码为 8–32 位可打印且不含空格的 ASCII 字符，不裁剪用户输入。
func ValidatePassword(password string) error {
	valid := len(password) >= MinPasswordLength && len(password) <= MaxPasswordLength
	for i := 0; valid && i < len(password); i++ {
		valid = password[i] >= 0x21 && password[i] <= 0x7e
	}
	if !valid {
		return &ValidationError{Fields: map[string]string{"password": passwordValidationMessage}}
	}
	return nil
}

// validateCredentials 汇总用户名格式与密码策略错误，不回显原始凭据。
func validateCredentials(username, password string) error {
	fields := map[string]string{}
	if !usernamePattern.MatchString(username) {
		fields["username"] = "用户名需为 3–32 位英文字母、数字、下划线、点或连字符"
	}
	if err := ValidatePassword(password); err != nil {
		fields["password"] = passwordValidationMessage
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

// validateSettings 校验 HTTP(S) 服务源地址、规范绝对输出目录和支持的会话期限；不检查目录写权限。
func validateSettings(settings domain.Settings) error {
	fields := map[string]string{}
	parsed, err := url.Parse(settings.ServiceURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.Contains(settings.ServiceURL, "#") || (parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" || invalidPort(parsed) {
		fields["service_url"] = "请输入完整的 HTTP(S) 服务地址，不含账号、路径前缀、查询或片段"
	}
	if len(settings.ServiceURL) > 2048 {
		fields["service_url"] = "服务地址过长"
	}
	output := settings.OutputDirectory
	if len(output) > 4096 || !utf8.ValidString(output) || !path.IsAbs(output) || path.Clean(output) != output || strings.ContainsRune(output, '\\') || strings.ContainsFunc(output, unicode.IsControl) || strings.TrimSpace(output) != output {
		fields["output_directory"] = "请输入规范的服务器绝对目录路径"
	}
	if settings.SessionLifetimeDays != 1 && settings.SessionLifetimeDays != 7 && settings.SessionLifetimeDays != 30 {
		fields["session_lifetime_days"] = "登录有效期仅支持 1、7 或 30 天"
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

// invalidPort 拒绝空端口及 1–65535 范围外的显式端口。
func invalidPort(address *url.URL) bool {
	if strings.HasSuffix(address.Host, ":") {
		return true
	}
	if port := address.Port(); port != "" {
		number, err := strconv.Atoi(port)
		return err != nil || number < 1 || number > 65535
	}
	return false
}
