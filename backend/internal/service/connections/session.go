package connections

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/drive115"
)

var ErrSessionMissing = errors.New("QR session not found")

const sessionTTL = 5 * time.Minute
const maxSessions = 8

type qrSession struct {
	mu           sync.Mutex
	id           string
	device       drive115.Device
	challenge    drive115.Challenge
	status       drive115.Status
	expires      time.Time
	purgeAt      time.Time
	image        []byte
	completed    *Connection
	message      string
	upstreamCode string
}
type sessionManager struct {
	mu       sync.Mutex
	sessions map[string]*qrSession
	now      func() time.Time
}

func newSessionManager() *sessionManager {
	return &sessionManager{sessions: map[string]*qrSession{}, now: func() time.Time { return time.Now().UTC() }}
}
func randomID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (m *sessionManager) add(device drive115.Device, ch drive115.Challenge) (*qrSession, error) {
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
	if len(m.sessions) >= maxSessions {
		return nil, errors.New("QR session capacity reached")
	}
	now := m.now()
	s := &qrSession{id: id, device: device, challenge: ch, status: drive115.Waiting, expires: now.Add(sessionTTL), purgeAt: now.Add(3 * sessionTTL), image: append([]byte(nil), ch.Image...)}
	m.sessions[id] = s
	return s, nil
}
func (m *sessionManager) get(id string) (*qrSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
	s := m.sessions[id]
	if s == nil {
		return nil, ErrSessionMissing
	}
	return s, nil
}
func (m *sessionManager) pruneLocked() {
	for id, s := range m.sessions {
		if !m.now().Before(s.purgeAt) {
			delete(m.sessions, id)
		}
	}
}
func (m *sessionManager) removeChallenge(s *qrSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current := m.sessions[s.id]; current == s {
		current.challenge = drive115.Challenge{}
		current.image = nil
	}
}
