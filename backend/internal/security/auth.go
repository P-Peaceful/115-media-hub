package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

const SessionLifetime = 24 * time.Hour

var ErrInvalidCredentials = errors.New("invalid credentials")

type SessionInfo struct {
	CSRFToken string
	ExpiresAt time.Time
}

type session struct {
	csrfToken string
	expiresAt time.Time
}

type Service struct {
	username string
	password string
	now      func() time.Time
	mu       sync.Mutex
	sessions map[string]session
}

func New(username, password string) *Service {
	return newService(username, password, time.Now)
}

func newService(username, password string, now func() time.Time) *Service {
	return &Service{username: username, password: password, now: now, sessions: make(map[string]session)}
}

func (s *Service) Login(username, password string) (string, string, time.Time, error) {
	usernameHash := sha256.Sum256([]byte(username))
	configuredUsernameHash := sha256.Sum256([]byte(s.username))
	passwordHash := sha256.Sum256([]byte(password))
	configuredPasswordHash := sha256.Sum256([]byte(s.password))
	userOK := subtle.ConstantTimeCompare(usernameHash[:], configuredUsernameHash[:])
	passwordOK := subtle.ConstantTimeCompare(passwordHash[:], configuredPasswordHash[:])
	if userOK&passwordOK != 1 {
		return "", "", time.Time{}, ErrInvalidCredentials
	}
	id, err := randomToken()
	if err != nil {
		return "", "", time.Time{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return "", "", time.Time{}, err
	}
	expires := s.now().Add(SessionLifetime)
	s.mu.Lock()
	defer s.mu.Unlock()
	for sessionID, existing := range s.sessions {
		if !s.now().Before(existing.expiresAt) {
			delete(s.sessions, sessionID)
		}
	}
	s.sessions[id] = session{csrfToken: csrf, expiresAt: expires}
	return id, csrf, expires, nil
}

func (s *Service) Session(id string) (SessionInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[id]
	if !ok {
		return SessionInfo{}, false
	}
	if !s.now().Before(v.expiresAt) {
		delete(s.sessions, id)
		return SessionInfo{}, false
	}
	return SessionInfo{CSRFToken: v.csrfToken, ExpiresAt: v.expiresAt}, true
}

func (s *Service) ValidateCSRF(id, token string) bool {
	validSession, validToken := s.CheckCSRF(id, token)
	return validSession && validToken
}

func (s *Service) CheckCSRF(id, token string) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[id]
	if !ok {
		return false, false
	}
	if !s.now().Before(v.expiresAt) {
		delete(s.sessions, id)
		return false, false
	}
	expectedHash := sha256.Sum256([]byte(v.csrfToken))
	providedHash := sha256.Sum256([]byte(token))
	return true, subtle.ConstantTimeCompare(expectedHash[:], providedHash[:]) == 1
}

func (s *Service) Logout(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
