package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const profileHeader = "X-MGA-Profile-ID"

type ProfileController struct {
	repo   core.ProfileRepository
	logger core.Logger
}

func NewProfileController(repo core.ProfileRepository, logger core.Logger) *ProfileController {
	return &ProfileController{repo: repo, logger: logger}
}

type setupStatusResponse struct {
	SetupRequired bool            `json:"setup_required"`
	Profiles      []*core.Profile `json:"profiles"`
}

type profileRequest struct {
	DisplayName string           `json:"display_name"`
	AvatarKey   string           `json:"avatar_key"`
	Role        core.ProfileRole `json:"role"`
}

func (c *ProfileController) SetupStatus(w http.ResponseWriter, r *http.Request) {
	profiles, err := c.repo.List(r.Context())
	if err != nil {
		c.logger.Error("list profiles", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(setupStatusResponse{
		SetupRequired: len(profiles) == 0,
		Profiles:      profiles,
	})
}

func (c *ProfileController) StartFresh(w http.ResponseWriter, r *http.Request) {
	count, err := c.repo.Count(r.Context())
	if err != nil {
		c.logger.Error("count profiles", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Error(w, "setup is already complete", http.StatusConflict)
		return
	}

	var body profileRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
	}
	if strings.TrimSpace(body.DisplayName) == "" {
		body.DisplayName = "Admin Player"
	}
	now := time.Now()
	profile := &core.Profile{
		ID:          uuid.NewString(),
		DisplayName: body.DisplayName,
		AvatarKey:   body.AvatarKey,
		Role:        core.ProfileRoleAdminPlayer,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := c.repo.Create(r.Context(), profile); err != nil {
		c.logger.Error("create first profile", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(profile)
}

func (c *ProfileController) RestoreSync(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "first-run sync restore is not implemented yet", http.StatusNotImplemented)
}

func (c *ProfileController) ListProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := c.repo.List(r.Context())
	if err != nil {
		c.logger.Error("list profiles", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profiles)
}

func (c *ProfileController) CreateProfile(w http.ResponseWriter, r *http.Request) {
	if !core.ProfileIsAdmin(r.Context()) {
		http.Error(w, core.ErrProfileForbidden.Error(), http.StatusForbidden)
		return
	}
	var body profileRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	now := time.Now()
	if body.Role == "" {
		body.Role = core.ProfileRolePlayer
	}
	profile := &core.Profile{
		ID:          uuid.NewString(),
		DisplayName: body.DisplayName,
		AvatarKey:   body.AvatarKey,
		Role:        body.Role,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := c.repo.Create(r.Context(), profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(profile)
}

func (c *ProfileController) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	if !core.ProfileIsAdmin(r.Context()) {
		http.Error(w, core.ErrProfileForbidden.Error(), http.StatusForbidden)
		return
	}
	id := chi.URLParam(r, "id")
	var body profileRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	existing, err := c.repo.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.NotFound(w, r)
		return
	}
	existing.DisplayName = body.DisplayName
	existing.AvatarKey = body.AvatarKey
	if body.Role != "" {
		existing.Role = body.Role
	}
	existing.UpdatedAt = time.Now()
	if err := c.repo.Update(r.Context(), existing); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(existing)
}

func (c *ProfileController) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	if !core.ProfileIsAdmin(r.Context()) {
		http.Error(w, core.ErrProfileForbidden.Error(), http.StatusForbidden)
		return
	}
	id := chi.URLParam(r, "id")
	err := c.repo.Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func ProfileContextMiddleware(repo core.ProfileRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			profileID := strings.TrimSpace(r.Header.Get(profileHeader))
			if profileID == "" {
				profileID = strings.TrimSpace(r.URL.Query().Get("profile_id"))
			}
			if profileID == "" {
				http.Error(w, core.ErrProfileRequired.Error(), http.StatusBadRequest)
				return
			}
			profile, err := repo.GetByID(r.Context(), profileID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if profile == nil {
				http.Error(w, "profile not found", http.StatusBadRequest)
				return
			}
			next.ServeHTTP(w, r.WithContext(core.WithProfile(r.Context(), profile)))
		})
	}
}

func RequireAdminProfile(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !core.ProfileIsAdmin(r.Context()) {
			http.Error(w, core.ErrProfileForbidden.Error(), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
