package drive115

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/SheltonZhu/115driver/pkg/driver"
)

type Device string

const (
	DeviceWeb        Device = "web"
	DeviceAlipayMini Device = "alipaymini"
	DeviceWechatMini Device = "wechatmini"
)

type Status string

const (
	Waiting  Status = "waiting"
	Scanned  Status = "scanned"
	Allowed  Status = "allowed"
	Expired  Status = "expired"
	Canceled Status = "canceled"
	Failed   Status = "failed"
)

type Challenge struct {
	opaque    *driver.QRCodeSession
	Image     []byte
	ExpiresAt time.Time
}
type Credential struct {
	UID  string `json:"UID"`
	CID  string `json:"CID"`
	SEID string `json:"SEID"`
	KID  string `json:"KID"`
}
type Evidence struct {
	AccountID    string
	AccountName  string
	RootReadable bool
	Profile      AccountProfile
}

// AccountProfile is a safe, non-secret snapshot suitable for the management API.
type AccountProfile struct {
	AccountName         string     `json:"accountName"`
	AvatarURL           string     `json:"avatarUrl,omitempty"`
	AuthorizationType   string     `json:"authorizationType"`
	MembershipStatus    string     `json:"membershipStatus"` // active or unknown; Cookie SDK cannot distinguish an omitted VIP value from non-VIP.
	MembershipLevel     int        `json:"membershipLevel"`
	MembershipExpiresAt *time.Time `json:"membershipExpiresAt,omitempty"`
	DeviceCount         *int       `json:"deviceCount,omitempty"`
	StorageTotalBytes   *int64     `json:"storageTotalBytes,omitempty"` // Values are bytes; nil means upstream data was unavailable.
	StorageUsedBytes    *int64     `json:"storageUsedBytes,omitempty"`
	StorageFreeBytes    *int64     `json:"storageFreeBytes,omitempty"`
	RetrievedAt         time.Time  `json:"retrievedAt"`
}

// Adapter is the project-owned boundary around the upstream driver.
type Adapter interface {
	StartQRCode(context.Context, Device) (Challenge, error)
	PollQRCode(context.Context, Challenge) (Status, error)
	CompleteQRCode(context.Context, Challenge, Device) (Credential, error)
	VerifyCredential(context.Context, Credential) (Evidence, error)
}

type Driver struct{ timeout time.Duration }

func New(timeout time.Duration) *Driver {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Driver{timeout: timeout}
}

type contextRoundTripper struct {
	base http.RoundTripper
	ctx  context.Context
}

func (t contextRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(req.WithContext(t.ctx))
}

