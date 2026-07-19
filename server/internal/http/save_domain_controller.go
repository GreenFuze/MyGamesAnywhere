package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
	"github.com/go-chi/chi/v5"
)

func (c *DeviceController) ClaimScummVMSaveDomain(w http.ResponseWriter, r *http.Request) {
	if c.emulators == nil || c.gameStore == nil || c.archiveTransfers == nil {
		http.Error(w, "ScummVM save setup is unavailable", http.StatusServiceUnavailable)
		return
	}
	endpointID := chi.URLParam(r, "id")
	gameID := chi.URLParam(r, "game_id")
	sourceGameID, err := decodedPathParam(r, "source_game_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var body struct {
		LocalSaveDomainID string `json:"local_save_domain_id,omitempty"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	game, err := c.gameStore.GetCanonicalGameByID(r.Context(), gameID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	if game == nil {
		http.NotFound(w, r)
		return
	}
	source := findEmulatorSourceGame(game, sourceGameID)
	if source == nil || source.Status != "found" || source.GroupKind != core.GroupKindSelfContained {
		http.Error(w, "this game copy is not ready for ScummVM", http.StatusConflict)
		return
	}
	profileID := core.ProfileIDFromContext(r.Context())
	if _, err := c.emulators.RequireReady(r.Context(), endpointID, profileID, game.Platform, "scummvm"); err != nil {
		writeEmulationError(w, err)
		return
	}
	artifacts, err := c.createEmulatorArtifacts(source)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	fingerprint, err := devicev1.EmulatorRouteFingerprint(artifacts)
	if err != nil {
		http.Error(w, "MGA could not identify this exact ScummVM copy", http.StatusConflict)
		return
	}
	request := devicev1.SaveDomainClaimRequest{
		GameID: gameID, SourceGameID: sourceGameID, Title: game.Title,
		AdapterID: "scummvm", RouteKind: "emulator", EmulatorID: "scummvm",
		RouteFingerprint: fingerprint, LocalSaveDomainID: strings.TrimSpace(body.LocalSaveDomainID),
	}
	payload, err := json.Marshal(request)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), endpointID, profileID, devicev1.CapabilitySaveDomainClaim, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) SnapshotSaveDomain(w http.ResponseWriter, r *http.Request) {
	if c.gameStore == nil || c.saveSync == nil || c.saveDomainTransfers == nil {
		http.Error(w, "save backup is unavailable", http.StatusServiceUnavailable)
		return
	}
	endpointID, gameID := chi.URLParam(r, "id"), chi.URLParam(r, "game_id")
	sourceGameID, err := decodedPathParam(r, "source_game_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var body struct {
		IntegrationID string `json:"integration_id"`
		Force         bool   `json:"force,omitempty"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	profileID := core.ProfileIDFromContext(r.Context())
	game, link, err := c.requireOwnedSaveDomain(r.Context(), endpointID, profileID, gameID, sourceGameID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	ref := core.SaveSyncSlotRef{CanonicalGameID: gameID, SourceGameID: sourceGameID, Runtime: "scummvm", SlotID: devicev1.SaveDomainSlotID, IntegrationID: strings.TrimSpace(body.IntegrationID)}
	token, err := c.saveDomainTransfers.CreateUpload(saveDomainUpload{Ref: ref, BaseManifestHash: link.LastSnapshotManifestHash, Force: body.Force})
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	request := devicev1.SaveDomainSnapshotRequest{GameID: gameID, SourceGameID: sourceGameID, Title: game.Title, LocalSaveDomainID: link.LocalSaveDomainID, UploadURL: "/api/device-transfers/save-domain", UploadToken: token}
	payload, err := json.Marshal(request)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), endpointID, profileID, devicev1.CapabilitySaveDomainSnapshot, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) RestoreSaveDomain(w http.ResponseWriter, r *http.Request) {
	if c.gameStore == nil || c.saveSync == nil || c.saveDomainTransfers == nil {
		http.Error(w, "save restore is unavailable", http.StatusServiceUnavailable)
		return
	}
	endpointID, gameID := chi.URLParam(r, "id"), chi.URLParam(r, "game_id")
	sourceGameID, err := decodedPathParam(r, "source_game_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var body struct {
		IntegrationID string `json:"integration_id"`
		PreserveLocal bool   `json:"preserve_local,omitempty"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	profileID := core.ProfileIDFromContext(r.Context())
	game, link, err := c.requireOwnedSaveDomain(r.Context(), endpointID, profileID, gameID, sourceGameID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	ref := core.SaveSyncSlotRef{CanonicalGameID: gameID, SourceGameID: sourceGameID, Runtime: "scummvm", SlotID: devicev1.SaveDomainSlotID, IntegrationID: strings.TrimSpace(body.IntegrationID)}
	snapshot, err := c.saveSync.GetSlot(r.Context(), ref)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if snapshot == nil {
		http.Error(w, "there is no saved backup to restore", http.StatusConflict)
		return
	}
	transfer, err := protocolSaveDomainSnapshot(snapshot)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	token, err := c.saveDomainTransfers.CreateDownload(transfer)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	request := devicev1.SaveDomainRestoreRequest{GameID: gameID, SourceGameID: sourceGameID, Title: game.Title, LocalSaveDomainID: link.LocalSaveDomainID, DownloadURL: "/api/device-transfers/save-domain", DownloadToken: token, ManifestHash: snapshot.ManifestHash, PreserveLocal: body.PreserveLocal}
	payload, err := json.Marshal(request)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), endpointID, profileID, devicev1.CapabilitySaveDomainRestore, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) ReconcileSaveDomain(w http.ResponseWriter, r *http.Request) {
	if c.gameStore == nil || c.saveSync == nil || c.saveDomainTransfers == nil {
		http.Error(w, "save reconciliation is unavailable", http.StatusServiceUnavailable)
		return
	}
	endpointID, gameID := chi.URLParam(r, "id"), chi.URLParam(r, "game_id")
	sourceGameID, err := decodedPathParam(r, "source_game_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var body struct {
		IntegrationID string `json:"integration_id"`
		Strategy      string `json:"strategy"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	profileID := core.ProfileIDFromContext(r.Context())
	game, link, err := c.requireSaveDomain(r.Context(), endpointID, profileID, gameID, sourceGameID, "reconciliation_required")
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	ref := core.SaveSyncSlotRef{CanonicalGameID: gameID, SourceGameID: sourceGameID, Runtime: "scummvm", SlotID: devicev1.SaveDomainSlotID, IntegrationID: strings.TrimSpace(body.IntegrationID)}
	request := devicev1.SaveDomainReconcileRequest{GameID: gameID, SourceGameID: sourceGameID, Title: game.Title, LocalSaveDomainID: link.LocalSaveDomainID, Strategy: strings.TrimSpace(body.Strategy), TransferURL: "/api/device-transfers/save-domain"}
	switch request.Strategy {
	case "keep_local":
		request.TransferToken, err = c.saveDomainTransfers.CreateUpload(saveDomainUpload{Ref: ref, BaseManifestHash: link.LastSnapshotManifestHash, Force: true})
	case "keep_server":
		var snapshot *core.SaveSyncSnapshot
		snapshot, err = c.saveSync.GetSlot(r.Context(), ref)
		if err == nil && snapshot == nil {
			err = errors.New("there is no saved backup to keep")
		}
		if err == nil {
			var transfer devicev1.SaveDomainSnapshot
			transfer, err = protocolSaveDomainSnapshot(snapshot)
			if err == nil {
				request.TransferToken, err = c.saveDomainTransfers.CreateDownload(transfer)
				request.ManifestHash = snapshot.ManifestHash
			}
		}
	default:
		err = errors.New("choose either this device or the saved backup")
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	payload, err := json.Marshal(request)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), endpointID, profileID, devicev1.CapabilitySaveDomainReconcile, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) requireOwnedSaveDomain(ctx context.Context, endpointID, profileID, gameID, sourceGameID string) (*core.CanonicalGame, devices.SaveDomainLink, error) {
	return c.requireSaveDomain(ctx, endpointID, profileID, gameID, sourceGameID, "owned_here")
}

func (c *DeviceController) requireSaveDomain(ctx context.Context, endpointID, profileID, gameID, sourceGameID, authorityState string) (*core.CanonicalGame, devices.SaveDomainLink, error) {
	game, err := c.gameStore.GetCanonicalGameByID(ctx, gameID)
	if err != nil {
		return nil, devices.SaveDomainLink{}, err
	}
	if game == nil || findEmulatorSourceGame(game, sourceGameID) == nil {
		return nil, devices.SaveDomainLink{}, devices.ErrInstallationNotFound
	}
	links, err := c.service.ListSaveDomainLinks(ctx, endpointID, profileID)
	if err != nil {
		return nil, devices.SaveDomainLink{}, err
	}
	for _, link := range links {
		if link.GameID == gameID && link.SourceGameID == sourceGameID && link.RouteKind == "emulator" && link.EmulatorID == "scummvm" && link.AuthorityState == authorityState {
			return game, link, nil
		}
	}
	return nil, devices.SaveDomainLink{}, fmt.Errorf("this MGA Server does not have the required authority for the selected local save domain")
}

func (c *DeviceController) ReleaseSaveDomain(w http.ResponseWriter, r *http.Request) {
	if c.gameStore == nil {
		http.Error(w, "save access release is unavailable", http.StatusServiceUnavailable)
		return
	}
	endpointID := chi.URLParam(r, "id")
	gameID := chi.URLParam(r, "game_id")
	sourceGameID, err := decodedPathParam(r, "source_game_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var body struct {
		LocalSaveDomainID string `json:"local_save_domain_id"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	game, err := c.gameStore.GetCanonicalGameByID(r.Context(), gameID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	if game == nil || findEmulatorSourceGame(game, sourceGameID) == nil {
		http.NotFound(w, r)
		return
	}
	request := devicev1.SaveDomainReleaseRequest{GameID: gameID, SourceGameID: sourceGameID, Title: game.Title, LocalSaveDomainID: strings.TrimSpace(body.LocalSaveDomainID)}
	payload, err := json.Marshal(request)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), endpointID, core.ProfileIDFromContext(r.Context()), devicev1.CapabilitySaveDomainRelease, payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}
