package drive115

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Driver implements the Cookie protocol using a shared connection pool.
type Driver struct{ http *http.Client }

func New(timeout time.Duration) *Driver {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Driver{http: &http.Client{Timeout: timeout, Transport: http.DefaultTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

const userAgent = "Mozilla/5.0 115Browser/36.0.0"
const filesAPI = "https://webapi.115.com/files"
const maxResponse = 8 << 20

func (d *Driver) request(ctx context.Context, method, endpoint string, params url.Values, cr Credential) ([]byte, error) {
	var body io.Reader
	if method == http.MethodGet {
		if len(params) > 0 {
			if strings.Contains(endpoint, "?") {
				endpoint += "&"
			} else {
				endpoint += "?"
			}
			endpoint += params.Encode()
		}
	} else {
		body = strings.NewReader(params.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, protocolError()
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://115.com/")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, c := range []struct{ name, value string }{{"UID", cr.UID}, {"CID", cr.CID}, {"SEID", cr.SEID}, {"KID", cr.KID}} {
		if c.value != "" {
			req.AddCookie(&http.Cookie{Name: c.name, Value: c.value})
		}
	}
	resp, err := d.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &UpstreamError{Message: "115 网络请求失败", Cause: ErrUnreachable}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, protocolError()
	}
	if len(data) > maxResponse {
		return nil, protocolError()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		cause := ErrUnreachable
		if resp.StatusCode == 401 {
			cause = ErrCredentialInvalid
		}
		if resp.StatusCode == 429 || resp.StatusCode == 403 {
			cause = ErrLimited
		}
		return nil, redactError(remoteError(data, cause), params, cr)
	}
	return data, nil
}
func (d *Driver) getJSON(ctx context.Context, endpoint string, params url.Values, cr Credential, result any) error {
	data, err := d.request(ctx, http.MethodGet, endpoint, params, cr)
	if err != nil {
		return err
	}
	return redactError(decodeResponse(data, result), params, cr)
}
func decodeResponse(data []byte, result any) error {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(data, &envelope) != nil || envelope == nil {
		return protocolError()
	}
	// Endpoints disagree on bool versus numeric state, but none should accept a
	// missing state as success. Token/status/nav endpoints all return state.
	state, ok := envelope["state"]
	if !ok {
		return protocolError()
	}
	var success wireBool
	if json.Unmarshal(state, &success) != nil {
		return protocolError()
	}
	if !success {
		return remoteError(data, ErrLimited)
	}
	if json.Unmarshal(data, result) != nil {
		return protocolError()
	}
	return nil
}
func protocolError() error {
	return &UpstreamError{Message: "115 返回的数据格式异常", Cause: ErrUnreachable}
}
func remoteError(data []byte, cause error) error {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	code := ""
	for _, key := range []string{"errno", "errNo", "code", "error_code"} {
		value := errorText(fields, key)
		if value != "" && value != "0" {
			code = value
			break
		}
	}
	switch code {
	case "99", "990001", "40101032", "40101035", "40101037":
		cause = ErrCredentialInvalid
	case "20018", "70005", "231011":
		cause = ErrDirectoryNotFound
	}
	message := errorText(fields, "message", "msg", "error_description", "error")
	if message == "" {
		message = "115 上游请求失败"
	}
	return &UpstreamError{Code: sanitizeUpstreamText(code), Message: sanitizeUpstreamText(message), Cause: cause}
}
func errorText(fields map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw := fields[key]
		var text string
		if json.Unmarshal(raw, &text) == nil && strings.TrimSpace(text) != "" {
			return text
		}
		var n wireInteger
		if json.Unmarshal(raw, &n) == nil && n != "" && n != "0" {
			return string(n)
		}
	}
	return ""
}
func isNetworkFailure(err error) bool {
	return errors.Is(err, ErrUnreachable) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// redactError removes exact request secrets as well as labeled values. Only
// safe local causes survive; URL-bearing transport errors are never wrapped.
func redactError(err error, params url.Values, cr Credential) error {
	var upstream *UpstreamError
	if !errors.As(err, &upstream) {
		return err
	}
	message, code := upstream.Message, upstream.Code
	values := []string{cr.UID, cr.CID, cr.SEID, cr.KID}
	for _, key := range []string{"uid", "sign", "account", "time"} {
		values = append(values, params.Get(key))
	}
	for _, value := range values {
		if value != "" {
			message = strings.ReplaceAll(message, value, "[已隐藏]")
			code = strings.ReplaceAll(code, value, "[已隐藏]")
		}
	}
	return &UpstreamError{Code: code, Message: message, Cause: upstream.Cause}
}
