package connections

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/drive115"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/database"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite/model"
	"gorm.io/gorm"
)

type serviceTestAdapter struct {
	status       drive115.Status
	startErr     error
	pollErr      error
	completeErr  error
	verifyErr    error
	credential   drive115.Credential
	evidence     drive115.Evidence
	completeCall int
}

func (a *serviceTestAdapter) StartQRCode(context.Context, drive115.Device) (drive115.Challenge, error) {
	return drive115.Challenge{Image: []byte("test-qr"), ExpiresAt: time.Now().Add(time.Minute)}, a.startErr
}
func (a *serviceTestAdapter) PollQRCode(context.Context, drive115.Challenge) (drive115.Status, error) {
	return a.status, a.pollErr
}
func (a *serviceTestAdapter) CompleteQRCode(context.Context, drive115.Challenge, drive115.Device) (drive115.Credential, error) {
	a.completeCall++
	return a.credential, a.completeErr
}
func (a *serviceTestAdapter) VerifyCredential(context.Context, drive115.Credential) (drive115.Evidence, error) {
	return a.evidence, a.verifyErr
}

func openServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "connections.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestQRSessionLifecycleNormalizesTerminalStates(t *testing.T) {
	for _, status := range []drive115.Status{drive115.Waiting, drive115.Scanned, drive115.Allowed, drive115.Expired, drive115.Canceled} {
		t.Run(string(status), func(t *testing.T) {
			adapter := &serviceTestAdapter{status: status}
			service := New(openServiceTestDB(t), adapter)
			started, err := service.Start(context.Background(), drive115.DeviceWeb)
			if err != nil {
				t.Fatal(err)
			}
			got, err := service.Poll(context.Background(), started.SessionID)
			if err != nil || got.Status != string(status) {
				t.Fatalf("Poll() = (%q, %v), want %q", got.Status, err, status)
			}
			if status == drive115.Expired || status == drive115.Canceled {
				if _, err := service.Complete(context.Background(), started.SessionID); !errors.Is(err, ErrNotAllowed) {
					t.Fatalf("Complete() error = %v, want ErrNotAllowed", err)
				}
				if _, err := service.QRImage(started.SessionID); !errors.Is(err, ErrSessionMissing) {
					t.Fatalf("QRImage() error = %v, want challenge to be removed", err)
				}
			}
		})
	}
}

func TestQRSessionUpstreamPollFailureBecomesFailed(t *testing.T) {
	adapter := &serviceTestAdapter{pollErr: errors.New("network unavailable")}
	service := New(openServiceTestDB(t), adapter)
	started, err := service.Start(context.Background(), drive115.DeviceWeb)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.Poll(context.Background(), started.SessionID)
	if err != nil || got.Status != string(drive115.Failed) {
		t.Fatalf("failed Poll() = (%q, %v)", got.Status, err)
	}
	if _, err := service.Complete(context.Background(), started.SessionID); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("Complete() error = %v, want ErrNotAllowed", err)
	}
}

func TestQRStartsCreateIndependentSessions(t *testing.T) {
	service := New(openServiceTestDB(t), &serviceTestAdapter{status: drive115.Waiting})
	first, err := service.Start(context.Background(), drive115.DeviceWeb)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Start(context.Background(), drive115.DeviceWechatMini)
	if err != nil {
		t.Fatal(err)
	}
	if first.SessionID == second.SessionID {
		t.Fatalf("separate QR starts reused session ID %q", first.SessionID)
	}
	if _, err := service.QRImage(first.SessionID); err != nil {
		t.Fatalf("first session image unavailable: %v", err)
	}
	if _, err := service.QRImage(second.SessionID); err != nil {
		t.Fatalf("second session image unavailable: %v", err)
	}
}

