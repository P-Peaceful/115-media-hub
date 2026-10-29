package drive115

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
)

type userInfo struct {
	UserID      wireInteger   `json:"user_id"`
	UserName    string        `json:"user_name"`
	Face        string        `json:"face"`
	Vip         wireInteger   `json:"vip"`
	Expire      wireInteger   `json:"expire"`
	Forever     wireInteger   `json:"forever"`
	IsPrivilege wireBool      `json:"is_privilege"`
	Privilege   privilegeInfo `json:"privilege"`
}

type privilegeInfo struct {
	Expire wireInteger `json:"expire"`
	State  wireBool    `json:"state"`
}

// Optional profile fields must not invalidate otherwise valid account evidence.
func (u *userInfo) UnmarshalJSON(data []byte) error {
	var identity struct {
		UserID   wireInteger `json:"user_id"`
		UserName string      `json:"user_name"`
	}
	if err := json.Unmarshal(data, &identity); err != nil {
		return err
	}
	u.UserID = identity.UserID
	u.UserName = identity.UserName
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for key, target := range map[string]any{"face": &u.Face, "vip": &u.Vip, "expire": &u.Expire, "forever": &u.Forever, "is_privilege": &u.IsPrivilege} {
		// A malformed optional field contributes no evidence. It never invents zero
		// capacity or turns an unknown membership into an ordinary account.
		if raw, ok := fields[key]; ok {
			_ = json.Unmarshal(raw, target)
		}
	}
	// json.Unmarshal can partially populate a struct before returning an error.
	// Commit composite optional evidence only when the entire field is valid.
	var privilege privilegeInfo
	if raw, ok := fields["privilege"]; ok && json.Unmarshal(raw, &privilege) == nil {
		u.Privilege = privilege
	}
	return nil
}
func (d *Driver) VerifyCredential(ctx context.Context, cr Credential) (Evidence, error) {
	var response struct {
		Data userInfo `json:"data"`
	}
	if err := d.getJSON(ctx, "https://my.115.com/?ct=ajax&ac=nav", nil, cr, &response); err != nil {
		return Evidence{}, err
	}
	user := response.Data
	if user.UserID == "" || user.UserID == "0" || strings.HasPrefix(string(user.UserID), "-") {
		return Evidence{}, protocolError()
	}
	profile := profileFromUser(&user)
	evidence := Evidence{AccountID: string(user.UserID), AccountName: user.UserName, Profile: profile}
	if _, err := d.ListDirectory(ctx, cr, "0", 0, 1, "all", ""); err != nil {
		if errors.Is(err, ErrCredentialInvalid) {
			return Evidence{}, err
		}
		if isNetworkFailure(err) {
			return evidence, err
		}
		var upstream *UpstreamError
		if errors.As(err, &upstream) {
			return evidence, &UpstreamError{Code: upstream.Code, Message: upstream.Message, Cause: ErrRootUnreadable}
		}
		return evidence, ErrRootUnreadable
	}
	evidence.RootReadable = true
	// Enrichment is optional. Pointer fields distinguish unavailable data from zero.
	var info struct {
		Data struct {
			Space struct {
				Total struct {
					Size *byteSize `json:"size"`
				} `json:"all_total"`
				Used struct {
					Size *byteSize `json:"size"`
				} `json:"all_use"`
				Free struct {
					Size *byteSize `json:"size"`
				} `json:"all_remain"`
			} `json:"space_info"`
			Devices *struct {
				List []struct{} `json:"list"`
			} `json:"login_devices_info"`
		} `json:"data"`
	}
	if err := d.getJSON(ctx, "https://webapi.115.com/files/index_info", nil, cr, &info); err == nil {
		size := func(v *byteSize) *int64 {
			if v == nil {
				return nil
			}
			n := int64(*v)
			return &n
		}
		evidence.Profile.StorageTotalBytes = size(info.Data.Space.Total.Size)
		evidence.Profile.StorageUsedBytes = size(info.Data.Space.Used.Size)
		evidence.Profile.StorageFreeBytes = size(info.Data.Space.Free.Size)
		if info.Data.Devices != nil && info.Data.Devices.List != nil {
			n := len(info.Data.Devices.List)
			evidence.Profile.DeviceCount = &n
		}
	}

	// Cookie vip_limit supplies the display text. Never synthesize names from
	// numeric nav grades or use an unfamiliar name to infer an entitlement.
	var membership struct {
		Data struct {
			UserLimit struct {
				Name string `json:"level_name"`
			} `json:"user_limit"`
		} `json:"data"`
	}
	if err := d.getJSON(ctx, "https://webapi.115.com/user/vip_limit", url.Values{"feature": {"2"}}, cr, &membership); err == nil && strings.TrimSpace(membership.Data.UserLimit.Name) != "" {
		evidence.Profile.MembershipName = membership.Data.UserLimit.Name
	}
	if ctx.Err() != nil {
		return evidence, ctx.Err()
	}
	return evidence, nil
}
func profileFromUser(user *userInfo) AccountProfile {
	expiry, _ := user.Privilege.Expire.int64()
	if expiry == 0 {
		expiry, _ = user.Expire.int64()
	}
	forever, _ := user.Forever.int64()
	now := time.Now().UTC()
	profile := AccountProfile{AccountName: user.UserName, AvatarURL: user.Face, AuthorizationType: "cookie", MembershipStatus: "unknown", RetrievedAt: now}
	if bool(user.IsPrivilege) || bool(user.Privilege.State) || forever > 0 {
		profile.MembershipStatus = "active"
	}
	if expiry > 0 {
		t := time.Unix(expiry, 0).UTC()
		profile.MembershipExpiresAt = &t
	}
	return profile
}
