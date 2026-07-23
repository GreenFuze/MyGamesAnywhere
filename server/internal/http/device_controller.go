package http

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/emulation"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/installprefs"
	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	deviceHeartbeatSeconds = 15
	deviceReadTimeout      = 45 * time.Second
)

type DeviceController struct {
	service             *devices.Service
	hub                 *devices.Hub
	logger              core.Logger
	clientInstallerPath string
	gameStore           core.GameStore
	integrationRepo     core.IntegrationRepository
	googleDriveRoot     string
	archiveTransfers    *archiveTransferRegistry
	saveDomainTransfers *saveDomainTransferRegistry
	saveSync            core.SaveSyncService
	validation          *InstallationValidationService
	installPreferences  *installprefs.Service
	emulators           *emulation.Service
}

func (c *DeviceController) SetEmulationService(service *emulation.Service) {
	c.emulators = service
}

func NewDeviceController(service *devices.Service, hub *devices.Hub, logger core.Logger, clientInstallerPath ...string) (*DeviceController, error) {
	if service == nil || hub == nil || logger == nil {
		return nil, errors.New("device service, hub, and logger are required")
	}
	installerPath := ""
	if len(clientInstallerPath) > 0 {
		installerPath = strings.TrimSpace(clientInstallerPath[0])
	}
	return &DeviceController{service: service, hub: hub, logger: logger, clientInstallerPath: installerPath, archiveTransfers: newArchiveTransferRegistry(), saveDomainTransfers: newSaveDomainTransferRegistry()}, nil
}

func (c *DeviceController) SetSaveDomainDependencies(saveSync core.SaveSyncService) {
	c.saveSync = saveSync
}

func (c *DeviceController) SetArchiveInstallDependencies(gameStore core.GameStore, integrationRepo core.IntegrationRepository, googleDriveDesktopRoot string) {
	c.gameStore = gameStore
	c.integrationRepo = integrationRepo
	c.googleDriveRoot = strings.TrimSpace(googleDriveDesktopRoot)
}

func (c *DeviceController) List(w http.ResponseWriter, r *http.Request) {
	endpoints, err := c.service.ListEndpoints(r.Context(), core.ProfileIDFromContext(r.Context()))
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, endpoints)
}

func (c *DeviceController) CreatePairingChallenge(w http.ResponseWriter, r *http.Request) {
	code, challenge, err := c.service.CreatePairingChallenge(r.Context(), core.ProfileIDFromContext(r.Context()))
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	serverURL := requestBaseURL(r)
	pairURI := &url.URL{Scheme: "mga", Host: "pair"}
	pairQuery := pairURI.Query()
	pairQuery.Set("server", serverURL)
	pairQuery.Set("code", code)
	pairURI.RawQuery = pairQuery.Encode()
	writeJSON(w, http.StatusCreated, map[string]any{
		"code":         code,
		"expires_at":   challenge.ExpiresAt,
		"pair_command": fmt.Sprintf("mga-client pair --server %s --code %s", serverURL, code),
		"pair_uri":     pairURI.String(),
	})
}

