package connections

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/drive115"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite/model"
	"gorm.io/gorm"
)

var ErrAccountConflict = errors.New("115 account differs from existing connection")
var ErrNotAllowed = errors.New("QR session is not authorized")
var ErrConnectionMissing = errors.New("115 connection not found")

type Capability struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}
type Connection struct {
	ID                string                  `json:"id"`
	Name              string                  `json:"name"`
	ExternalAccountID string                  `json:"externalAccountId"`
	Status            string                  `json:"status"`
	Capabilities      []Capability            `json:"capabilities"`
	Profile           drive115.AccountProfile `json:"profile"`
	UpdatedAt         time.Time               `json:"updatedAt"`
}

// profileSnapshot versions persisted profile JSON independently from credentials.
type profileSnapshot struct {
	SchemaVersion int                     `json:"schema_version"`
	Profile       drive115.AccountProfile `json:"profile"`
}
type Session struct {
	SessionID    string    `json:"sessionId"`
	Device       string    `json:"device"`
	ExpiresAt    time.Time `json:"expiresAt"`
	Status       string    `json:"status"`
	Message      string    `json:"message,omitempty"`
	UpstreamCode string    `json:"upstreamCode,omitempty"`
}
type Service struct {
	db       *gorm.DB
	repo     sqlite.ConnectionRepository
	driver   drive115.Adapter
	sessions *sessionManager
}

func New(db *gorm.DB, adapter drive115.Adapter) *Service {
	return &Service{db: db, repo: sqlite.ConnectionRepository{DB: db}, driver: adapter, sessions: newSessionManager()}
}