func (d *Driver) client(ctx context.Context) *driver.Pan115Client {
	return driver.New().SetHttpClient(&http.Client{Timeout: d.timeout, Transport: contextRoundTripper{base: http.DefaultTransport, ctx: ctx}})
}
func (d *Driver) StartQRCode(ctx context.Context, device Device) (Challenge, error) {
	if err := ctx.Err(); err != nil {
		return Challenge{}, err
	}
	c, err := d.client(ctx).QRCodeStart()
	if err != nil {
		return Challenge{}, safeNetworkError(err)
	}
	image, err := c.QRCode()
	if err != nil {
		return Challenge{}, safeNetworkError(err)
	}
	// QR data and signed challenge stay in this backend-only opaque value.
	return Challenge{opaque: c, Image: image, ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}, nil
}
func (d *Driver) PollQRCode(ctx context.Context, ch Challenge) (Status, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if ch.opaque == nil {
		return "", ErrInvalidChallenge
	}
	st, err := d.client(ctx).QRCodeStatus(ch.opaque)
	if err != nil {
		return "", safeNetworkError(err)
	}
	switch {
	case st.IsWaiting():
		return Waiting, nil
	case st.IsScanned():
		return Scanned, nil
	case st.IsAllowed():
		return Allowed, nil
	case st.IsExpired():
		return Expired, nil
	case st.IsCanceled():
		return Canceled, nil
	default:
		return "", ErrUnknownStatus
	}
}
func (d *Driver) CompleteQRCode(ctx context.Context, ch Challenge, device Device) (Credential, error) {
	if err := ctx.Err(); err != nil {
		return Credential{}, err
	}
	if ch.opaque == nil {
		return Credential{}, ErrInvalidChallenge
	}
	app, ok := loginApp(device)
	if !ok {
		return Credential{}, ErrInvalidDevice
	}
	cr, err := d.client(ctx).QRCodeLoginWithApp(ch.opaque, app)
	if err != nil {
		return Credential{}, safeNetworkError(err)
	}
	return Credential{UID: cr.UID, CID: cr.CID, SEID: cr.SEID, KID: cr.KID}, nil
}
func (d *Driver) VerifyCredential(ctx context.Context, cr Credential) (Evidence, error) {
	if err := ctx.Err(); err != nil {
		return Evidence{}, err
	}
	c := d.client(ctx).ImportCredential(&driver.Credential{UID: cr.UID, CID: cr.CID, SEID: cr.SEID, KID: cr.KID})
	// CookieCheck discards transport errors in this SDK version; GetUser retains
	// its error so an outage cannot be mistaken for an invalid credential.
	user, err := c.GetUser()
	if err != nil {
		if errors.Is(err, driver.ErrNotLogin) || errors.Is(err, driver.ErrCredentialInvalid) || errors.Is(err, driver.ErrBadCookie) {
			return Evidence{}, safeCredentialError(err)
		}
		return Evidence{}, safeNetworkError(err)
	}
	profile := profileFromUser(user)
	// Listing root is read-only and is the evidence required by the contract.
	if _, err := c.List("0"); err != nil {
		if errors.Is(err, driver.ErrNotLogin) || errors.Is(err, driver.ErrCredentialInvalid) || errors.Is(err, driver.ErrBadCookie) {
			return Evidence{}, safeCredentialError(err)
		}
		if isTransportError(err) {
			return Evidence{}, safeNetworkError(err)
		}
		return Evidence{AccountID: formatID(user.UserID), AccountName: user.UserName, Profile: profile}, safeRootError(err)
	}
	// Space data is optional profile enrichment; its absence must not invalidate authorization.
	if info, infoErr := c.GetInfo(); infoErr == nil {
		profile.StorageTotalBytes = int64Pointer(info.SpaceInfo.AllTotal.Size)
		profile.StorageUsedBytes = int64Pointer(info.SpaceInfo.AllUse.Size)
		profile.StorageFreeBytes = int64Pointer(info.SpaceInfo.AllRemain.Size)
		deviceCount := len(info.LoginDevicesInfo.List)
		profile.DeviceCount = &deviceCount
	}
	return Evidence{AccountID: formatID(user.UserID), AccountName: user.UserName, RootReadable: true, Profile: profile}, nil
}

func profileFromUser(user *driver.UserInfo) AccountProfile {
	expiry := user.Privilege.Expire
	if expiry == 0 {
		expiry = user.Expire
	}
	now := time.Now().UTC()
	profile := AccountProfile{
		AccountName:       user.UserName,
		AvatarURL:         user.Face,
		AuthorizationType: "cookie",
		MembershipLevel:   user.Vip,
		MembershipStatus:  "unknown",
		RetrievedAt:       time.Now().UTC(),
	}
	// The Cookie user-info endpoint exposes the raw VIP level in `vip`; it is
	// independent of the privilege booleans and must also identify VIP users.
	// An elapsed expiry takes precedence over a positive historical VIP level.
	vipLevelIsCurrent := user.Vip > 0 && (expiry == 0 || time.Unix(int64(expiry), 0).After(now))
	if user.IsPrivilege || user.Privilege.State || user.Forever > 0 || vipLevelIsCurrent {
		profile.MembershipStatus = "active"
	}
	if expiry > 0 {
		value := time.Unix(int64(expiry), 0).UTC()
		profile.MembershipExpiresAt = &value
	}
	return profile
}

func int64Pointer(value int64) *int64 {
	if value < 0 {
		return nil
	}
	return &value
}
