package connections

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/drive115"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite/model"
)

func TestSummaryNormalizesLegacyMembershipSnapshot(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name       string
		level      int
		expiresAt  *time.Time
		wantStatus string
	}{
		{
			name:       "positive current VIP level repairs stale ordinary status",
			level:      2,
			wantStatus: "active",
		},
		{
			name:       "positive expired VIP level is unknown",
			level:      2,
			expiresAt:  timePointer(now.Add(-time.Hour)),
			wantStatus: "unknown",
		},
		{
			name:       "zero VIP level does not prove ordinary membership",
			wantStatus: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profileJSON, err := json.Marshal(profileSnapshot{
				SchemaVersion: 1,
				Profile: drive115.AccountProfile{
					MembershipStatus:    "ordinary",
					MembershipLevel:     tt.level,
					MembershipExpiresAt: tt.expiresAt,
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			connection := summary(&model.Connection{ProfileJSON: string(profileJSON)})
			if got := connection.Profile.MembershipStatus; got != tt.wantStatus {
				t.Fatalf("MembershipStatus = %q, want %q", got, tt.wantStatus)
			}
			if tt.level > 0 && connection.Profile.MembershipLevel != tt.level {
				t.Fatalf("MembershipLevel = %d, want %d", connection.Profile.MembershipLevel, tt.level)
			}
		})
	}
}

func timePointer(value time.Time) *time.Time { return &value }