func (s *Service) Current(ctx context.Context) (*Connection, error) {
	row, err := s.repo.Active115(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "115 connection lookup failed", "error", err)
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	return summary(row), nil
}
func (s *Service) Start(ctx context.Context, device drive115.Device) (Session, error) {
	switch device {
	case drive115.DeviceWeb, drive115.DeviceAlipayMini, drive115.DeviceWechatMini:
	default:
		return Session{}, drive115.ErrInvalidDevice
	}
	ch, err := s.driver.StartQRCode(ctx, device)
	if err != nil {
		slog.ErrorContext(ctx, "115 QR session creation failed", "device", device, "stage", "upstream_start", "error", err)
		return Session{}, err
	}
	q, err := s.sessions.add(device, ch)
	if err != nil {
		slog.ErrorContext(ctx, "115 QR session creation failed", "device", device, "stage", "session_store", "error", err)
		return Session{}, err
	}
	slog.InfoContext(ctx, "115 QR session created", "device", device, "expires_at", q.expires)
	return sessionSummary(q), nil
}
func (s *Service) QRImage(id string) ([]byte, error) {
	q, err := s.sessions.get(id)
	if err != nil {
		return nil, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if !time.Now().Before(q.expires) {
		s.failSession(q)
		return nil, ErrSessionMissing
	}
	if len(q.image) == 0 {
		return nil, ErrSessionMissing
	}
	return append([]byte(nil), q.image...), nil
}
func (s *Service) Poll(ctx context.Context, id string) (Session, error) {
	q, err := s.sessions.get(id)
	if err != nil {
		return Session{}, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if !time.Now().Before(q.expires) {
		previous := q.status
		q.status = drive115.Expired
		s.sessions.removeChallenge(q)
		slog.Info("115 QR session status changed", "from_status", previous, "to_status", q.status)
		return sessionSummary(q), nil
	}
	if q.status == drive115.Allowed || q.status == drive115.Expired || q.status == drive115.Canceled || q.status == drive115.Failed {
		return sessionSummary(q), nil
	}
	status, err := s.driver.PollQRCode(ctx, q.challenge)
	if err != nil {
		s.failSession(q)
		if upstream, ok := drive115.AsUpstreamError(err); ok {
			q.message, q.upstreamCode = upstream.Message, upstream.Code
		}
		slog.ErrorContext(ctx, "115 QR session polling failed", "status", q.status, "error", err)
		return sessionSummary(q), nil
	}
	previous := q.status
	q.status = status
	if status == drive115.Expired || status == drive115.Canceled {
		s.sessions.removeChallenge(q)
	}
	if status != previous {
		slog.InfoContext(ctx, "115 QR session status changed", "from_status", previous, "to_status", status)
	}
	return sessionSummary(q), nil
}
func (s *Service) Complete(ctx context.Context, id string) (*Connection, error) {
	q, err := s.sessions.get(id)
	if err != nil {
		return nil, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.completed != nil {
		return q.completed, nil
	}
	if q.status != drive115.Allowed {
		return nil, ErrNotAllowed
	}
	cr, err := s.driver.CompleteQRCode(ctx, q.challenge, q.device)
	if err != nil {
		s.failSession(q)
		slog.ErrorContext(ctx, "115 QR authorization completion failed", "device", q.device, "stage", "upstream_complete", "error", err)
		return nil, err
	}
	defer func() { cr = drive115.Credential{} }()
	evidence, err := s.driver.VerifyCredential(ctx, cr)
	if err != nil {
		s.failSession(q)
		slog.ErrorContext(ctx, "115 QR authorization completion failed", "device", q.device, "stage", "credential_verify", "error", err)
		return nil, err
	}
	if evidence.AccountID == "" || !evidence.RootReadable {
		s.failSession(q)
		err = errors.New("115 verification incomplete")
		slog.ErrorContext(ctx, "115 QR authorization completion failed", "device", q.device, "stage", "verification_evidence", "root_readable", evidence.RootReadable, "error", err)
		return nil, err
	}
	old, err := s.repo.Active115(ctx)
	if err != nil {
		s.failSession(q)
		slog.ErrorContext(ctx, "115 QR authorization completion failed", "device", q.device, "stage", "load_existing_connection", "error", err)
		return nil, err
	}
	idValue := q.id
	if old != nil {
		idValue = old.ID
		if old.ExternalAccountID != evidence.AccountID {
			s.failSession(q)
			slog.WarnContext(ctx, "115 QR authorization rejected", "device", q.device, "reason", "account_conflict")
			return nil, ErrAccountConflict
		}
	}
	plain, err := json.Marshal(cr)
	if err != nil {
		s.failSession(q)
		err = fmt.Errorf("encode 115 credential: %w", err)
		slog.ErrorContext(ctx, "115 QR authorization completion failed", "device", q.device, "stage", "encode_credential", "error", err)
		return nil, err
	}
	defer clear(plain)
	caps, _ := json.Marshal([]Capability{{Name: "account_identity", Status: "available"}, {Name: "root_directory_read", Status: "available"}})
	profile, err := marshalProfile(evidence.Profile)
	if err != nil {
		s.failSession(q)
		err = fmt.Errorf("encode 115 account profile: %w", err)
		slog.ErrorContext(ctx, "115 QR authorization completion failed", "device", q.device, "stage", "encode_profile", "error", err)
		return nil, err
	}
	now := time.Now().UTC()
	row := &model.Connection{ID: idValue, Kind: "115", Name: evidence.AccountName, ExternalAccountID: evidence.AccountID, CredentialJSON: string(plain), CapabilitiesJSON: string(caps), ProfileJSON: string(profile), IsActive: true, Status: "available", CreatedAt: now, UpdatedAt: now}
	if old != nil {
		row.CreatedAt = old.CreatedAt
	}
	// Validate singleton ownership and persist the plaintext credential atomically; never hold a DB transaction over upstream I/O.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if e := tx.Model(&model.Connection{}).Where("kind = ? AND is_active = ? AND id <> ?", "115", true, idValue).Count(&count).Error; e != nil {
			return e
		}
		if count > 0 {
			return ErrAccountConflict
		}
		return tx.Save(row).Error
	})
	if err != nil {
		s.failSession(q)
		slog.ErrorContext(ctx, "115 QR authorization persistence failed", "device", q.device, "error", err)
		return nil, err
	}
	s.sessions.removeChallenge(q)
	q.status = drive115.Allowed
	q.completed = summary(row)
	action := "created"
	if old != nil {
		action = "updated"
	}
	slog.InfoContext(ctx, "115 connection authorized", "device", q.device, "action", action, "capabilities", len(q.completed.Capabilities))
	return q.completed, nil
}

// A completion attempt consumes upstream authorization unpredictably; on any
// failure discard all challenge material and require a fresh QR session.
func (s *Service) failSession(q *qrSession) {
	q.status = drive115.Failed
	s.sessions.removeChallenge(q)
}
func (s *Service) ImageSessionForTest(id string) (string, error) {
	q, e := s.sessions.get(id)
	if e != nil {
		return "", e
	}
	return q.id, nil
}
func sessionSummary(q *qrSession) Session {
	return Session{SessionID: q.id, Device: string(q.device), ExpiresAt: q.expires, Status: string(q.status), Message: q.message, UpstreamCode: q.upstreamCode}
}
func summary(r *model.Connection) *Connection {
	var caps []Capability
	_ = json.Unmarshal([]byte(r.CapabilitiesJSON), &caps)
	var snapshot profileSnapshot
	_ = json.Unmarshal([]byte(r.ProfileJSON), &snapshot)
	profile := snapshot.Profile
	if profile.MembershipStatus == "" {
		profile.MembershipStatus = "unknown"
	}
	profile.MembershipStatus = normalizeMembershipStatus(profile, time.Now().UTC())
	return &Connection{ID: r.ID, Name: r.Name, ExternalAccountID: r.ExternalAccountID, Status: r.Status, Capabilities: caps, Profile: profile, UpdatedAt: r.UpdatedAt}
}

