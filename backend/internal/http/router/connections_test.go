package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/drive115"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/database"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite/model"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/security"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/connections"
)

type qrImageTestAdapter struct {
	image []byte
}

func (a qrImageTestAdapter) StartQRCode(context.Context, drive115.Device) (drive115.Challenge, error) {
	return drive115.Challenge{Image: a.image, ExpiresAt: time.Now().Add(time.Minute)}, nil
}

func (qrImageTestAdapter) PollQRCode(context.Context, drive115.Challenge) (drive115.Status, error) {
	return "", &drive115.UpstreamError{Code: "40199002", Message: "115 扫码已过期，请重新获取二维码", Cause: drive115.ErrLimited}
}

func (qrImageTestAdapter) CompleteQRCode(context.Context, drive115.Challenge, drive115.Device) (drive115.Credential, error) {
	return drive115.Credential{}, errors.New("not used in this test")
}

func (qrImageTestAdapter) VerifyCredential(context.Context, drive115.Credential) (drive115.Evidence, error) {
	return drive115.Evidence{}, errors.New("not used in this test")
}

type profileTestAdapter struct{ profile drive115.AccountProfile }

func (profileTestAdapter) StartQRCode(context.Context, drive115.Device) (drive115.Challenge, error) {
	return drive115.Challenge{}, errors.New("not used in this test")
}
func (profileTestAdapter) PollQRCode(context.Context, drive115.Challenge) (drive115.Status, error) {
	return "", errors.New("not used in this test")
}
func (profileTestAdapter) CompleteQRCode(context.Context, drive115.Challenge, drive115.Device) (drive115.Credential, error) {
	return drive115.Credential{}, errors.New("not used in this test")
}
func (a profileTestAdapter) VerifyCredential(_ context.Context, credential drive115.Credential) (drive115.Evidence, error) {
	if credential.UID != "credential-sentinel" {
		return drive115.Evidence{}, errors.New("unexpected stored credential")
	}
	return drive115.Evidence{AccountID: "account-115", AccountName: a.profile.AccountName, RootReadable: true, Profile: a.profile}, nil
}

// Ensure the protected image route can serve the QR before status polling is
// allowed to invalidate the short-lived upstream challenge.
func TestQRCodeImageLoadsBeforePollingCanInvalidateSession(t *testing.T) {
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(filepath.Join(t.TempDir(), "connections.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	auth := security.New("admin", "secret")
	service := connections.New(db, qrImageTestAdapter{image: imageData.Bytes()})
	r := NewWithConnections(nil, nil, auth, false, service)

	login := httptest.NewRecorder()
	r.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"secret"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	var loginBody struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &loginBody); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/admin/connections/115/qr-sessions", bytes.NewBufferString(`{"device":"web"}`))
	createReq.AddCookie(cookie)
	createReq.Header.Set("X-CSRF-Token", loginBody.CSRFToken)
	r.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create QR session status = %d: %s", create.Code, create.Body.String())
	}
	var session struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}

	base := "/api/admin/connections/115/qr-sessions/" + session.SessionID
	qr := httptest.NewRecorder()
	qrReq := httptest.NewRequest(http.MethodGet, base+"/qr", nil)
	qrReq.AddCookie(cookie)
	r.ServeHTTP(qr, qrReq)
	if qr.Code != http.StatusOK || qr.Header().Get("Content-Type") != "image/png" || !bytes.Equal(qr.Body.Bytes(), imageData.Bytes()) {
		t.Fatalf("QR image response = %d, content-type %q, body length %d", qr.Code, qr.Header().Get("Content-Type"), qr.Body.Len())
	}

	// A failed upstream poll clears its challenge. This must happen only after
	// the browser has already loaded the image, as enforced by the page query gate.
	poll := httptest.NewRecorder()
	pollReq := httptest.NewRequest(http.MethodGet, base, nil)
	pollReq.AddCookie(cookie)
	r.ServeHTTP(poll, pollReq)
	if poll.Code != http.StatusOK {
		t.Fatalf("QR status poll = %d: %s", poll.Code, poll.Body.String())
	}
	if !bytes.Contains(poll.Body.Bytes(), []byte(`"message":"115 扫码已过期，请重新获取二维码"`)) || !bytes.Contains(poll.Body.Bytes(), []byte(`"upstreamCode":"40199002"`)) {
		t.Fatalf("QR poll did not expose the sanitized upstream details: %s", poll.Body.String())
	}
	for _, secretField := range [][]byte{[]byte(`"UID"`), []byte(`"CID"`), []byte(`"SEID"`), []byte(`"KID"`), []byte("Cookie:"), []byte("Set-Cookie"), []byte("sign=")} {
		if bytes.Contains(poll.Body.Bytes(), secretField) {
			t.Fatalf("QR status response leaked a credential or challenge field %q: %s", secretField, poll.Body.String())
		}
	}
	missingImage := httptest.NewRecorder()
	missingImageReq := httptest.NewRequest(http.MethodGet, base+"/qr", nil)
	missingImageReq.AddCookie(cookie)
	r.ServeHTTP(missingImage, missingImageReq)
	if missingImage.Code != http.StatusNotFound || !bytes.Contains(missingImage.Body.Bytes(), []byte(`"message":"QR session not found"`)) {
		t.Fatalf("image after invalidated session = %d: %s", missingImage.Code, missingImage.Body.String())
	}
}

