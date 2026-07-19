package http

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/auth"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const sessionCookieName = "mga_session"

type loginAttempt struct {
	count      int
	windowEnds time.Time
}

type AuthController struct {
	service  *auth.Service
	profiles core.ProfileRepository
	logger   core.Logger
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

type authStatusResponse struct {
	Authenticated bool          `json:"authenticated"`
	Profile       *core.Profile `json:"profile,omitempty"`
	MustChange    bool          `json:"must_change"`
}

func NewAuthController(service *auth.Service, profiles core.ProfileRepository, logger core.Logger) (*AuthController, error) {
	if service == nil {
		return nil, errors.New("auth service is required")
	}
	if profiles == nil {
		return nil, errors.New("profile repository is required")
	}
	if logger == nil {
		return nil, errors.New("logger is required")
	}
	return &AuthController{service: service, profiles: profiles, logger: logger, attempts: map[string]loginAttempt{}}, nil
}

func (c *AuthController) Session(w http.ResponseWriter, r *http.Request) {
	token := sessionToken(r)
	session, err := c.service.Authenticate(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusOK, authStatusResponse{})
		return
	}
	profile, err := c.profiles.GetByID(r.Context(), session.ProfileID)
	if err != nil || profile == nil {
		writeJSON(w, http.StatusOK, authStatusResponse{})
		return
	}
	writeJSON(w, http.StatusOK, authStatusResponse{Authenticated: true, Profile: profile, MustChange: session.MustChange})
}

func (c *AuthController) CredentialStatus(w http.ResponseWriter, r *http.Request) {
	status, err := c.service.CredentialStatus(r.Context(), core.ProfileIDFromContext(r.Context()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (c *AuthController) InitializeCredential(w http.ResponseWriter, r *http.Request) {
	var body struct {
		New  string              `json:"new"`
		Kind auth.CredentialKind `json:"kind"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	profileID := core.ProfileIDFromContext(r.Context())
	if err := c.service.InitializeCredential(r.Context(), profileID, body.New, body.Kind); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, auth.ErrCredentialConfigured) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	credentialStatus, err := c.service.CredentialStatus(r.Context(), profileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, credentialStatus)
}

func (c *AuthController) RemoveCredential(w http.ResponseWriter, r *http.Request) {
	session, err := c.service.Authenticate(r.Context(), sessionToken(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if err := c.service.RemoveOwnCredential(r.Context(), session, core.ProfileIDFromContext(r.Context())); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProfileID  string `json:"profile_id"`
		Credential string `json:"credential"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	key := remoteIP(r) + "|" + strings.TrimSpace(body.ProfileID)
	if !c.allowLoginAttempt(key, time.Now()) {
		http.Error(w, "too many login attempts; try again later", http.StatusTooManyRequests)
		return
	}
	token, session, err := c.service.Login(r.Context(), body.ProfileID, body.Credential)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, auth.ErrCredentialRequired) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	c.clearLoginAttempts(key)
	setSessionCookie(w, r, token, session.ExpiresAt)
	profile, err := c.profiles.GetByID(r.Context(), session.ProfileID)
	if err != nil || profile == nil {
		http.Error(w, "profile not found after login", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, authStatusResponse{Authenticated: true, Profile: profile, MustChange: session.MustChange})
}

func (c *AuthController) ChangeCredential(w http.ResponseWriter, r *http.Request) {
	session, err := c.service.Authenticate(r.Context(), sessionToken(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var body struct {
		Current string              `json:"current"`
		New     string              `json:"new"`
		Kind    auth.CredentialKind `json:"kind"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	token, nextSession, err := c.service.ChangeOwnCredential(r.Context(), session, body.Current, body.New, body.Kind)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, auth.ErrInvalidCredential) {
			status = http.StatusUnauthorized
		}
		http.Error(w, err.Error(), status)
		return
	}
	setSessionCookie(w, r, token, nextSession.ExpiresAt)
	profile, err := c.profiles.GetByID(r.Context(), nextSession.ProfileID)
	if err != nil || profile == nil {
		http.Error(w, "profile not found after credential change", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, authStatusResponse{Authenticated: true, Profile: profile})
}

func (c *AuthController) Logout(w http.ResponseWriter, r *http.Request) {
	if err := c.service.Logout(r.Context(), sessionToken(r)); err != nil {
		c.logger.Error("delete auth session", err)
		http.Error(w, "logout failed", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

func RequireDeviceSession(service *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := service.Authenticate(r.Context(), sessionToken(r))
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
			if err := service.RequireDeviceAuthority(session, core.ProfileIDFromContext(r.Context())); err != nil {
				status := http.StatusForbidden
				if errors.Is(err, auth.ErrUnauthenticated) {
					status = http.StatusUnauthorized
				}
				http.Error(w, err.Error(), status)
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithSession(r.Context(), session)))
		})
	}
}

// RequireProfileAccess enforces the common profile boundary after
// ProfileContextMiddleware selected and validated the profile. Unprotected
// profiles remain available on the trusted LAN; protected profiles require an
// exact, non-must-change session.
func RequireProfileAccess(service *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if service == nil {
				http.Error(w, "profile access service is unavailable", http.StatusInternalServerError)
				return
			}
			profileID := core.ProfileIDFromContext(r.Context())
			session, err := service.AuthorizeProfileAccess(r.Context(), profileID, sessionToken(r))
			if err != nil {
				status := http.StatusUnauthorized
				if errors.Is(err, auth.ErrForbidden) || errors.Is(err, auth.ErrCredentialChange) {
					status = http.StatusForbidden
				} else if !errors.Is(err, auth.ErrUnauthenticated) && !errors.Is(err, auth.ErrProfileNotFound) {
					status = http.StatusInternalServerError
				}
				http.Error(w, err.Error(), status)
				return
			}
			ctx := auth.WithProfileAccess(r.Context(), auth.ProfileAccess{ProfileID: profileID, Session: session})
			if session != nil {
				ctx = auth.WithSession(ctx, session)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
	})
}

func isLoopbackRequest(r *http.Request) bool {
	ip := net.ParseIP(remoteIP(r))
	return ip != nil && ip.IsLoopback()
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (c *AuthController) allowLoginAttempt(key string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	attempt := c.attempts[key]
	if attempt.windowEnds.IsZero() || !now.Before(attempt.windowEnds) {
		attempt = loginAttempt{windowEnds: now.Add(5 * time.Minute)}
	}
	if attempt.count >= 5 {
		c.attempts[key] = attempt
		return false
	}
	attempt.count++
	c.attempts[key] = attempt
	return true
}

func (c *AuthController) clearLoginAttempts(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.attempts, key)
}
