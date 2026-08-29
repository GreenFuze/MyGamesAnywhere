package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/frontendauth"
)

type fakeFrontendAPIClientService struct {
	principal frontendauth.Principal
	clients   []frontendauth.Client
	err       error
	token     string
}

func (s *fakeFrontendAPIClientService) Create(context.Context, string, string, []frontendauth.Scope, *time.Time) (*frontendauth.IssuedClient, error) {
	return nil, s.err
}
func (s *fakeFrontendAPIClientService) List(context.Context, string) ([]frontendauth.Client, error) {
	return s.clients, s.err
}
func (s *fakeFrontendAPIClientService) Rotate(context.Context, string, string) (*frontendauth.IssuedClient, error) {
	return nil, s.err
}
func (s *fakeFrontendAPIClientService) Revoke(context.Context, string, string) (*frontendauth.Client, error) {
	return nil, s.err
}
func (s *fakeFrontendAPIClientService) Authenticate(_ context.Context, token string, _ ...frontendauth.Scope) (frontendauth.Principal, error) {
	s.token = token
	return s.principal, s.err
}

func TestRequireFrontendAPIClientDerivesProfileAndRejectsCrossProfile(t *testing.T) {
	profile := &core.Profile{ID: "profile-1", DisplayName: "Admin", Role: core.ProfileRoleAdminPlayer}
	service := &fakeFrontendAPIClientService{principal: frontendauth.Principal{ClientID: "client-1", ProfileID: profile.ID, Scopes: []frontendauth.Scope{frontendauth.ScopeCatalogRead}}}
	repository := lanProfileRepository{profile: profile}
	handler := RequireFrontendAPIClient(service, repository, frontendauth.ScopeCatalogRead)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := frontendauth.PrincipalFromContext(r.Context())
		if !ok || principal.ClientID != "client-1" || core.ProfileIDFromContext(r.Context()) != profile.ID {
			t.Fatal("frontend principal or profile missing from context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/frontend/v1/capabilities", nil)
	request.Header.Set("Authorization", "Bearer one-time-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent || service.token != "one-time-token" {
		t.Fatalf("response = %d, token = %q", recorder.Code, service.token)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/frontend/v1/capabilities?profile_id=profile-2", nil)
	request.Header.Set("Authorization", "Bearer one-time-token")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-profile response = %d", recorder.Code)
	}
}

func TestRequireFrontendAPIClientFailsClosedAndRedactsErrors(t *testing.T) {
	profile := &core.Profile{ID: "profile-1", Role: core.ProfileRoleAdminPlayer}
	service := &fakeFrontendAPIClientService{err: frontendauth.ErrForbidden}
	handler := RequireFrontendAPIClient(service, lanProfileRepository{profile: profile}, frontendauth.ScopeManagement)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler should not run") }))
	request := httptest.NewRequest(http.MethodGet, "/api/frontend/v1/capabilities", nil)
	request.Header.Set("Authorization", "Bearer secret-that-must-not-leak")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || strings.Contains(recorder.Body.String(), "secret-that-must-not-leak") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}

	service.err = errors.New("database detail")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized || strings.Contains(recorder.Body.String(), "database detail") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestFrontendAPIClientListNeverSerializesSecretHash(t *testing.T) {
	service := &fakeFrontendAPIClientService{clients: []frontendauth.Client{{ID: "client-1", ProfileID: "profile-1", Name: "Playnite", SecretHash: "must-never-appear", Scopes: []frontendauth.Scope{frontendauth.ScopeCatalogRead}}}}
	controller, err := NewFrontendAPIClientController(service, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	profile := &core.Profile{ID: "profile-1", Role: core.ProfileRoleAdminPlayer}
	request := httptest.NewRequest(http.MethodGet, "/api/frontend-clients", nil).WithContext(core.WithProfile(context.Background(), profile))
	recorder := httptest.NewRecorder()
	controller.List(recorder, request)
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "must-never-appear") || strings.Contains(recorder.Body.String(), "secret_hash") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}
