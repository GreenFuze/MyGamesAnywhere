package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	dbpkg "github.com/GreenFuze/MyGamesAnywhere/server/internal/db"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
	"github.com/go-chi/chi/v5"
)

func TestFindSupportedArchiveAndSafeInstallFolderName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Game.zip", "Game.7z", "Game.rar"} {
		game := &core.CanonicalGame{SourceGames: []*core.SourceGame{{
			ID:    "source-1",
			Files: []core.GameFile{{Path: "Games/Installers/" + name, FileName: name, FileKind: "archive", Size: 10}},
		}}}
		source, archive := findSupportedArchive(game, "source-1")
		if source == nil || archive == nil || archive.FileName != name {
			t.Fatalf("findSupportedArchive(%q) = %#v, %#v", name, source, archive)
		}
		format, supported := supportedArchiveFormat(archive.Path)
		if !supported || format != strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".") {
			t.Fatalf("supportedArchiveFormat(%q) = %q, %t", name, format, supported)
		}
	}
	multiple := &core.CanonicalGame{SourceGames: []*core.SourceGame{{
		ID: "source-1",
		Files: []core.GameFile{
			{Path: "Game.zip", FileName: "Game.zip"},
			{Path: "Game.rar", FileName: "Game.rar"},
		},
	}}}
	if source, archive := findSupportedArchive(multiple, "source-1"); source == nil || archive != nil {
		t.Fatalf("multiple archives = %#v, %#v", source, archive)
	}
	if got := safeInstallFolderName(`Game: The / Adventure?`); got != "Game  The   Adventure" {
		t.Fatalf("safeInstallFolderName() = %q", got)
	}
}

