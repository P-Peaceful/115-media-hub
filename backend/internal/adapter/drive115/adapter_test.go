package drive115

import (
	"testing"
	"time"

	"strconv"
)

func TestProfileFromUserDoesNotInferMembershipFromGrades(t *testing.T) {
	tests := []struct {
		name string
		user userInfo
		want string
	}{
		{
			name: "positive VIP level without entitlement is unknown",
			user: userInfo{Vip: "2"},
			want: "unknown",
		},
		{
			name: "expired VIP level is unknown rather than ordinary",
			user: userInfo{Vip: "2", Expire: wireInteger(strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10))},
			want: "unknown",
		},
		{
			name: "explicit upstream entitlement remains active",
			user: userInfo{IsPrivilege: true},
			want: "active",
		},
		{
			name: "zero VIP level without entitlement signals is unknown",
			user: userInfo{},
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
