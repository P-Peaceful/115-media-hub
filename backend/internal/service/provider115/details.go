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

// AccountProfile contains only fields returned by the provider's nav endpoint.
type AccountProfile struct {
	UserName        string     `json:"user_name"`
	UserID          *string    `json:"user_id"`
	Avatar          *string    `json:"avatar"`
	IsMember        *bool      `json:"is_member"`
	MemberExpiresAt *time.Time `json:"member_expires_at"`
	MemberForever   *bool      `json:"member_forever"`
}

// AccountSpace contains byte counts as decimal strings, avoiding JavaScript precision loss.
type AccountSpace struct {
	UsedBytes  string `json:"used_bytes"`
	TotalBytes string `json:"total_bytes"`
}

// AccountDetails distinguishes provider authorization from local configuration and optional space information.
type AccountDetails struct {
	Status        string          `json:"status"`
	Profile       *AccountProfile `json:"profile"`
	Space         *AccountSpace   `json:"space"`
	SpaceStatus   string          `json:"space_status"`
	Devices       []AccountDevice `json:"devices"`
	DevicesStatus string          `json:"devices_status"`
}

type AccountDevice struct {
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	IP        string     `json:"ip"`
	City      string     `json:"city"`
	LoginAt   *time.Time `json:"login_at"`
	IsCurrent bool       `json:"is_current"`
}
type credentialRepository interface {
	ProviderCredential(context.Context) (string, error)
}

// Details requests fresh account information without exposing the stored credential.
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
	// A replaced or removed account must not publish an in-flight response for
	// the previously stored credential.
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

func parseString(raw json.RawMessage) *string {
	value := parseOptionalString(raw)
	if value == "" {
		return nil
	}
	return &value
}

func parseOptionalString(raw json.RawMessage) string {
	var value string
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}

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

func parseBoolLike(raw json.RawMessage) *bool {
	value := parseInteger(raw)
	if value == nil {
		return nil
	}
	result := *value != 0
	return &result
}

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

func parseUnixTime(raw json.RawMessage) *time.Time {
	seconds := parseInteger(raw)
	if seconds == nil || *seconds <= 0 {
		return nil
	}
	value := time.Unix(*seconds, 0).UTC()
	return &value
}

func parseBytes(raw json.RawMessage) (string, bool) {
	text := strings.TrimSpace(string(raw))
	if strings.HasPrefix(text, `"`) {
		if json.Unmarshal(raw, &text) != nil {
			return "", false
		}
	}
	f, _, err := big.ParseFloat(text, 10, 256, big.ToZero)
	// Bound upstream values before converting to an integer to avoid enormous allocations.
	if err != nil || f.IsInf() || f.Sign() < 0 || f.MantExp(nil) > 256 {
		return "", false
	}
	integer, _ := f.Int(nil)
	if integer == nil {
		return "", false
	}
	return integer.String(), true
}
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
