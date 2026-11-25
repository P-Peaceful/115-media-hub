// details.go：负责读取 115 账号资料、容量及设备信息，兼容可选字段并阻止旧凭据的在途结果返回。

package provider115

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AccountProfile 描述上游资料接口实际提供的公开账号字段，指针为空表示未知。
type AccountProfile struct {
	UserName        string     `json:"user_name"`
	UserID          *string    `json:"user_id"`
	Avatar          *string    `json:"avatar"`
	IsMember        *bool      `json:"is_member"`
	MemberExpiresAt *time.Time `json:"member_expires_at"`
	MemberForever   *bool      `json:"member_forever"`
}

// AccountSpace 以十进制字符串表示容量字节数，避免前端整数精度丢失。
type AccountSpace struct {
	UsedBytes  string `json:"used_bytes"`
	TotalBytes string `json:"total_bytes"`
}

// AccountDetails 区分本地未配置、上游授权状态和容量、设备信息的可用性。
type AccountDetails struct {
	Status        string          `json:"status"`
	Profile       *AccountProfile `json:"profile"`
	Space         *AccountSpace   `json:"space"`
	SpaceStatus   string          `json:"space_status"`
	Devices       []AccountDevice `json:"devices"`
	DevicesStatus string          `json:"devices_status"`
}

// AccountDevice 描述上游返回的登录设备信息，不包含设备凭据。
type AccountDevice struct {
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	IP        string     `json:"ip"`
	City      string     `json:"city"`
	LoginAt   *time.Time `json:"login_at"`
	IsCurrent bool       `json:"is_current"`
}

// credentialRepository 提供当前存储的凭据，仅供上游请求边界使用。
type credentialRepository interface {
	ProviderCredential(context.Context) (string, error)
}

// Details 读取最新账号资料；容量和设备信息允许部分不可用，返回前再次校验凭据未被替换。
func (s *Service) Details(ctx context.Context, correlation string) (result AccountDetails, resultErr error) {
	start := time.Now()
	defer func() {
		if resultErr != nil {
			s.log(ctx, "account_details", correlation, start, resultErr)
		}
	}()
	result = AccountDetails{Status: "unconfigured", SpaceStatus: "unavailable", Devices: []AccountDevice{}, DevicesStatus: "unavailable"}
	repo, ok := s.repo.(credentialRepository)
	if !ok {
		return result, ErrUpstream
	}
	cookie, err := repo.ProviderCredential(ctx)
	if err != nil {
		return result, err
	}
	if cookie == "" {
		return result, nil
	}
	// 授权被替换或删除后，不得返回旧凭据对应的在途资料。
	defer func() {
		if resultErr != nil {
			return
		}
		current, err := repo.ProviderCredential(ctx)
		if err != nil || current != cookie {
			resultErr = ErrUpstream
			result = AccountDetails{}
		}
	}()
	body, err := s.accountRequest(ctx, s.navURL, cookie)
	if err != nil {
		return result, ErrUpstream
	}
	var nav struct {
		State upstreamState   `json:"state"`
		Error string          `json:"error"`
		Data  json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &nav) != nil {
		return result, ErrUpstream
	}
	if !bool(nav.State) {
		if strings.Contains(nav.Error, "请先登录") || strings.Contains(nav.Error, "未登录") {
			result.Status = "invalid"
			return result, nil
		}
		return result, ErrUpstream
	}
	var data struct {
		UserName string          `json:"user_name"`
		UserID   json.RawMessage `json:"user_id"`
		Face     json.RawMessage `json:"face"`
		VIP      json.RawMessage `json:"vip"`
		Expire   json.RawMessage `json:"expire"`
		Forever  json.RawMessage `json:"forever"`
	}
	if json.Unmarshal(nav.Data, &data) != nil || data.UserName == "" {
		return result, ErrUpstream
	}
	profile := &AccountProfile{UserName: data.UserName, Avatar: parseString(data.Face)}
	profile.IsMember = parseMember(data.VIP)
	profile.MemberExpiresAt = parseUnixTime(data.Expire)
	profile.MemberForever = parseBoolLike(data.Forever)
	if len(data.UserID) > 0 && string(data.UserID) != "null" {
		profile.UserID = parseID(data.UserID)
	}
	result.Status = "valid"
	result.Profile = profile
	spaceBody, err := s.accountRequest(ctx, s.spaceURL, cookie)
	if err != nil {
		s.log(ctx, "account_space", correlation, start, err)
		return result, nil
	}
	var space struct {
		State upstreamState `json:"state"`
		Data  struct {
			SpaceInfo struct {
				AllUse struct {
					Size json.RawMessage `json:"size"`
				} `json:"all_use"`
				AllTotal struct {
					Size json.RawMessage `json:"size"`
				} `json:"all_total"`
			} `json:"space_info"`
			LoginDevicesInfo json.RawMessage `json:"login_devices_info"`
		} `json:"data"`
	}
	if json.Unmarshal(spaceBody, &space) != nil || !bool(space.State) {
		s.log(ctx, "account_space", correlation, start, ErrUpstream)
		return result, nil
	}
	used, okUsed := parseBytes(space.Data.SpaceInfo.AllUse.Size)
	total, okTotal := parseBytes(space.Data.SpaceInfo.AllTotal.Size)
	if !okUsed || !okTotal {
		s.log(ctx, "account_space", correlation, start, ErrUpstream)
		return result, nil
	}
	result.Space = &AccountSpace{UsedBytes: used, TotalBytes: total}
	result.SpaceStatus = "available"
	if len(space.Data.LoginDevicesInfo) > 0 && string(space.Data.LoginDevicesInfo) != "null" {
		var deviceInfo struct {
			List []json.RawMessage `json:"list"`
		}
		if json.Unmarshal(space.Data.LoginDevicesInfo, &deviceInfo) == nil && deviceInfo.List != nil {
			devices := make([]AccountDevice, 0, len(deviceInfo.List))
			valid := true
			for _, raw := range deviceInfo.List {
				var device struct {
					Name      json.RawMessage `json:"name"`
					Type      json.RawMessage `json:"device"`
					IP        json.RawMessage `json:"ip"`
					City      json.RawMessage `json:"city"`
					Utime     json.RawMessage `json:"utime"`
					IsCurrent json.RawMessage `json:"is_current"`
				}
				if json.Unmarshal(raw, &device) != nil {
					valid = false
					break
				}
				current := parseBoolLike(device.IsCurrent)
				devices = append(devices, AccountDevice{
					Name: parseOptionalString(device.Name), Type: parseOptionalString(device.Type),
					IP: parseOptionalString(device.IP), City: parseOptionalString(device.City),
					LoginAt: parseUnixTime(device.Utime), IsCurrent: current != nil && *current,
				})
			}
			if valid {
				result.Devices = devices
				result.DevicesStatus = "available"
			}
		}
	}
	return result, nil
}

