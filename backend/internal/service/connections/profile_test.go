package connections

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/drive115"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite/model"
)

func TestLegacyMembershipSnapshotDoesNotSynthesizeNameOrStatus(t *testing.T) {
	for _, status := range []string{"ordinary", "active", "unknown"} {
		stored := `{"schema_version":1,"profile":{"membershipStatus":"` + status + `","membershipLevel":8}}`
		got := summary(&model.Connection{ProfileJSON: stored})
		if got.Profile.MembershipName != "" || got.Profile.MembershipStatus != status {
			t.Fatalf("legacy snapshot synthesized evidence %+v", got.Profile)
		}
		data, err := json.Marshal(got.Profile)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		json.Unmarshal(data, &fields)
		if _, ok := fields["membershipLevel"]; ok {
			t.Fatal("numeric legacy field escaped projection")
		}
	}
}

func TestRefreshFillsUpstreamMembershipNameForLegacySnapshot(t *testing.T) {
	db := openServiceTestDB(t)
	const credential = `{"UID":"uid","CID":"cid","SEID":"seid","KID":"kid"}`
	row := model.Connection{ID: "membership-connection", Kind: "115", Name: "account", ExternalAccountID: "123", CredentialJSON: credential, CapabilitiesJSON: "[]", ProfileJSON: `{"schema_version":1,"profile":{"membershipLevel":8,"membershipStatus":"active"}}`, IsActive: true, Status: "available", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	adapter := &serviceTestAdapter{evidence: drive115.Evidence{AccountID: "123", RootReadable: true, Profile: drive115.AccountProfile{MembershipName: "  未来会员 · 原文  ", MembershipStatus: "unknown"}}}
	service := New(db, adapter)
	before := summary(&row)
	if before.Profile.MembershipName != "" {
		t.Fatal("legacy name fabricated")
	}
	after, err := service.RefreshProfile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if after.Profile.MembershipName != "  未来会员 · 原文  " {
		t.Fatalf("name changed: %q", after.Profile.MembershipName)
	}
	var saved model.Connection
	if err := db.First(&saved, "id = ?", row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.CredentialJSON != credential || saved.Status != "available" {
		t.Fatal("profile refresh changed authorization")
	}
	restored := summary(&saved)
	if restored.Profile.MembershipName != after.Profile.MembershipName {
		t.Fatal("name did not survive persistence")
	}
	var snapshot map[string]any
	json.Unmarshal([]byte(saved.ProfileJSON), &snapshot)
	profile := snapshot["profile"].(map[string]any)
	if _, ok := profile["membershipLevel"]; ok {
		t.Fatal("legacy grade persisted on refresh")
	}
}
