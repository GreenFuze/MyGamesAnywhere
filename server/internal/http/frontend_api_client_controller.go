package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/frontendauth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type FrontendAPIClientService interface {
	Create(context.Context, string, string, []frontendauth.Scope, *time.Time) (*frontendauth.IssuedClient, error)
	List(context.Context, string) ([]frontendauth.Client, error)
	Rotate(context.Context, string, string) (*frontendauth.IssuedClient, error)
	Revoke(context.Context, string, string) (*frontendauth.Client, error)
	Authenticate(context.Context, string, ...frontendauth.Scope) (frontendauth.Principal, error)
}

type FrontendAPIClientController struct {
	service FrontendAPIClientService
	logger  core.Logger
}

func NewFrontendAPIClientController(service FrontendAPIClientService, logger core.Logger) (*FrontendAPIClientController, error) {
	if service == nil || logger == nil {
		return nil, errors.New("frontend API client service and logger are required")
	}
	return &FrontendAPIClientController{service: service, logger: logger}, nil
}

type createFrontendAPIClientRequest struct {
	Name      string               `json:"name"`
	Scopes    []frontendauth.Scope `json:"scopes"`
	ExpiresAt *time.Time           `json:"expires_at,omitempty"`
}

func (c *FrontendAPIClientController) Create(w http.ResponseWriter, r *http.Request) {
	var request createFrontendAPIClientRequest
	if err := decodeSingleJSON(w, r, &request); err != nil {
		writeContentError(w, http.StatusBadRequest, "invalid_request", "request must be one valid JSON object")
		return
	}
	issued, err := c.service.Create(frontendAuditContext(r), core.ProfileIDFromContext(r.Context()), request.Name, request.Scopes, request.ExpiresAt)
	if err != nil {
		c.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, issued)
	c.logger.Info("frontend API client audit", "action", "create", "outcome", "success", "profile_id", issued.ProfileID, "client_id", issued.ID)
}

func (c *FrontendAPIClientController) List(w http.ResponseWriter, r *http.Request) {
	clients, err := c.service.List(r.Context(), core.ProfileIDFromContext(r.Context()))
	if err != nil {
		c.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"clients":           clients,
		"supported_scopes":  frontendauth.AllScopes(),
		"transport_warning": frontendauth.TransportWarning,
	})
}

func (c *FrontendAPIClientController) Rotate(w http.ResponseWriter, r *http.Request) {
	issued, err := c.service.Rotate(frontendAuditContext(r), core.ProfileIDFromContext(r.Context()), chi.URLParam(r, "client_id"))
	if err != nil {
		c.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issued)
	c.logger.Info("frontend API client audit", "action", "rotate", "outcome", "success", "profile_id", issued.ProfileID, "client_id", issued.ID)
}

func (c *FrontendAPIClientController) Revoke(w http.ResponseWriter, r *http.Request) {
	client, err := c.service.Revoke(frontendAuditContext(r), core.ProfileIDFromContext(r.Context()), chi.URLParam(r, "client_id"))
	if err != nil {
		c.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, client)
	c.logger.Info("frontend API client audit", "action", "revoke", "outcome", "success", "profile_id", client.ProfileID, "client_id", client.ID)
}

func (c *FrontendAPIClientController) Capabilities(w http.ResponseWriter, r *http.Request) {
	principal, ok := frontendauth.PrincipalFromContext(r.Context())
	if !ok {
		writeContentError(w, http.StatusUnauthorized, "unauthenticated", frontendauth.ErrUnauthenticated.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"api":              map[string]string{"name": "MGA Frontend API", "version": "v1"},
		"client":           principal,
		"supported_scopes": frontendauth.AllScopes(),
		"features":         []string{"capability-discovery", "catalog-projection", "metadata-media", "content-delivery", "cache-preparation", "runtime-artifacts"},
		"endpoints":        map[string]string{"capabilities": "/api/frontend/v1/capabilities"},
	})
}

func (c *FrontendAPIClientController) writeError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusInternalServerError, "internal_error", "frontend API client request failed"
	switch {
	case errors.Is(err, frontendauth.ErrInvalid):
		status, code, message = http.StatusBadRequest, "invalid_client", "name, future expiry, and at least one supported scope are required"
	case errors.Is(err, frontendauth.ErrNotFound), errors.Is(err, frontendauth.ErrProfileMismatch):
		status, code, message = http.StatusNotFound, "not_found", "frontend API client not found"
	default:
		c.logger.Error("frontend API client request failed", err)
	}
	writeContentError(w, status, code, message)
}

func RequireFrontendAPIClient(service FrontendAPIClientService, profiles core.ProfileRepository, required ...frontendauth.Scope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if service == nil || profiles == nil {
				http.Error(w, "frontend API authentication is unavailable", http.StatusInternalServerError)
				return
			}
			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				writeContentError(w, http.StatusUnauthorized, "unauthenticated", frontendauth.ErrUnauthenticated.Error())
				return
			}
			auditContext := frontendAuditContext(r)
			principal, err := service.Authenticate(auditContext, token, required...)
			if err != nil {
				status, code := http.StatusUnauthorized, "unauthenticated"
				if errors.Is(err, frontendauth.ErrForbidden) {
					status, code = http.StatusForbidden, "insufficient_scope"
				}
				writeContentError(w, status, code, "frontend API client authorization failed")
				return
			}
			requestedProfile := strings.TrimSpace(r.Header.Get(profileHeader))
			if requestedProfile == "" {
				requestedProfile = strings.TrimSpace(r.URL.Query().Get("profile_id"))
			}
			if requestedProfile != "" && requestedProfile != principal.ProfileID {
				writeContentError(w, http.StatusForbidden, "profile_mismatch", "frontend API client authorization failed")
				return
			}
			profile, err := profiles.GetByID(r.Context(), principal.ProfileID)
			if err != nil {
				http.Error(w, "profile lookup failed", http.StatusInternalServerError)
				return
			}
			if profile == nil {
				writeContentError(w, http.StatusUnauthorized, "unauthenticated", "frontend API client authorization failed")
				return
			}
			ctx := frontendauth.WithPrincipal(core.WithProfile(auditContext, profile), principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	return func() (string, bool) {
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			return "", false
		}
		return parts[1], true
	}()
}

func decodeSingleJSON(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("body must contain exactly one JSON value")
	}
	return nil
}

func requestRemoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func frontendAuditContext(r *http.Request) context.Context {
	return frontendauth.WithAuditMetadata(r.Context(), frontendauth.AuditMetadata{
		RequestID: middleware.GetReqID(r.Context()),
		RemoteIP:  requestRemoteIP(r),
	})
}
