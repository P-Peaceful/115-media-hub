package drive115

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/SheltonZhu/115driver/pkg/driver"
)

// UpstreamError carries only individually extracted, sanitized fields. The raw
// SDK error is intentionally never attached as a cause because it may include
// the complete 115 response body.
type UpstreamError struct {
	Code    string
	Message string
	Cause   error
}

func (e *UpstreamError) Error() string { return e.Message }
func (e *UpstreamError) Unwrap() error { return e.Cause }

func AsUpstreamError(err error) (*UpstreamError, bool) {
	var upstream *UpstreamError
	if !errors.As(err, &upstream) {
		return nil, false
	}
	return upstream, true
}

var secretPattern = regexp.MustCompile(`(?i)((?:UID|CID|SEID|KID|Cookie|Set-Cookie|Authorization|token|access[_-]?token|refresh[_-]?token|sign|time|challenge|qrcode|qr[_-]?code)(?:\s*[=:]\s*|%3[dD]))[^&\s,;"']+`)

func parseUpstreamError(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	text := err.Error()
	start := strings.IndexByte(text, '{')
	if start >= 0 {
		var body map[string]any
		if json.Unmarshal([]byte(text[start:]), &body) == nil {
			code := firstText(body, "code", "errno", "error_code")
			message := firstText(body, "message", "msg", "error_description", "error")
			if message != "" {
				return sanitizeUpstreamText(code), sanitizeUpstreamText(message)
			}
		}
	}
	// Known SDK errors are fixed library strings, not raw response bodies.
	switch {
	case errors.Is(err, driver.ErrNotLogin), errors.Is(err, driver.ErrCredentialInvalid), errors.Is(err, driver.ErrBadCookie):
		return "", "115 登录凭据无效或已过期"
	case errors.Is(err, driver.ErrQrcodeExpired):
		return "", "115 扫码已过期，请重新获取二维码"
	case errors.Is(err, driver.ErrRepeatLogin):
		return "", "115 登录状态发生变化，请重新扫码"
	}
	return "", "115 上游请求失败"
}

func firstText(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			switch v := value.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					return v
				}
			case float64:
				return strconv.FormatInt(int64(v), 10)
			}
		}
	}
	return ""
}

func sanitizeUpstreamText(value string) string {
	value = secretPattern.ReplaceAllString(value, "$1[已隐藏]")
	value = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' {
			return -1
		}
		return r
	}, value)
	if len(value) > 240 {
		value = value[:240]
	}
	return strings.TrimSpace(value)
}

var (
	ErrInvalidChallenge    = errors.New("invalid QR challenge")
	ErrInvalidDevice       = errors.New("invalid QR device")
	ErrUnknownStatus       = errors.New("unknown upstream QR status")
	ErrCredentialInvalid   = errors.New("115 credential is invalid")
	ErrRootUnreadable      = errors.New("115 root directory is not readable")
	ErrUnreachable         = errors.New("115 service is unreachable")
	ErrLimited             = errors.New("115 request was limited")
	ErrInvalidBrowseFilter = errors.New("invalid 115 browse filter")
	ErrDirectoryNotFound   = errors.New("115 directory not found")
)

func loginApp(device Device) (driver.LoginApp, bool) {
	switch device {
	case DeviceWeb:
		return driver.LoginAppWeb, true
	case DeviceAlipayMini:
		return driver.LoginAppAlipayMini, true
	case DeviceWechatMini:
		return driver.LoginAppWechatMini, true
	default:
		return "", false
	}
}
func formatID(id int64) string { return strconv.FormatInt(id, 10) }

func safeNetworkError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, driver.ErrUnexpected) {
		code, message := parseUpstreamError(err)
		return &UpstreamError{Code: code, Message: message, Cause: ErrLimited}
	}
	code, message := parseUpstreamError(err)
	return &UpstreamError{Code: code, Message: message, Cause: ErrUnreachable}
}

func safeCredentialError(err error) error {
	code, message := parseUpstreamError(err)
	if message == "115 上游请求失败" {
		message = "115 登录凭据无效或已过期"
	}
	return &UpstreamError{Code: code, Message: message, Cause: ErrCredentialInvalid}
}

func safeRootError(err error) error {
	code, message := parseUpstreamError(err)
	if message == "115 上游请求失败" {
		message = "115 网盘根目录不可读取"
	}
	return &UpstreamError{Code: code, Message: message, Cause: ErrRootUnreadable}
}

func isTransportError(err error) bool {
	var netErr net.Error
	var urlErr *url.Error
	return errors.As(err, &netErr) || errors.As(err, &urlErr)
}
