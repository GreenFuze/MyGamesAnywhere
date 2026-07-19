package v1

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const SaveDomainCommandSchemaVersion uint16 = 1

type SaveDomainClaimRequest struct {
	GameID            string `json:"game_id"`
	SourceGameID      string `json:"source_game_id"`
	Title             string `json:"title"`
	AdapterID         string `json:"adapter_id"`
	RouteKind         string `json:"route_kind"`
	EmulatorID        string `json:"emulator_id,omitempty"`
	RouteFingerprint  string `json:"route_fingerprint"`
	LocalSaveDomainID string `json:"local_save_domain_id,omitempty"`
}

func (r SaveDomainClaimRequest) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" || strings.TrimSpace(r.Title) == "" {
		return errors.New("game ID, source game ID, and title are required")
	}
	if r.AdapterID != "scummvm" || r.RouteKind != "emulator" || r.EmulatorID != "scummvm" {
		return errors.New("the initial save-domain claim supports only an exact ScummVM emulator route")
	}
	if !sha256Pattern.MatchString(strings.ToLower(strings.TrimSpace(r.RouteFingerprint))) {
		return errors.New("route_fingerprint must contain 64 hexadecimal characters")
	}
	return nil
}

type SaveDomainClaimResult struct {
	GameID            string    `json:"game_id"`
	SourceGameID      string    `json:"source_game_id"`
	LocalSaveDomainID string    `json:"local_save_domain_id"`
	AdapterID         string    `json:"adapter_id"`
	RouteFingerprint  string    `json:"route_fingerprint"`
	State             string    `json:"state"`
	GrantedAt         time.Time `json:"granted_at"`
}

func (r SaveDomainClaimResult) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" || strings.TrimSpace(r.LocalSaveDomainID) == "" || r.AdapterID != "scummvm" {
		return errors.New("save-domain claim result identity is incomplete")
	}
	if !sha256Pattern.MatchString(strings.ToLower(strings.TrimSpace(r.RouteFingerprint))) || (r.State != "owned_here" && r.State != "reconciliation_required") || r.GrantedAt.IsZero() {
		return errors.New("save-domain claim result evidence is invalid")
	}
	return nil
}

type SaveDomainReleaseRequest struct {
	GameID            string `json:"game_id"`
	SourceGameID      string `json:"source_game_id"`
	Title             string `json:"title"`
	LocalSaveDomainID string `json:"local_save_domain_id"`
}

func (r SaveDomainReleaseRequest) Validate() error {
	for name, value := range map[string]string{"game_id": r.GameID, "source_game_id": r.SourceGameID, "title": r.Title, "local_save_domain_id": r.LocalSaveDomainID} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	return nil
}

type SaveDomainReleaseResult struct {
	GameID            string    `json:"game_id"`
	SourceGameID      string    `json:"source_game_id"`
	LocalSaveDomainID string    `json:"local_save_domain_id"`
	State             string    `json:"state"`
	ReleasedAt        time.Time `json:"released_at"`
}

func (r SaveDomainReleaseResult) Validate() error {
	if strings.TrimSpace(r.GameID) == "" || strings.TrimSpace(r.SourceGameID) == "" || strings.TrimSpace(r.LocalSaveDomainID) == "" || r.State != "released" || r.ReleasedAt.IsZero() {
		return errors.New("save-domain release result is invalid")
	}
	return nil
}