// normalizeMembershipStatus repairs legacy snapshots where a raw SDK VIP level
// was saved alongside an incorrect ordinary status. It only changes the API
// projection; the stored profile remains untouched until a profile refresh.
func normalizeMembershipStatus(profile drive115.AccountProfile, now time.Time) string {
	if profile.MembershipStatus == "active" {
		return "active"
	}
	if profile.MembershipLevel > 0 {
		if profile.MembershipExpiresAt == nil || profile.MembershipExpiresAt.After(now) {
			return "active"
		}
		return "unknown"
	}
	if profile.MembershipStatus == "ordinary" {
		// The Cookie SDK does not retain whether a zero VIP field was present,
		// so zero alone cannot prove the account is ordinary.
		return "unknown"
	}
	return profile.MembershipStatus
}

// RefreshProfile updates only the safe profile snapshot; failure does not alter connection status or credentials.
func (s *Service) RefreshProfile(ctx context.Context) (connection *Connection, err error) {
	defer func() {
		if err != nil {
			slog.ErrorContext(ctx, "115 account profile refresh failed", "error", err)
			return
		}
		slog.InfoContext(ctx, "115 account profile refreshed")
	}()
	row, err := s.repo.Active115(ctx)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrConnectionMissing
	}
	var cr drive115.Credential
	if err = json.Unmarshal([]byte(row.CredentialJSON), &cr); err != nil {
		return nil, errors.New("stored 115 credential is invalid")
	}
	defer func() { cr = drive115.Credential{} }()
	evidence, err := s.driver.VerifyCredential(ctx, cr)
	if err != nil {
		return nil, err
	}
	if evidence.AccountID != row.ExternalAccountID {
		return nil, ErrAccountConflict
	}
	profile, err := marshalProfile(evidence.Profile)
	if err != nil {
		return nil, fmt.Errorf("encode 115 account profile: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(&model.Connection{}).Where("id = ? AND kind = ? AND is_active = ?", row.ID, "115", true).Update("profile_json", string(profile)).Error; err != nil {
		return nil, err
	}
	row.ProfileJSON = string(profile)
	return summary(row), nil
}

func marshalProfile(profile drive115.AccountProfile) ([]byte, error) {
	return json.Marshal(profileSnapshot{SchemaVersion: 1, Profile: profile})
}

type TestResult struct {
	Status       string
	Capabilities []Capability
	CheckedAt    time.Time
	Message      string
	UpstreamCode string
}

func (s *Service) Test(ctx context.Context) (result TestResult, err error) {
	defer func() {
		if err != nil {
			slog.ErrorContext(ctx, "115 connection test failed", "error", err)
		}
	}()
	at := time.Now().UTC()
	row, err := s.repo.Active115(ctx)
	if err != nil {
		return TestResult{}, err
	}
	if row == nil {
		return TestResult{}, ErrConnectionMissing
	}
	var cr drive115.Credential
	if err = json.Unmarshal([]byte(row.CredentialJSON), &cr); err != nil {
		return TestResult{Status: "unknown", CheckedAt: at}, errors.New("stored 115 credential is invalid")
	}
	defer func() { cr = drive115.Credential{} }()
	_, verifyErr := s.driver.VerifyCredential(ctx, cr)
	result = TestResult{Status: "available", Capabilities: []Capability{{Name: "account_identity", Status: "available"}, {Name: "root_directory_read", Status: "available"}}, CheckedAt: at}
	if verifyErr != nil {
		if upstream, ok := drive115.AsUpstreamError(verifyErr); ok {
			result.Message, result.UpstreamCode = upstream.Message, upstream.Code
		}
		result.Status = "unreachable"
		result.Capabilities = []Capability{{Name: "account_identity", Status: "unknown"}, {Name: "root_directory_read", Status: "unknown"}}
		if errors.Is(verifyErr, drive115.ErrLimited) {
			result.Status = "limited"
		}
		if errors.Is(verifyErr, drive115.ErrCredentialInvalid) {
			result.Status = "invalid"
			result.Capabilities[0].Status = "unavailable"
			result.Capabilities[1].Status = "unavailable"
		}
		if errors.Is(verifyErr, drive115.ErrRootUnreadable) {
			result.Status = "limited"
			result.Capabilities[0].Status = "available"
			result.Capabilities[1].Status = "unavailable"
		}
	}
	if err = s.persistCheck(ctx, row.ID, result); err != nil {
		return TestResult{}, err
	}
	if result.Status == "available" {
		slog.InfoContext(ctx, "115 connection test completed", "status", result.Status)
	} else {
		slog.WarnContext(ctx, "115 connection test completed", "status", result.Status, "upstream_code", result.UpstreamCode)
	}
	return result, nil
}
func (s *Service) persistCheck(ctx context.Context, id string, result TestResult) error {
	caps, err := json.Marshal(result.Capabilities)
	if err != nil {
		return err
	}
	return s.repo.UpdateCheck(ctx, id, result.Status, string(caps), result.CheckedAt)
}