func TestRefreshAccountProfilePersistsOnlySafeSnapshot(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "profile.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	const credentialJSON = `{"UID":"credential-sentinel","CID":"cookie-cid","SEID":"cookie-seid","KID":"cookie-kid"}`
	row := model.Connection{
		ID: "connection-115", Kind: "115", Name: "115 account", ExternalAccountID: "account-115",
		CredentialJSON: credentialJSON, CapabilitiesJSON: "[]", IsActive: true, Status: "available",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}

	profile := drive115.AccountProfile{
		AccountName: "阿文", MembershipStatus: "active", MembershipName: "上游会员名",
		StorageTotalBytes: int64Pointer(1_000_000), StorageUsedBytes: int64Pointer(250_000),
		StorageFreeBytes: int64Pointer(750_000), RetrievedAt: time.Now().UTC(),
	}
	service := connections.New(db, profileTestAdapter{profile: profile})
	r := NewWithConnections(nil, nil, security.New("admin", "secret"), false, service)
	cookie, csrf := loginForConnectionsTest(t, r)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/connections/115/profile/refresh", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", csrf)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("profile refresh status = %d: %s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"accountName":"阿文"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"storageTotalBytes":1000000`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"membershipName":"上游会员名"`)) {
		t.Fatalf("profile response does not include the safe account snapshot: %s", response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("credential-sentinel")) || bytes.Contains(response.Body.Bytes(), []byte(`"UID"`)) || bytes.Contains(response.Body.Bytes(), []byte(`"membershipLevel"`)) {
		t.Fatalf("profile response leaked credential material: %s", response.Body.String())
	}

	var saved model.Connection
	if err := db.Where("id = ?", row.ID).Take(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.CredentialJSON != credentialJSON || saved.Status != "available" {
		t.Fatalf("profile refresh changed credential or connection status: status=%q", saved.Status)
	}
	if !bytes.Contains([]byte(saved.ProfileJSON), []byte(`"schema_version":1`)) {
		t.Fatalf("profile snapshot was not stored with a schema version: %s", saved.ProfileJSON)
	}
}

func TestConnectionSummaryReturnsProfileWithoutStoredCredential(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "summary.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	profileJSON, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"profile":        drive115.AccountProfile{AccountName: "展示账号", MembershipStatus: "active", MembershipName: "上游会员名"},
	})
	if err != nil {
		t.Fatal(err)
	}
	row := model.Connection{
		ID: "summary-connection", Kind: "115", Name: "展示账号", ExternalAccountID: "115-uid-123",
		CredentialJSON:   `{"UID":"credential-json-sentinel","CID":"cookie-cid-sentinel","SEID":"cookie-seid-sentinel","KID":"cookie-kid-sentinel"}`,
		CapabilitiesJSON: `[]`, ProfileJSON: string(profileJSON), IsActive: true, Status: "available",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	r := NewWithConnections(nil, nil, security.New("admin", "secret"), false, connections.New(db, &lifecycleTestAdapter{}))
	cookie, _ := loginForConnectionsTest(t, r)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/connections/115", nil)
	req.AddCookie(cookie)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("connection summary status = %d: %s", w.Code, w.Body.String())
	}
	for _, expected := range []string{`"name":"展示账号"`, `"externalAccountId":"115-uid-123"`, `"accountName":"展示账号"`, `"membershipStatus":"active"`, `"membershipName":"上游会员名"`} {
		if !bytes.Contains(w.Body.Bytes(), []byte(expected)) {
			t.Errorf("summary omitted %s: %s", expected, w.Body.String())
		}
	}
	for _, forbidden := range []string{"credential-json-sentinel", "cookie-cid-sentinel", "cookie-seid-sentinel", "cookie-kid-sentinel", `"credentialJson"`, `"UID"`, `"CID"`, `"SEID"`, `"KID"`} {
		if bytes.Contains(w.Body.Bytes(), []byte(forbidden)) {
			t.Errorf("summary exposed stored credential field %q: %s", forbidden, w.Body.String())
		}
	}
}

