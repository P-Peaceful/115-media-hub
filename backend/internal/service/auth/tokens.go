package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

func randomToken() string {
	var value [32]byte
	_, _ = rand.Read(value[:]) // Go 1.26 guarantees a full buffer and a nil error.
	return hex.EncodeToString(value[:])
}

func tokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func csrfToken(value string) string { return tokenHash("mediahub-csrf:" + value) }
func tokenEqual(a, b string) bool   { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func validSessionToken(token string) bool {
	return len(token) == 64 && !strings.ContainsAny(token, ". \t\r\n")
}

// AnonymousBinding issues a short-lived, restart-local cookie and its CSRF proof.
func (s *Service) AnonymousBinding() (cookie, csrf string) {
	payload := randomToken() + "." + strconv.FormatInt(s.now().Unix(), 10)
	cookie = payload + "." + s.signAnonymous(payload)
	return cookie, csrfToken(cookie)
}

func (s *Service) signAnonymous(value string) string {
	mac := hmac.New(sha256.New, s.anonymousKey)
	_, _ = mac.Write([]byte(value)) // hash.Hash.Write cannot fail.
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidateAnonymous requires the signed cookie and the corresponding in-memory CSRF token.
func (s *Service) ValidateAnonymous(cookie, csrf string) bool {
	if len(cookie) > 180 {
		return false
	}
	parts := strings.Split(cookie, ".")
	if len(parts) != 3 || len(parts[0]) != 64 {
		return false
	}
	issued, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}
	now := s.now()
	created := time.Unix(issued, 0)
	if created.After(now) || !now.Before(created.Add(20*time.Minute)) {
		return false
	}
	return tokenEqual(parts[2], s.signAnonymous(parts[0]+"."+parts[1])) && tokenEqual(csrf, csrfToken(cookie))
}

// AnonymousCSRF reuses a still-valid bootstrap cookie so refetches and multiple tabs share one proof.
func (s *Service) AnonymousCSRF(cookie string) (string, bool) {
	proof := csrfToken(cookie)
	if !s.ValidateAnonymous(cookie, proof) {
		return "", false
	}
	return proof, true
}