// parseMember 只识别已知会员状态 0、1、2，未知或畸形值保留为空。
func parseMember(raw json.RawMessage) *bool {
	value := parseInteger(raw)
	if value == nil {
		return nil
	}
	member := *value == 1 || *value == 2
	if *value != 0 && !member {
		return nil
	}
	return &member
}

// parseString 解析可选的非空字符串，缺失或格式错误时返回空指针。
func parseString(raw json.RawMessage) *string {
	value := parseOptionalString(raw)
	if value == "" {
		return nil
	}
	return &value
}

// parseOptionalString 仅接受 JSON 字符串，缺失、null 或类型错误时返回空串。
func parseOptionalString(raw json.RawMessage) string {
	var value string
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}

// parseID 兼容字符串和数字形式的大整数 ID，不经浮点数转换。
func parseID(raw json.RawMessage) *string {
	var value json.Number
	if json.Unmarshal(raw, &value) == nil {
		text := value.String()
		if _, ok := new(big.Int).SetString(text, 10); ok {
			return &text
		}
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if _, ok := new(big.Int).SetString(text, 10); ok {
			return &text
		}
	}
	return nil
}

// parseBoolLike 将整数形式的零映射为假、非零映射为真，无法解析时保留未知。
func parseBoolLike(raw json.RawMessage) *bool {
	value := parseInteger(raw)
	if value == nil {
		return nil
	}
	result := *value != 0
	return &result
}

// parseInteger 解析数字或数字字符串形式的有符号 64 位整数，拒绝小数和越界值。
func parseInteger(raw json.RawMessage) *int64 {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return nil
	}
	if strings.HasPrefix(text, `"`) {
		if json.Unmarshal(raw, &text) != nil {
			return nil
		}
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return nil
	}
	return &value
}

// parseUnixTime 将正整数秒级时间戳转换为 UTC，未知或非正值返回空指针。
func parseUnixTime(raw json.RawMessage) *time.Time {
	seconds := parseInteger(raw)
	if seconds == nil || *seconds <= 0 {
		return nil
	}
	value := time.Unix(*seconds, 0).UTC()
	return &value
}

// parseBytes 解析非负容量并截为整数字符串；限制数值指数以避免异常上游数据造成巨大分配。
func parseBytes(raw json.RawMessage) (string, bool) {
	text := strings.TrimSpace(string(raw))
	if strings.HasPrefix(text, `"`) {
		if json.Unmarshal(raw, &text) != nil {
			return "", false
		}
	}
	f, _, err := big.ParseFloat(text, 10, 256, big.ToZero)
	// 转整数前限制上游数值范围，避免异常容量导致巨大内存分配。
	if err != nil || f.IsInf() || f.Sign() < 0 || f.MantExp(nil) > 256 {
		return "", false
	}
	integer, _ := f.Int(nil)
	if integer == nil {
		return "", false
	}
	return integer.String(), true
}

// accountRequest 携带 Cookie 发起 15 秒只读请求，禁用跳转以免转发凭据，限制响应读取并关闭响应体。
func (s *Service) accountRequest(ctx context.Context, address, cookie string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 115Browser/36.0.0")
	req.Header.Set("Referer", "https://115.com/")
	client := *s.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	closeErr := resp.Body.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, ErrUpstream
	}
	return data, nil
}