func loginForConnectionsTest(t *testing.T, r http.Handler) (*http.Cookie, string) {
	t.Helper()
	login := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"secret"}`))
	r.ServeHTTP(login, req)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", login.Code, login.Body.String())
	}
	var body struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return login.Result().Cookies()[0], body.CSRFToken
}

func int64Pointer(value int64) *int64 { return &value }

type lifecycleTestAdapter struct {
	startCalls, pollCalls, completeCalls, verifyCalls int
	status                                            drive115.Status
	credential                                        drive115.Credential
	evidence                                          drive115.Evidence
	pollErr                                           error
}

func (a *lifecycleTestAdapter) StartQRCode(context.Context, drive115.Device) (drive115.Challenge, error) {
	a.startCalls++
	return drive115.Challenge{Image: []byte("png"), ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (a *lifecycleTestAdapter) PollQRCode(context.Context, drive115.Challenge) (drive115.Status, error) {
	a.pollCalls++
	return a.status, a.pollErr
}
func (a *lifecycleTestAdapter) CompleteQRCode(context.Context, drive115.Challenge, drive115.Device) (drive115.Credential, error) {
	a.completeCalls++
	return a.credential, nil
}
func (a *lifecycleTestAdapter) VerifyCredential(context.Context, drive115.Credential) (drive115.Evidence, error) {
	a.verifyCalls++
	return a.evidence, nil
}

func TestConnectionRoutesRequireAdminSessionAndCSRF(t *testing.T) {
	driver := &lifecycleTestAdapter{}
	r := NewWithConnections(nil, nil, security.New("admin", "secret"), false, connections.New(nil, driver))

	for _, target := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/connections/115"},
		{http.MethodPost, "/api/admin/connections/115/qr-sessions"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(target.method, target.path, bytes.NewBufferString(`{"device":"web"}`)))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without admin session = %d, want 401", target.method, target.path, w.Code)
		}
	}
	cookie, _ := loginForConnectionsTest(t, r)
	mutations := []struct{ path, body string }{
		{"/api/admin/connections/115/qr-sessions", `{"device":"web"}`},
		{"/api/admin/connections/115/test", ""},
		{"/api/admin/connections/115/profile/refresh", ""},
	}
	for _, target := range mutations {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, target.path, bytes.NewBufferString(target.body))
		req.AddCookie(cookie)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("POST %s without CSRF = %d, want 403", target.path, w.Code)
		}
	}
	if driver.startCalls != 0 {
		t.Fatalf("unauthorized or CSRF-rejected requests reached 115 adapter %d times", driver.startCalls)
	}
}

func TestInvalidQRCodeDeviceIsRejectedBeforeUpstreamCall(t *testing.T) {
	driver := &lifecycleTestAdapter{}
	r := NewWithConnections(nil, nil, security.New("admin", "secret"), false, connections.New(nil, driver))
	cookie, csrf := loginForConnectionsTest(t, r)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/connections/115/qr-sessions", bytes.NewBufferString(`{"device":"unsupported"}`))
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", csrf)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || driver.startCalls != 0 {
		t.Fatalf("invalid device response = %d (%s), upstream start calls=%d", w.Code, w.Body.String(), driver.startCalls)
	}
}

func TestQRCodeCompletionConflictPreservesExistingConnection(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "conflict.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	old := model.Connection{ID: "existing", Kind: "115", Name: "old", ExternalAccountID: "account-old", CredentialJSON: `{"UID":"old-secret"}`, CapabilitiesJSON: `[]`, IsActive: true, Status: "available", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	driver := &lifecycleTestAdapter{status: drive115.Allowed, credential: drive115.Credential{UID: "new-secret"}, evidence: drive115.Evidence{AccountID: "account-new", AccountName: "new", RootReadable: true}}
	r := NewWithConnections(nil, nil, security.New("admin", "secret"), false, connections.New(db, driver))
	cookie, csrf := loginForConnectionsTest(t, r)
	base, sessionID := startAndAllowConnectionSession(t, r, driver, cookie, csrf)
	complete := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, base+"/complete", nil)
		req.AddCookie(cookie)
		req.Header.Set("X-CSRF-Token", csrf)
		r.ServeHTTP(w, req)
		return w
	}
	first := complete()
	if first.Code != http.StatusConflict {
		t.Fatalf("account mismatch completion = %d: %s", first.Code, first.Body.String())
	}
	second := complete()
	if second.Code != http.StatusUnprocessableEntity || driver.completeCalls != 1 {
		t.Fatalf("repeating failed completion for session %q = %d after %d upstream completions: %s", sessionID, second.Code, driver.completeCalls, second.Body.String())
	}
	var saved model.Connection
	if err := db.Where("id = ?", old.ID).Take(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.ID != old.ID || saved.ExternalAccountID != old.ExternalAccountID || saved.CredentialJSON != old.CredentialJSON || saved.Status != old.Status || saved.CapabilitiesJSON != old.CapabilitiesJSON {
		t.Fatalf("conflicting reauthorization changed existing connection: before=%+v after=%+v", old, saved)
	}
}

func TestQRCodeCompletionIsIdempotentAfterSuccess(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "complete.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	driver := &lifecycleTestAdapter{status: drive115.Allowed, credential: drive115.Credential{UID: "secret"}, evidence: drive115.Evidence{AccountID: "account", AccountName: "account", RootReadable: true}}
	r := NewWithConnections(nil, nil, security.New("admin", "secret"), false, connections.New(db, driver))
	cookie, csrf := loginForConnectionsTest(t, r)
	base, _ := startAndAllowConnectionSession(t, r, driver, cookie, csrf)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, base+"/complete", nil)
		req.AddCookie(cookie)
		req.Header.Set("X-CSRF-Token", csrf)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("completion attempt %d = %d: %s", i+1, w.Code, w.Body.String())
		}
		if bytes.Contains(w.Body.Bytes(), []byte("secret")) || bytes.Contains(w.Body.Bytes(), []byte(`"UID"`)) {
			t.Fatalf("completion response exposed credentials: %s", w.Body.String())
		}
	}
	if driver.completeCalls != 1 || driver.verifyCalls != 1 {
		t.Fatalf("repeated complete called upstream complete=%d verify=%d times", driver.completeCalls, driver.verifyCalls)
	}
}

func startAndAllowConnectionSession(t *testing.T, r http.Handler, driver *lifecycleTestAdapter, cookie *http.Cookie, csrf string) (base, sessionID string) {
	t.Helper()
	start := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/connections/115/qr-sessions", bytes.NewBufferString(`{"device":"web"}`))
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", csrf)
	r.ServeHTTP(start, req)
	if start.Code != http.StatusCreated {
		t.Fatalf("start QR = %d: %s", start.Code, start.Body.String())
	}
	var session struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	base = "/api/admin/connections/115/qr-sessions/" + session.SessionID
	poll := httptest.NewRecorder()
	pollReq := httptest.NewRequest(http.MethodGet, base, nil)
	pollReq.AddCookie(cookie)
	r.ServeHTTP(poll, pollReq)
	if poll.Code != http.StatusOK || !bytes.Contains(poll.Body.Bytes(), []byte(`"status":"allowed"`)) {
		t.Fatalf("allow QR session = %d: %s", poll.Code, poll.Body.String())
	}
	return base, session.SessionID
}
