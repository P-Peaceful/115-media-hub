package drive115

import (
	"context"
	"time"
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
	opaque    *qrSession
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

// DirectoryEntry contains only the safe fields required by directory browsing.
type DirectoryEntry struct {
	ID, ParentID, Name string
	IsDirectory        bool
	Size               int64
	ModifiedAt         time.Time
	FileType           string
}

// DirectoryPage is one bounded upstream response; Total comes directly from 115.
type DirectoryPage struct {
	Entries []DirectoryEntry
	Total   int64
}

type Breadcrumb struct{ ID, Name string }
type Directory struct {
	ID, Name    string
	Breadcrumbs []Breadcrumb
}

// AccountProfile is a safe, non-secret snapshot suitable for the management API.
type AccountProfile struct {
	AccountName         string     `json:"accountName"`
	AvatarURL           string     `json:"avatarUrl,omitempty"`
	AuthorizationType   string     `json:"authorizationType"`
	MembershipStatus    string     `json:"membershipStatus"` // active or unknown; missing membership evidence stays unknown.
	MembershipName      string     `json:"membershipName,omitempty"`
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

// BrowserAdapter adds read-only single-page directory operations.
type BrowserAdapter interface {
	ListDirectory(context.Context, Credential, string, int, int, string, string) (DirectoryPage, error)
	StatDirectory(context.Context, Credential, string) (Directory, error)
}
