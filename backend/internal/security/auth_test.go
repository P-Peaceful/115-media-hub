package security

import (
	"errors"
	"testing"
	"time"
)

func TestSessionHasFixedTwentyFourHourLifetime(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := newService("admin", "secret", func() time.Time { return now })
	id, csrf, expires, err := s.Login("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !expires.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("expiry = %v", expires)
	}
	now = now.Add(23 * time.Hour)
	if _, ok := s.Session(id); !ok {
		t.Fatal("active session rejected")
	}
	now = now.Add(2 * time.Hour)
	if _, ok := s.Session(id); ok {
		t.Fatal("expired session accepted")
	}
	if s.ValidateCSRF(id, csrf) {
		t.Fatal("expired session accepted CSRF token")
	}
}

func TestNewLoginPreservesOtherActiveSessions(t *testing.T) {
	s := New("admin", "secret")
	firstID, _, _, err := s.Login("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	secondID, _, _, err := s.Login("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Session(firstID); !ok {
		t.Fatal("new login revoked the first active session")
	}
	if _, ok := s.Session(secondID); !ok {
		t.Fatal("new session is not active")
	}
}

func TestLoginUsesConfiguredCredentialsAndLogoutRevokesSession(t *testing.T) {
	s := New("admin", "secret")
	for _, pair := range [][2]string{{"wrong", "secret"}, {"admin", "wrong"}} {
		if _, _, _, err := s.Login(pair[0], pair[1]); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("expected generic credentials error, got %v", err)
		}
	}
	id, csrf, _, err := s.Login("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !s.ValidateCSRF(id, csrf) {
		t.Fatal("valid CSRF token rejected")
	}
	s.Logout(id)
	if _, ok := s.Session(id); ok {
		t.Fatal("logged out session remains active")
	}
}

func TestNewLoginPreservesOtherUnexpiredSessions(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := newService("admin", "secret", func() time.Time { return now })
	firstID, _, _, err := s.Login("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	secondID, _, _, err := s.Login("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if firstID == secondID {
		t.Fatal("separate logins must receive distinct session IDs")
	}
	if _, ok := s.Session(firstID); !ok {
		t.Fatal("second login revoked an unexpired session")
	}
	if _, ok := s.Session(secondID); !ok {
		t.Fatal("new session is not active")
	}
}
