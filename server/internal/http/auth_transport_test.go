package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/auth"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/coder/websocket"
)

type lanAuthStore struct {
	credentials map[string]auth.Credential
	sessions    map[string]auth.Session
}

func newLANAuthStore() *lanAuthStore {
	return &lanAuthStore{
		credentials: make(map[string]auth.Credential),
		sessions:    make(map[string]auth.Session),
	}
}

func (s *lanAuthStore) GetCredential(_ context.Context, profileID string) (*auth.Credential, error) {
	credential, ok := s.credentials[profileID]
	if !ok {
		return nil, nil
	}
	return &credential, nil
}

func (s *lanAuthStore) CreateCredential(_ context.Context, credential auth.Credential) error {
	s.credentials[credential.ProfileID] = credential
	return nil
}

func (s *lanAuthStore) SetCredential(_ context.Context, credential auth.Credential) error {
	s.credentials[credential.ProfileID] = credential
	return nil
}

func (s *lanAuthStore) DeleteCredential(_ context.Context, profileID string) error {
	delete(s.credentials, profileID)
	return nil
}

func (s *lanAuthStore) CreateSession(_ context.Context, session auth.Session, tokenHash string) error {
	s.sessions[tokenHash] = session
	return nil
}

func (s *lanAuthStore) GetSessionByTokenHash(_ context.Context, tokenHash string) (*auth.Session, error) {
	session, ok := s.sessions[tokenHash]
	if !ok {
		return nil, nil
	}
	return &session, nil
}

func (s *lanAuthStore) DeleteSessionByTokenHash(_ context.Context, tokenHash string) error {
	delete(s.sessions, tokenHash)
	return nil
}

func (s *lanAuthStore) DeleteSessionsByProfile(_ context.Context, profileID string) error {
	for tokenHash, session := range s.sessions {
		if session.ProfileID == profileID {
			delete(s.sessions, tokenHash)
		}
	}
	return nil
}

func (s *lanAuthStore) DeleteExpiredSessions(_ context.Context, now time.Time) error {
	for tokenHash, session := range s.sessions {
		if !now.Before(session.ExpiresAt) {
			delete(s.sessions, tokenHash)
		}
	}
	return nil
}

type lanProfileRepository struct {
	profile *core.Profile
}

func (r lanProfileRepository) Create(context.Context, *core.Profile) error { return nil }
func (r lanProfileRepository) Update(context.Context, *core.Profile) error { return nil }
func (r lanProfileRepository) Delete(context.Context, string) error        { return nil }
func (r lanProfileRepository) List(context.Context) ([]*core.Profile, error) {
	return []*core.Profile{r.profile}, nil
}
func (r lanProfileRepository) GetByID(_ context.Context, id string) (*core.Profile, error) {
	if r.profile != nil && r.profile.ID == id {
		return r.profile, nil
	}
	return nil, nil
}
func (r lanProfileRepository) Count(context.Context) (int, error)       { return 1, nil }
func (r lanProfileRepository) CountAdmins(context.Context) (int, error) { return 1, nil }
func (r lanProfileRepository) EnsureDefaultForExistingData(context.Context) (*core.Profile, error) {
	return r.profile, nil
}

func TestProfileLoginAndDeviceSessionAllowHTTPFromLAN(t *testing.T) {
	profile := &core.Profile{ID: "admin-1", DisplayName: "Admin", Role: core.ProfileRoleAdminPlayer}
	store := newLANAuthStore()
	service, err := auth.NewService(store, lanProfileRepository{profile: profile})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := service.InitializeCredential(context.Background(), profile.ID, "mga-e2e-2026", auth.CredentialPassword); err != nil {
		t.Fatalf("InitializeCredential() error = %v", err)
	}
	controller, err := NewAuthController(service, lanProfileRepository{profile: profile}, noopLogger{})
	if err != nil {
		t.Fatalf("NewAuthController() error = %v", err)
	}

	loginRequest := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"profile_id":"admin-1","credential":"mga-e2e-2026"}`))
	loginRequest.RemoteAddr = "192.168.68.20:53000"
	loginRecorder := httptest.NewRecorder()
	controller.Login(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("Login() status = %d, body = %q", loginRecorder.Code, loginRecorder.Body.String())
	}
	cookies := loginRecorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Login() cookies = %d, want 1", len(cookies))
	}
	if cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("Login() cookie security attributes = %+v", cookies[0])
	}

	deviceRequest := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	deviceRequest.RemoteAddr = "192.168.68.20:53001"
	deviceRequest.AddCookie(cookies[0])
	deviceRequest = deviceRequest.WithContext(core.WithProfile(deviceRequest.Context(), profile))
	deviceRecorder := httptest.NewRecorder()
	called := false
	handler := RequireDeviceSession(service)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	handler.ServeHTTP(deviceRecorder, deviceRequest)
	if deviceRecorder.Code != http.StatusNoContent || !called {
		t.Fatalf("RequireDeviceSession() status = %d, called = %v, body = %q", deviceRecorder.Code, called, deviceRecorder.Body.String())
	}
}

func TestDeviceClientTransportsAllowHTTPFromLAN(t *testing.T) {
	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		method  string
		path    string
	}{
		{name: "launch acknowledgement", handler: (&DeviceController{}).RedeemClientLaunch, method: http.MethodPost, path: "/api/devices/client-launch/redeem"},
		{name: "pairing", handler: (&DeviceController{}).Pair, method: http.MethodPost, path: "/api/devices/pair"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader("{"))
			request.RemoteAddr = "192.168.68.21:54000"
			recorder := httptest.NewRecorder()
			test.handler(recorder, request)
			if recorder.Code == http.StatusUpgradeRequired {
				t.Fatalf("status = %d, trusted-LAN HTTP must not require TLS", recorder.Code)
			}
		})
	}

	server := httptest.NewServer(http.HandlerFunc((&DeviceController{}).Connect))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, strings.Replace(server.URL, "http://", "ws://", 1), nil)
	if err != nil {
		t.Fatalf("trusted-LAN WS connection error = %v", err)
	}
	_ = connection.Close(websocket.StatusNormalClosure, "test complete")
}