func TestArchiveTransferRegistryExpiresTokens(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "game.zip")
	if err := os.WriteFile(path, []byte("zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	registry := newArchiveTransferRegistry()
	registry.now = func() time.Time { return now }
	token, err := registry.Create(path, "game.zip")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, ok := registry.Get(token); !ok {
		t.Fatal("Get() rejected a live token")
	}
	now = now.Add(archiveTransferLifetime + time.Second)
	if _, ok := registry.Get(token); ok {
		t.Fatal("Get() accepted an expired token")
	}
}

func TestServeArchiveTransferRequiresBearerGrant(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "game.zip")
	if err := os.WriteFile(path, []byte("zip bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	controller := &DeviceController{archiveTransfers: newArchiveTransferRegistry()}
	token, err := controller.archiveTransfers.Create(path, "game.zip")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	withoutGrant := httptest.NewRecorder()
	controller.ServeArchiveTransfer(withoutGrant, httptest.NewRequest(http.MethodGet, "/api/device-transfers/archive", nil))
	if withoutGrant.Code != http.StatusNotFound {
		t.Fatalf("request without grant status = %d, want %d", withoutGrant.Code, http.StatusNotFound)
	}

	withGrant := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/device-transfers/archive", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	controller.ServeArchiveTransfer(withGrant, request)
	if withGrant.Code != http.StatusOK || withGrant.Body.String() != "zip bytes" {
		t.Fatalf("request with grant = status %d body %q", withGrant.Code, withGrant.Body.String())
	}
}

type captureDeviceTransport struct {
	writes [][]byte
}

func (t *captureDeviceTransport) Write(_ context.Context, data []byte) error {
	t.writes = append(t.writes, append([]byte(nil), data...))
	return nil
}

func (*captureDeviceTransport) Close() error {
	return nil
}

func TestDeviceLaunchRoutesDecodeSourceIDSelectCandidateAndDispatch(t *testing.T) {
	t.Parallel()
	database := dbpkg.NewSQLiteDatabase(noopLogger{}, restoreConfig{"DB_PATH": filepath.Join(t.TempDir(), "launch.sqlite")})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	const (
		endpointID   = "endpoint-1"
		profileID    = "profile-1"
		gameID       = "game-1"
		sourceGameID = "source:encoded"
	)
	now := time.Now().Unix()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO profiles (id, display_name, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, []any{profileID, "Player", "admin_player", now, now}},
		{`INSERT INTO canonical_games (id, created_at) VALUES (?, ?)`, []any{gameID, now}},
		{`INSERT INTO source_games (id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, review_state, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{sourceGameID, profileID, "integration-1", "game-source-google-drive", "source-1", "Game", "windows_pc", "base_game", "packed", "found", "matched", now}},
		{`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, client_version, protocol_version, capabilities_json, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{endpointID, "instance-1", "key", "PC", "pc", "user", "windows", "amd64", "dev", 1, `["game.launch"]`, "ready", now, now}},
		{`INSERT INTO device_grants (endpoint_id, profile_id, access_level, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, []any{endpointID, profileID, "manage", now, now}},
		{`INSERT INTO device_game_installations (endpoint_id, game_id, source_game_id, profile_id, install_root, install_path, archive_sha256, archive_bytes, installed_at, updated_at, launch_target, launch_candidates_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{endpointID, gameID, sourceGameID, profileID, `C:\Games`, `C:\Games\Game`, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 42, now, now, "Game/game.exe", `["Game/game.exe","Game/alternate.exe"]`}},
	} {
		if _, err := database.GetDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed launch route: %v", err)
		}
	}

	hub := devices.NewHub()
	transport := &captureDeviceTransport{}
	if err := hub.Register(endpointID, transport); err != nil {
		t.Fatal(err)
	}
	service, err := devices.NewService(dbpkg.NewDeviceStore(database), hub)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewDeviceController(service, hub, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := core.WithProfile(r.Context(), &core.Profile{ID: profileID, Role: core.ProfileRoleAdminPlayer})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	router.Put("/api/devices/{id}/games/{game_id}/sources/{source_game_id}/launch-target", controller.SetLaunchTarget)
	router.Post("/api/devices/{id}/games/{game_id}/sources/{source_game_id}/launch", controller.LaunchGame)

	selectRequest := httptest.NewRequest(
		http.MethodPut,
		"/api/devices/"+endpointID+"/games/"+gameID+"/sources/source%3Aencoded/launch-target",
		bytes.NewBufferString(`{"launch_target":"Game/alternate.exe"}`),
	)
	selectRequest.Header.Set("Content-Type", "application/json")
	selectRecorder := httptest.NewRecorder()
	router.ServeHTTP(selectRecorder, selectRequest)
	if selectRecorder.Code != http.StatusNoContent {
		t.Fatalf("select launch target status = %d, body = %s", selectRecorder.Code, selectRecorder.Body.String())
	}
	var selected string
	if err := database.GetDB().QueryRow(`SELECT launch_target FROM device_game_installations WHERE endpoint_id=?`, endpointID).Scan(&selected); err != nil {
		t.Fatal(err)
	}
	if selected != "Game/alternate.exe" {
		t.Fatalf("selected launch target = %q", selected)
	}

	launchRecorder := httptest.NewRecorder()
	router.ServeHTTP(launchRecorder, httptest.NewRequest(
		http.MethodPost,
		"/api/devices/"+endpointID+"/games/"+gameID+"/sources/source%3Aencoded/launch",
		http.NoBody,
	))
	if launchRecorder.Code != http.StatusAccepted {
		t.Fatalf("launch status = %d, body = %s", launchRecorder.Code, launchRecorder.Body.String())
	}
	var command devices.Command
	if err := json.Unmarshal(launchRecorder.Body.Bytes(), &command); err != nil {
		t.Fatal(err)
	}
	if command.Name != devicev1.CapabilityGameLaunch || len(transport.writes) != 1 {
		t.Fatalf("command = %#v, writes = %d", command, len(transport.writes))
	}
	envelope, err := devicev1.DecodeEnvelope(transport.writes[0])
	if err != nil {
		t.Fatal(err)
	}
	request, err := devicev1.DecodePayload[devicev1.CommandRequest](envelope)
	if err != nil {
		t.Fatal(err)
	}
	var launch devicev1.GameLaunchRequest
	if err := json.Unmarshal(request.Payload, &launch); err != nil {
		t.Fatal(err)
	}
	if launch.SourceGameID != sourceGameID || launch.LaunchTarget != "Game/alternate.exe" {
		t.Fatalf("launch request = %#v", launch)
	}
}

func TestFailedGogInstallRoutesAuthorizeEncodeIgnoreReopenAndDispatchCleanup(t *testing.T) {
	t.Parallel()
	database := dbpkg.NewSQLiteDatabase(noopLogger{}, restoreConfig{"DB_PATH": filepath.Join(t.TempDir(), "cleanup.sqlite")})
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	const (
		endpointID   = "endpoint-1"
		profileID    = "profile-1"
		gameID       = "game-1"
		sourceGameID = "scan:encoded-source"
		markerID     = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	)
	now := time.Now().Unix()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO profiles (id, display_name, role, created_at, updated_at) VALUES (?, 'Player', 'admin_player', ?, ?)`, []any{profileID, now, now}},
		{`INSERT INTO canonical_games (id, created_at) VALUES (?, ?)`, []any{gameID, now}},
		{`INSERT INTO source_games (id, profile_id, integration_id, plugin_id, external_id, raw_title, platform, kind, group_kind, status, review_state, created_at)
			VALUES (?, ?, 'integration-1', 'game-source-google-drive', 'source-1', 'Game', 'windows_pc', 'base_game', 'packed', 'found', 'matched', ?)`, []any{sourceGameID, profileID, now}},
		{`INSERT INTO device_endpoints (id, client_instance_id, public_key, display_name, host_name, os_user, platform, arch, client_version, protocol_version, capabilities_json, status, created_at, updated_at)
			VALUES (?, 'instance-1', 'key', 'PC', 'pc', 'user', 'windows', 'amd64', 'dev', 1, '["game.cleanup_gog_inno_failed"]', 'ready', ?, ?)`, []any{endpointID, now, now}},
		{`INSERT INTO device_grants (endpoint_id, profile_id, access_level, created_at, updated_at) VALUES (?, ?, 'manage', ?, ?)`, []any{endpointID, profileID, now, now}},
		{`INSERT INTO device_game_installations
			(endpoint_id, game_id, source_game_id, profile_id, install_root, install_path, archive_sha256, archive_bytes, installed_at, updated_at,
			 install_kind, installer_family, uninstall_target, install_state, state_reason, cleanup_marker_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'gog_inno', 'gog_inno', 'unins000.exe', 'cleanup_required', 'installer_exit_nonzero', ?)`,
			[]any{endpointID, gameID, sourceGameID, profileID, `C:\Games`, `C:\Games\Failed Game`, strings.Repeat("a", 64), 42, now, now, markerID}},
	} {
		if _, err := database.GetDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed cleanup route: %v", err)
		}
	}

	hub := devices.NewHub()
	transport := &captureDeviceTransport{}
	if err := hub.Register(endpointID, transport); err != nil {
		t.Fatal(err)
	}
	service, err := devices.NewService(dbpkg.NewDeviceStore(database), hub)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewDeviceController(service, hub, noopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := core.WithProfile(r.Context(), &core.Profile{ID: profileID, Role: core.ProfileRoleAdminPlayer})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	router.Post("/api/devices/{id}/games/{game_id}/sources/{source_game_id}/cleanup-failed", controller.CleanupFailedGogInno)
	router.Post("/api/devices/{id}/games/{game_id}/sources/{source_game_id}/ignore-failed", controller.IgnoreFailedGogInno)
	router.Post("/api/devices/{id}/games/{game_id}/sources/{source_game_id}/reopen-failed-cleanup", controller.ReopenFailedGogInno)
	baseURL := "/api/devices/" + endpointID + "/games/" + gameID + "/sources/scan%3Aencoded-source/"
	post := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, baseURL+path, bytes.NewBufferString(`{}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		return recorder
	}

	if _, err := database.GetDB().Exec(`UPDATE device_grants SET access_level='view' WHERE endpoint_id=? AND profile_id=?`, endpointID, profileID); err != nil {
		t.Fatal(err)
	}
	if recorder := post("ignore-failed"); recorder.Code != http.StatusForbidden {
		t.Fatalf("view-only ignore status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if _, err := database.GetDB().Exec(`UPDATE device_grants SET access_level='manage' WHERE endpoint_id=? AND profile_id=?`, endpointID, profileID); err != nil {
		t.Fatal(err)
	}
	if recorder := post("ignore-failed"); recorder.Code != http.StatusOK {
		t.Fatalf("ignore status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(transport.writes) != 0 {
		t.Fatalf("Ignore dispatched %d device commands", len(transport.writes))
	}
	var state string
	var ignoredAt int64
	var ignoredBy string
	if err := database.GetDB().QueryRow(`SELECT install_state, cleanup_ignored_at, cleanup_ignored_by_profile_id FROM device_game_installations
		WHERE endpoint_id=? AND source_game_id=?`, endpointID, sourceGameID).Scan(&state, &ignoredAt, &ignoredBy); err != nil {
		t.Fatal(err)
	}
	if state != devicev1.InstallStateIgnoredFailure || ignoredAt == 0 || ignoredBy != profileID {
		t.Fatalf("ignored state=%q at=%d by=%q", state, ignoredAt, ignoredBy)
	}
	if recorder := post("reopen-failed-cleanup"); recorder.Code != http.StatusOK {
		t.Fatalf("reopen status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(transport.writes) != 0 {
		t.Fatalf("Reopen dispatched %d device commands", len(transport.writes))
	}
	if recorder := post("cleanup-failed"); recorder.Code != http.StatusAccepted {
		t.Fatalf("cleanup status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(transport.writes) != 1 {
		t.Fatalf("cleanup writes = %d", len(transport.writes))
	}
	envelope, err := devicev1.DecodeEnvelope(transport.writes[0])
	if err != nil {
		t.Fatal(err)
	}
	commandRequest, err := devicev1.DecodePayload[devicev1.CommandRequest](envelope)
	if err != nil {
		t.Fatal(err)
	}
	var cleanup devicev1.GogInnoFailedCleanupRequest
	if err := json.Unmarshal(commandRequest.Payload, &cleanup); err != nil {
		t.Fatal(err)
	}
	if commandRequest.Name != devicev1.CapabilityGameCleanupGogInnoFailed || cleanup.SourceGameID != sourceGameID || cleanup.CleanupMarkerID != markerID || cleanup.InstallPath != `C:\Games\Failed Game` {
		t.Fatalf("cleanup command=%#v payload=%#v", commandRequest, cleanup)
	}
}
