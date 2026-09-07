package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/auth"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/frontendauth"
)

// How long a key issued this way lasts unless the caller asks for something
// shorter. Long enough that a living-room machine is not asked to sign in every
// week, short enough that a forgotten one eventually stops working.
const frontendSignInDefaultLifetime = 365 * 24 * time.Hour

type frontendSignInRequest struct {
	ProfileID  string   `json:"profile_id"`
	Credential string   `json:"credential"`
	Name       string   `json:"name"`
	Scopes     []string `json:"scopes,omitempty"`
}

// SignInFrontendClient exchanges a profile's own password for a scoped API key.
//
// Before this, connecting an external frontend meant opening the console as an
// administrator, issuing a key by hand, and copying it into the other
// application. That is a poor fit for the thing being connected — a Playnite
// install on a television — and it made a key something a person handles rather
// than something a program obtains.
//
// The exchange is deliberate about what it hands over. A password authorises
// everything that profile can do; the key it returns reaches only the scoped
// frontend API, is listed in the console, and can be revoked there without
// changing the password. So the client asks once, stores the key, and never
// keeps the password at all.
//
// No administrator is involved, because none is needed: a profile authorising
// read access to its own library is not an administrative act. That also means
// a non-administrator profile can connect a frontend, which the console-only
// route could never allow.
func (c *AuthController) SignInFrontendClient(w http.ResponseWriter, r *http.Request) {
	if c.frontendClients == nil {
		http.Error(w, "frontend API clients are unavailable", http.StatusServiceUnavailable)
		return
	}

	var body frontendSignInRequest
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	body.ProfileID = strings.TrimSpace(body.ProfileID)
	body.Name = strings.TrimSpace(body.Name)
	if body.ProfileID == "" || body.Name == "" {
		http.Error(w, "profile_id and name are required", http.StatusBadRequest)
		return
	}

	// The same limiter the browser login uses, keyed the same way. This
	// endpoint accepts a password, so it is exactly as attractive to guess at
	// as the login form, and leaving it unthrottled would make the throttle on
	// the other one pointless.
	key := remoteIP(r) + "|" + body.ProfileID
	if !c.allowLoginAttempt(key, time.Now()) {
		http.Error(w, "too many sign-in attempts; try again later", http.StatusTooManyRequests)
		return
	}

	if _, _, err := c.service.Login(r.Context(), body.ProfileID, body.Credential); err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, auth.ErrCredentialRequired) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	c.clearLoginAttempts(key)

	scopes, err := frontendSignInScopes(body.Scopes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	expiresAt := time.Now().UTC().Add(frontendSignInDefaultLifetime)
	issued, err := c.frontendClients.Create(r.Context(), body.ProfileID, body.Name, scopes, &expiresAt)
	if err != nil {
		c.logger.Error("issue frontend API client from sign-in", err, "profile_id", body.ProfileID)
		http.Error(w, "could not issue an access key", http.StatusInternalServerError)
		return
	}

	c.logger.Info("frontend API client audit",
		"action", "create", "outcome", "success", "via", "profile_sign_in",
		"profile_id", issued.ProfileID, "client_id", issued.ID)
	writeJSON(w, http.StatusCreated, issued)
}

// defaultFrontendSignInScopes is what a client gets when it asks for nothing:
// everything a library frontend needs to show a library and fetch what it is
// entitled to, and nothing else.
//
// Written out rather than taken from AllScopes(), which also contains
// `management`. That scope gates no route today, so granting it would change
// nothing — and that is exactly the problem: a key issued now would silently
// become a management key the day something starts gating on it. A default
// grant should say what it means and stop there.
var defaultFrontendSignInScopes = []frontendauth.Scope{
	frontendauth.ScopeCatalogRead,
	frontendauth.ScopeMetadataRead,
	frontendauth.ScopeContentRead,
	frontendauth.ScopeContentPrepare,
}

// frontendSignInScopes resolves what the key may reach.
//
// A client that names scopes gets those and no more, so something wanting only
// to read a catalogue can say so and hold a key that cannot fetch content.
func frontendSignInScopes(requested []string) ([]frontendauth.Scope, error) {
	if len(requested) == 0 {
		return append([]frontendauth.Scope(nil), defaultFrontendSignInScopes...), nil
	}

	scopes := make([]frontendauth.Scope, 0, len(requested))
	for _, value := range requested {
		scopes = append(scopes, frontendauth.Scope(value))
	}
	normalized, err := frontendauth.NormalizeScopes(scopes)
	if err != nil {
		return nil, errors.New("one of the requested permissions is not supported by this server")
	}
	return normalized, nil
}
