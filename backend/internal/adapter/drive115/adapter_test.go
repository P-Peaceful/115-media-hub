package drive115

import (
	"testing"
	"time"

	"github.com/SheltonZhu/115driver/pkg/driver"
)

func TestProfileFromUserMapsVIPLevelToMembershipStatus(t *testing.T) {
	tests := []struct {
		name string
		user driver.UserInfo
		want string
	}{
		{
			name: "positive VIP level is active without privilege flags",
			user: driver.UserInfo{Vip: 2},
			want: "active",
		},
		{
			name: "expired VIP level is unknown rather than ordinary",
			user: driver.UserInfo{Vip: 2, Expire: int(time.Now().Add(-time.Hour).Unix())},
			want: "unknown",
		},
		{
			name: "zero VIP level without entitlement signals is unknown",
			user: driver.UserInfo{},
			want: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := profileFromUser(&tt.user)
			if got.MembershipStatus != tt.want {
				t.Fatalf("MembershipStatus = %q, want %q", got.MembershipStatus, tt.want)
			}
		})
	}
}