func TestQRSessionExpiresBeforePollingAndCannotComplete(t *testing.T) {
	service := New(openServiceTestDB(t), &serviceTestAdapter{status: drive115.Allowed})
	started, err := service.Start(context.Background(), drive115.DeviceWeb)
	if err != nil {
		t.Fatal(err)
	}
	q, err := service.sessions.get(started.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	q.expires = time.Now().Add(-time.Second)
	got, err := service.Poll(context.Background(), started.SessionID)
	if err != nil || got.Status != string(drive115.Expired) {
		t.Fatalf("expired Poll() = (%q, %v)", got.Status, err)
	}
	if _, err := service.Complete(context.Background(), started.SessionID); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("Complete() error = %v, want ErrNotAllowed", err)
	}
}

func TestCompletePersistsCredentialJSONAndIsIdempotent(t *testing.T) {
	db := openServiceTestDB(t)
	adapter := &serviceTestAdapter{
		status:     drive115.Allowed,
		credential: drive115.Credential{UID: "u-1", CID: "cid-secret", SEID: "seid-secret", KID: "kid-secret"},
		evidence:   drive115.Evidence{AccountID: "account-1", AccountName: "test", RootReadable: true},
	}
	service := New(db, adapter)
	started, err := service.Start(context.Background(), drive115.DeviceWeb)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Poll(context.Background(), started.SessionID); err != nil {
		t.Fatal(err)
	}
	first, err := service.Complete(context.Background(), started.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Complete(context.Background(), started.SessionID)
	if err != nil || second.ID != first.ID || adapter.completeCall != 1 {
		t.Fatalf("duplicate Complete() = (%v, %v), calls=%d", second, err, adapter.completeCall)
	}
	var row model.Connection
	if err := db.Where("id = ?", first.ID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	var stored drive115.Credential
	if err := json.Unmarshal([]byte(row.CredentialJSON), &stored); err != nil {
		t.Fatalf("credential JSON is invalid: %v", err)
	}
	if stored != adapter.credential {
		t.Fatalf("stored credential = %#v, want %#v", stored, adapter.credential)
	}
}

func TestSameAccountReauthorizationKeepsIDAndReplacesCredential(t *testing.T) {
	db := openServiceTestDB(t)
	adapter := &serviceTestAdapter{status: drive115.Allowed, credential: drive115.Credential{UID: "u1", CID: "old"}, evidence: drive115.Evidence{AccountID: "account-1", RootReadable: true}}
	service := New(db, adapter)
	first, err := completeTestSession(t, service)
	if err != nil {
		t.Fatal(err)
	}
	adapter.credential = drive115.Credential{UID: "u1", CID: "new", SEID: "new-seid"}
	second, err := completeTestSession(t, service)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("reauthorization changed connection ID: %q -> %q", first.ID, second.ID)
	}
	var row model.Connection
	if err := db.Where("id = ?", first.ID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	var credential drive115.Credential
	if err := json.Unmarshal([]byte(row.CredentialJSON), &credential); err != nil {
		t.Fatal(err)
	}
	if credential.CID != "new" {
		t.Fatalf("stored credential was not replaced: %#v", credential)
	}
}

func TestDifferentAccountAuthorizationPreservesExistingConnection(t *testing.T) {
	db := openServiceTestDB(t)
	adapter := &serviceTestAdapter{status: drive115.Allowed, credential: drive115.Credential{UID: "u1", CID: "old"}, evidence: drive115.Evidence{AccountID: "account-1", RootReadable: true}}
	service := New(db, adapter)
	first, err := completeTestSession(t, service)
	if err != nil {
		t.Fatal(err)
	}
	var before model.Connection
	if err := db.Where("id = ?", first.ID).Take(&before).Error; err != nil {
		t.Fatal(err)
	}
	adapter.credential = drive115.Credential{UID: "u2", CID: "new-secret"}
	adapter.evidence.AccountID = "account-2"
	if _, err := completeTestSession(t, service); !errors.Is(err, ErrAccountConflict) {
		t.Fatalf("different account error = %v, want ErrAccountConflict", err)
	}
	var after model.Connection
	if err := db.Where("id = ?", first.ID).Take(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID || after.ExternalAccountID != before.ExternalAccountID || after.CredentialJSON != before.CredentialJSON || after.Status != before.Status || after.CapabilitiesJSON != before.CapabilitiesJSON {
		t.Fatalf("account conflict changed existing row: before=%#v after=%#v", before, after)
	}
}

func TestIncompleteEvidenceDoesNotPersistConnection(t *testing.T) {
	for _, tt := range []struct {
		name     string
		evidence drive115.Evidence
	}{
		{name: "missing account ID", evidence: drive115.Evidence{RootReadable: true}},
		{name: "root not readable", evidence: drive115.Evidence{AccountID: "account-1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := openServiceTestDB(t)
			adapter := &serviceTestAdapter{
				status:     drive115.Allowed,
				credential: drive115.Credential{UID: "u-1", CID: "credential-secret"},
				evidence:   tt.evidence,
			}
			service := New(db, adapter)
			if _, err := completeTestSession(t, service); err == nil {
				t.Fatal("Complete() succeeded with incomplete verification evidence")
			}
			var count int64
			if err := db.Model(&model.Connection{}).Where("kind = ? AND is_active = ?", "115", true).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("incomplete verification persisted %d active connections", count)
			}
		})
	}
}

func TestPersistenceFailureLeavesNoPartialConnection(t *testing.T) {
	db := openServiceTestDB(t)
	if err := db.Exec(`CREATE TRIGGER reject_connection_insert BEFORE INSERT ON connections BEGIN SELECT RAISE(ABORT, 'simulated persistence failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	adapter := &serviceTestAdapter{
		status:     drive115.Allowed,
		credential: drive115.Credential{UID: "u-1", CID: "credential-secret"},
		evidence:   drive115.Evidence{AccountID: "account-1", RootReadable: true},
	}
	service := New(db, adapter)
	if _, err := completeTestSession(t, service); err == nil {
		t.Fatal("Complete() succeeded despite a database write failure")
	}
	var count int64
	if err := db.Model(&model.Connection{}).Where("kind = ? AND is_active = ?", "115", true).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed persistence left %d active connections", count)
	}
}

func TestConnectionTestDistinguishesNetworkAndInvalidCredential(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want string
	}{
		{name: "network remains unreachable", err: drive115.ErrUnreachable, want: "unreachable"},
		{name: "invalid credential is invalid", err: drive115.ErrCredentialInvalid, want: "invalid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := openServiceTestDB(t)
			row := model.Connection{ID: "conn", Kind: "115", Name: "account", ExternalAccountID: "account-1", CredentialJSON: `{"UID":"u","CID":"c","SEID":"s","KID":"k"}`, CapabilitiesJSON: `[]`, IsActive: true, Status: "available", CreatedAt: time.Now(), UpdatedAt: time.Now()}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			service := New(db, &serviceTestAdapter{verifyErr: tt.err})
			result, err := service.Test(context.Background())
			if err != nil || result.Status != tt.want {
				t.Fatalf("Test() = (%q, %v), want %q", result.Status, err, tt.want)
			}
			var saved model.Connection
			if err := db.Where("id = ?", row.ID).Take(&saved).Error; err != nil {
				t.Fatal(err)
			}
			if saved.Status != tt.want {
				t.Fatalf("persisted status = %q, want %q", saved.Status, tt.want)
			}
		})
	}
}

func completeTestSession(t *testing.T, service *Service) (*Connection, error) {
	t.Helper()
	started, err := service.Start(context.Background(), drive115.DeviceWeb)
	if err != nil {
		return nil, err
	}
	if _, err := service.Poll(context.Background(), started.SessionID); err != nil {
		return nil, err
	}
	return service.Complete(context.Background(), started.SessionID)
}
