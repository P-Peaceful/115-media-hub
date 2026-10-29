package drive115

import (
	"errors"
	"regexp"
	"strings"
)

// UpstreamError carries only individually extracted, sanitized fields. The raw
// transport error is intentionally never attached as a cause because it may include
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

func sanitizeUpstreamText(value string) string {
	value = secretPattern.ReplaceAllString(value, "$1[已隐藏]")
	value = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' {
			return -1
		}
		return r
	}, value)
	if runes := []rune(value); len(runes) > 240 {
		value = string(runes[:240])
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