func (c *DeviceController) CreateClientLaunch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExecutionMode devicev1.ClientExecutionMode `json:"execution_mode"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	if body.ExecutionMode == "" {
		body.ExecutionMode = devicev1.ClientExecutionModeStandard
	}
	token, launch, err := c.service.CreateClientLaunchWithMode(core.ProfileIDFromContext(r.Context()), body.ExecutionMode)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	launchURI := &url.URL{Scheme: "mga", Host: "start"}
	query := launchURI.Query()
	query.Set("server", requestBaseURL(r))
	query.Set("launch_id", launch.ID)
	query.Set("token", token)
	query.Set("mode", string(launch.ExecutionMode))
	launchURI.RawQuery = query.Encode()
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         launch.ID,
		"status":     launch.Status,
		"expires_at": launch.ExpiresAt,
		"launch_uri": launchURI.String(),
	})
}

func (c *DeviceController) GetClientLaunch(w http.ResponseWriter, r *http.Request) {
	launch, err := c.service.GetClientLaunch(chi.URLParam(r, "id"), core.ProfileIDFromContext(r.Context()))
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, launch)
}

func (c *DeviceController) RedeemClientLaunch(w http.ResponseWriter, r *http.Request) {
	var request devicev1.ClientLaunchRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		return
	}
	if _, err := c.service.RedeemClientLaunch(r.Context(), request); err != nil {
		writeDeviceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *DeviceController) Pair(w http.ResponseWriter, r *http.Request) {
	var request devicev1.PairingRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		return
	}
	response, err := c.service.Pair(r.Context(), request, requestWebSocketURL(r))
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, response)
}

func (c *DeviceController) Rename(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	if err := c.service.RenameEndpoint(r.Context(), chi.URLParam(r, "id"), core.ProfileIDFromContext(r.Context()), body.DisplayName); err != nil {
		writeDeviceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *DeviceController) Revoke(w http.ResponseWriter, r *http.Request) {
	if err := c.service.RevokeEndpoint(r.Context(), chi.URLParam(r, "id"), core.ProfileIDFromContext(r.Context())); err != nil {
		writeDeviceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *DeviceController) ListGrants(w http.ResponseWriter, r *http.Request) {
	grants, err := c.service.ListGrants(r.Context(), chi.URLParam(r, "id"), core.ProfileIDFromContext(r.Context()))
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, grants)
}

func (c *DeviceController) SetGrant(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccessLevel devicev1.AccessLevel `json:"access_level"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	if err := c.service.SetGrant(r.Context(), chi.URLParam(r, "id"), core.ProfileIDFromContext(r.Context()), chi.URLParam(r, "profile_id"), body.AccessLevel); err != nil {
		writeDeviceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *DeviceController) DeleteGrant(w http.ResponseWriter, r *http.Request) {
	if err := c.service.DeleteGrant(r.Context(), chi.URLParam(r, "id"), core.ProfileIDFromContext(r.Context()), chi.URLParam(r, "profile_id")); err != nil {
		writeDeviceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *DeviceController) DispatchCommand(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string          `json:"name"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return
	}
	command, err := c.service.DispatchCommand(r.Context(), chi.URLParam(r, "id"), core.ProfileIDFromContext(r.Context()), body.Name, body.Payload)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, command)
}

func (c *DeviceController) ListCommands(w http.ResponseWriter, r *http.Request) {
	commands, err := c.service.ListCommands(r.Context(), chi.URLParam(r, "id"), core.ProfileIDFromContext(r.Context()))
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, commands)
}

func (c *DeviceController) ClientDownload(w http.ResponseWriter, _ *http.Request) {
	downloadURL := "https://github.com/GreenFuze/MyGamesAnywhere/releases/latest/download/mga-client-windows-amd64-installer.exe"
	if c.clientInstallerAvailable() {
		downloadURL = "/api/devices/client-installer"
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"platform":     "windows-amd64",
		"download_url": downloadURL,
	})
}

func (c *DeviceController) ServeClientInstaller(w http.ResponseWriter, r *http.Request) {
	if !c.clientInstallerAvailable() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.microsoft.portable-executable")
	w.Header().Set("Content-Disposition", `attachment; filename="mga-client-windows-amd64-installer.exe"`)
	http.ServeFile(w, r, c.clientInstallerPath)
}

func (c *DeviceController) clientInstallerAvailable() bool {
	if c.clientInstallerPath == "" || !filepath.IsAbs(c.clientInstallerPath) {
		return false
	}
	info, err := os.Stat(c.clientInstallerPath)
	return err == nil && info.Mode().IsRegular()
}

func (c *DeviceController) Connect(w http.ResponseWriter, r *http.Request) {
	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	transport := &websocketTransport{connection: connection}
	defer transport.Close()

	authContext, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	helloEnvelope, err := readDeviceEnvelope(authContext, connection)
	if err != nil || helloEnvelope.Type != devicev1.MessageHello {
		_ = connection.Close(websocket.StatusPolicyViolation, "hello required")
		return
	}
	hello, err := devicev1.DecodePayload[devicev1.Hello](helloEnvelope)
	if err != nil || hello.Validate() != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, "invalid hello")
		return
	}
	endpoint, err := c.service.EndpointForConnection(authContext, hello.EndpointID)
	if err != nil || endpoint.ClientInstanceID != hello.ClientInstanceID {
		_ = connection.Close(websocket.StatusPolicyViolation, "unknown endpoint")
		return
	}
	selectedVersion, err := devicev1.NegotiateVersion(hello.Versions, devicev1.SupportedVersionRange())
	if err != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, "incompatible protocol")
		return
	}

	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		_ = connection.Close(websocket.StatusInternalError, "challenge failed")
		return
	}
	challenge := devicev1.AuthChallenge{
		ConnectionID: uuid.NewString(),
		Nonce:        base64.RawURLEncoding.EncodeToString(nonce),
		IssuedAt:     time.Now().UTC(),
	}
	if err := writeDeviceMessage(authContext, connection, devicev1.MessageAuthChallenge, challenge.ConnectionID, helloEnvelope.MessageID, challenge); err != nil {
		return
	}
	responseEnvelope, err := readDeviceEnvelope(authContext, connection)
	if err != nil || responseEnvelope.Type != devicev1.MessageAuthResponse {
		_ = connection.Close(websocket.StatusPolicyViolation, "authentication response required")
		return
	}
	response, err := devicev1.DecodePayload[devicev1.AuthResponse](responseEnvelope)
	if err != nil || response.Validate() != nil || response.EndpointID != endpoint.ID || response.ConnectionID != challenge.ConnectionID {
		_ = connection.Close(websocket.StatusPolicyViolation, "invalid authentication response")
		return
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(endpoint.PublicKey)
	if err != nil {
		_ = connection.Close(websocket.StatusInternalError, "invalid paired key")
		return
	}
	signingBytes, err := challenge.SigningBytes(endpoint.ID)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(publicKey), signingBytes, mustDecodeBase64URL(response.Signature)) {
		_ = connection.Close(websocket.StatusPolicyViolation, "endpoint authentication failed")
		return
	}
	if err := c.service.MarkConnected(r.Context(), endpoint, hello, selectedVersion); err != nil {
		_ = connection.Close(websocket.StatusInternalError, "presence update failed")
		return
	}
	if err := c.hub.Register(endpoint.ID, transport); err != nil {
		_ = connection.Close(websocket.StatusInternalError, "connection registration failed")
		return
	}
	defer func() {
		if c.hub.Unregister(endpoint.ID, transport) {
			if err := c.service.MarkOffline(context.Background(), endpoint.ID); err != nil {
				c.logger.Error("mark device offline", err, "endpoint_id", endpoint.ID)
			}
		}
	}()
	accepted := devicev1.ConnectionAccepted{
		ConnectionID:     challenge.ConnectionID,
		ProtocolVersion:  selectedVersion,
		HeartbeatSeconds: deviceHeartbeatSeconds,
		ServerTime:       time.Now().UTC(),
	}
	if err := writeDeviceMessage(r.Context(), connection, devicev1.MessageConnectionAccepted, uuid.NewString(), responseEnvelope.MessageID, accepted); err != nil {
		return
	}
	c.readConnectedMessages(r.Context(), endpoint.ID, connection)
}

func (c *DeviceController) readConnectedMessages(ctx context.Context, endpointID string, connection *websocket.Conn) {
	for {
		readContext, cancel := context.WithTimeout(ctx, deviceReadTimeout)
		envelope, err := readDeviceEnvelope(readContext, connection)
		cancel()
		if err != nil {
			return
		}
		switch envelope.Type {
		case devicev1.MessageHeartbeat:
			heartbeat, err := devicev1.DecodePayload[devicev1.Heartbeat](envelope)
			if err != nil || c.service.RecordHeartbeat(ctx, endpointID, heartbeat) != nil {
				return
			}
		case devicev1.MessageInventoryReport:
			inventory, err := devicev1.DecodePayload[devicev1.DeviceInventory](envelope)
			if err != nil || c.service.RecordInventory(ctx, endpointID, inventory) != nil {
				return
			}
		case devicev1.MessageCommandAccepted:
			update, err := devicev1.DecodePayload[devicev1.CommandStatusUpdate](envelope)
			if err != nil || update.Validate() != nil || c.service.RecordCommandStatus(ctx, endpointID, update.CommandID, devicev1.CommandAccepted) != nil {
				return
			}
		case devicev1.MessageCommandProgress:
			progress, err := devicev1.DecodePayload[devicev1.CommandProgress](envelope)
			if err != nil || progress.Validate() != nil || c.service.RecordCommandProgress(ctx, endpointID, progress) != nil {
				return
			}
		case devicev1.MessageCommandResult, devicev1.MessageCommandRejected:
			result, err := devicev1.DecodePayload[devicev1.CommandResult](envelope)
			if err != nil || c.service.RecordCommandResult(ctx, endpointID, result) != nil {
				return
			}
		default:
			return
		}
	}
}

type websocketTransport struct {
	connection *websocket.Conn
	mu         sync.Mutex
}

func (t *websocketTransport) Write(ctx context.Context, data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connection.Write(ctx, websocket.MessageText, data)
}

func (t *websocketTransport) Close() error {
	return t.connection.Close(websocket.StatusNormalClosure, "connection closed")
}

func readDeviceEnvelope(ctx context.Context, connection *websocket.Conn) (devicev1.Envelope, error) {
	messageType, data, err := connection.Read(ctx)
	if err != nil {
		return devicev1.Envelope{}, err
	}
	if messageType != websocket.MessageText {
		return devicev1.Envelope{}, errors.New("device messages must be JSON text")
	}
	return devicev1.DecodeEnvelope(data)
}

func writeDeviceMessage(ctx context.Context, connection *websocket.Conn, messageType devicev1.MessageType, messageID, correlationID string, payload any) error {
	envelope, err := devicev1.NewEnvelope(messageType, messageID, correlationID, time.Now().UTC(), payload)
	if err != nil {
		return err
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return connection.Write(ctx, websocket.MessageText, data)
}

func mustDecodeBase64URL(value string) []byte {
	decoded, _ := base64.RawURLEncoding.DecodeString(value)
	return decoded
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return (&url.URL{Scheme: scheme, Host: r.Host}).String()
}

func requestWebSocketURL(r *http.Request) string {
	scheme := "ws"
	if r.TLS != nil {
		scheme = "wss"
	}
	return (&url.URL{Scheme: scheme, Host: r.Host, Path: "/api/devices/connect"}).String()
}

func writeDeviceError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, devices.ErrEndpointNotFound):
		status = http.StatusNotFound
	case errors.Is(err, devices.ErrInstallationNotFound):
		status = http.StatusNotFound
	case errors.Is(err, devices.ErrDeviceForbidden):
		status = http.StatusForbidden
	case errors.Is(err, devices.ErrEndpointOffline):
		status = http.StatusConflict
	case errors.Is(err, devices.ErrCapabilityMissing):
		status = http.StatusConflict
	case errors.Is(err, devices.ErrLastOwner):
		status = http.StatusConflict
	case errors.Is(err, devices.ErrGrantNotFound):
		status = http.StatusNotFound
	case errors.Is(err, devices.ErrClientAlreadyPaired):
		status = http.StatusConflict
	case errors.Is(err, devices.ErrPairingIdentity):
		status = http.StatusForbidden
	case errors.Is(err, devices.ErrClientLaunchNotFound):
		status = http.StatusNotFound
	case errors.Is(err, devices.ErrClientLaunchExpired), errors.Is(err, devices.ErrClientLaunchUsed):
		status = http.StatusGone
	case strings.Contains(strings.ToLower(err.Error()), "database"):
		status = http.StatusInternalServerError
	}
	http.Error(w, err.Error(), status)
}
