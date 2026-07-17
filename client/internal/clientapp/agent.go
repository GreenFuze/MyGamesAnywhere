package clientapp

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/buildinfo"
	clientconfig "github.com/GreenFuze/MyGamesAnywhere/client/internal/config"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/coder/websocket"
	"github.com/google/uuid"
)

var errStopRequested = errors.New("MGA Client stop requested")

const inventoryRefreshInterval = 15 * time.Minute

type Agent struct {
	config        clientconfig.Config
	privateKey    ed25519.PrivateKey
	buildInfo     buildinfo.Info
	active        atomic.Int32
	logger        *log.Logger
	executionMode devicev1.ClientExecutionMode
	inventory     InventoryCollector
	installer     ArchiveInstaller
	gogInstaller  GogInnoInstaller
	launcher      GameLauncher
	emulator      EmulatorLauncher
	emulatorSetup EmulatorSetupManager
	validator     InstallationValidator
}

func NewAgent(config clientconfig.Config, privateKey ed25519.PrivateKey, info buildinfo.Info, logger *log.Logger) (*Agent, error) {
	return NewAgentWithExecutionMode(config, privateKey, info, logger, devicev1.ClientExecutionModeStandard)
}

// NewAgentWithExecutionMode creates an agent that reports its actual current
// Windows execution mode as endpoint runtime metadata.
func NewAgentWithExecutionMode(config clientconfig.Config, privateKey ed25519.PrivateKey, info buildinfo.Info, logger *log.Logger, executionMode devicev1.ClientExecutionMode) (*Agent, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if executionMode == "" {
		executionMode = devicev1.ClientExecutionModeStandard
	}
	if err := executionMode.Validate(); err != nil {
		return nil, err
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("valid endpoint private key is required")
	}
	if logger == nil {
		return nil, errors.New("agent logger is required")
	}
	installer, err := NewManagedArchiveInstaller(config.ServerURL)
	if err != nil {
		return nil, err
	}
	gogInstaller, err := newPlatformGogInnoInstaller(config.ServerURL)
	if err != nil {
		return nil, err
	}
	validator, err := NewLocalInstallationValidator(newRegisteredProgramInspector())
	if err != nil {
		return nil, err
	}
	inventory := NewLocalInventoryCollector()
	emulator, err := NewManagedEmulatorLauncher(config.ServerURL, inventory)
	if err != nil {
		return nil, err
	}
	emulatorSetup, err := NewManagedEmulatorSetupManager(inventory)
	if err != nil {
		return nil, err
	}
	return &Agent{
		config: config, privateKey: privateKey, buildInfo: info, logger: logger, executionMode: executionMode,
		inventory: inventory, installer: installer, gogInstaller: gogInstaller,
		launcher: NewWindowsGameLauncher(), emulator: emulator, emulatorSetup: emulatorSetup, validator: validator,
	}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	delay := time.Second
	for {
		a.logger.Printf("connecting to %s", a.config.ServerURL)
		if err := a.runConnection(ctx); errors.Is(err, errStopRequested) {
			a.logger.Printf("stop requested by MGA Server")
			return nil
		} else if err != nil && ctx.Err() == nil {
			a.logger.Printf("connection failed: %v; retrying in %s", err, delay)
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
			if delay < 30*time.Second {
				delay *= 2
			}
			continue
		}
		if ctx.Err() != nil {
			return nil
		}
		delay = time.Second
	}
}

func (a *Agent) runConnection(ctx context.Context) error {
	connection, _, err := websocket.Dial(ctx, a.config.WebSocketURL, nil)
	if err != nil {
		return fmt.Errorf("connect to MGA Server: %w", err)
	}
	defer connection.CloseNow()
	writer := &deviceWriter{connection: connection}
	metadata, err := localMetadata(a.config.DisplayName, a.executionMode)
	if err != nil {
		return err
	}
	hello := devicev1.Hello{
		EndpointID:       a.config.EndpointID,
		ClientInstanceID: a.config.ClientInstanceID,
		ClientVersion:    a.buildInfo.Version,
		Versions:         devicev1.SupportedVersionRange(),
		Metadata:         metadata,
	}
	if err := writer.WriteMessage(ctx, devicev1.MessageHello, uuid.NewString(), "", hello); err != nil {
		return err
	}
	challengeEnvelope, err := readEnvelope(ctx, connection)
	if err != nil || challengeEnvelope.Type != devicev1.MessageAuthChallenge {
		return errors.New("server did not provide an authentication challenge")
	}
	challenge, err := devicev1.DecodePayload[devicev1.AuthChallenge](challengeEnvelope)
	if err != nil {
		return err
	}
	signingBytes, err := challenge.SigningBytes(a.config.EndpointID)
	if err != nil {
		return err
	}
	response := devicev1.AuthResponse{
		EndpointID:   a.config.EndpointID,
		ConnectionID: challenge.ConnectionID,
		Signature:    base64.RawURLEncoding.EncodeToString(ed25519.Sign(a.privateKey, signingBytes)),
	}
	if err := writer.WriteMessage(ctx, devicev1.MessageAuthResponse, uuid.NewString(), challengeEnvelope.MessageID, response); err != nil {
		return err
	}
	acceptedEnvelope, err := readEnvelope(ctx, connection)
	if err != nil || acceptedEnvelope.Type != devicev1.MessageConnectionAccepted {
		return errors.New("server did not accept the endpoint connection")
	}
	accepted, err := devicev1.DecodePayload[devicev1.ConnectionAccepted](acceptedEnvelope)
	if err != nil || accepted.Validate() != nil {
		return errors.New("server returned invalid connection policy")
	}
	a.logger.Printf("connected to MGA Server as endpoint %s", a.config.EndpointID)
	connectedContext, cancel := context.WithCancel(ctx)
	defer cancel()
	errorsChannel := make(chan error, 3)
	go func() {
		errorsChannel <- a.heartbeatLoop(connectedContext, writer, time.Duration(accepted.HeartbeatSeconds)*time.Second)
	}()
	go func() { errorsChannel <- a.readLoop(connectedContext, connection, writer) }()
	go func() { errorsChannel <- a.inventoryLoop(connectedContext, writer, inventoryRefreshInterval) }()
	select {
	case <-ctx.Done():
		return nil
	case err := <-errorsChannel:
		return err
	}
}

func (a *Agent) inventoryLoop(ctx context.Context, writer *deviceWriter, interval time.Duration) error {
	if a.inventory == nil {
		return errors.New("device inventory collector is unavailable")
	}
	if interval < time.Minute {
		return errors.New("inventory refresh interval must be at least one minute")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		inventory, err := a.inventory.Collect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			a.logger.Printf("device inventory refresh failed: %v", err)
		} else if err := writer.WriteMessage(ctx, devicev1.MessageInventoryReport, uuid.NewString(), "", inventory); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (a *Agent) heartbeatLoop(ctx context.Context, writer *deviceWriter, interval time.Duration) error {
	if interval < 5*time.Second {
		return errors.New("invalid heartbeat interval")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var sequence uint64
	for {
		sequence++
		state := devicev1.EndpointReady
		if a.active.Load() > 0 {
			state = devicev1.EndpointBusy
		}
		heartbeat := devicev1.Heartbeat{
			Sequence:           sequence,
			State:              state,
			ClientVersion:      a.buildInfo.Version,
			ActiveCommandCount: uint16(a.active.Load()),
		}
		if err := writer.WriteMessage(ctx, devicev1.MessageHeartbeat, uuid.NewString(), "", heartbeat); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (a *Agent) readLoop(ctx context.Context, connection *websocket.Conn, writer *deviceWriter) error {
	for {
		envelope, err := readEnvelope(ctx, connection)
		if err != nil {
			return err
		}
		if envelope.Type != devicev1.MessageCommandRequest {
			return fmt.Errorf("unexpected server message %s", envelope.Type)
		}
		request, err := devicev1.DecodePayload[devicev1.CommandRequest](envelope)
		if err != nil {
			return err
		}
		a.active.Add(1)
		if err := a.handleCommand(ctx, writer, request, envelope.MessageID); err != nil {
			a.active.Add(-1)
			return err
		}
		a.active.Add(-1)
	}
}

func (a *Agent) handleCommand(ctx context.Context, writer *deviceWriter, request devicev1.CommandRequest, correlationID string) error {
	if err := request.ValidateAt(time.Now()); err != nil {
		result := devicev1.CommandResult{
			CommandID: request.CommandID,
			Status:    devicev1.CommandRejected,
			Error:     &devicev1.ProtocolError{Code: "invalid_command", Message: err.Error()},
		}
		return writer.WriteMessage(ctx, devicev1.MessageCommandRejected, uuid.NewString(), correlationID, result)
	}
	accepted := devicev1.CommandStatusUpdate{CommandID: request.CommandID}
	if err := writer.WriteMessage(ctx, devicev1.MessageCommandAccepted, uuid.NewString(), correlationID, accepted); err != nil {
		return err
	}
	var progressSequence uint64
	report := func(update CommandProgressUpdate) error {
		progressSequence++
		value := update.Percent
		progress := devicev1.CommandProgress{CommandID: request.CommandID, Sequence: progressSequence, Phase: update.Phase, Message: update.Message, Percent: &value, Stage: update.Stage}
		if update.Stage != "" {
			stageValue := update.StagePercent
			progress.StagePercent = &stageValue
		}
		return writer.WriteMessage(ctx, devicev1.MessageCommandProgress, uuid.NewString(), correlationID, progress)
	}
	if err := report(CommandProgressUpdate{Phase: "executing", Message: "Starting", Percent: 0}); err != nil {
		return err
	}
	payload, stopAgent, errorCode, commandErr := a.executeEndpointCommand(ctx, request.CommandID, request.Name, request.Payload, report)
	if commandErr != nil {
		return a.writeFailedResult(ctx, writer, request.CommandID, correlationID, errorCode, commandErr, payload)
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	result := devicev1.CommandResult{CommandID: request.CommandID, Status: devicev1.CommandSucceeded, Payload: rawPayload}
	if err := writer.WriteMessage(ctx, devicev1.MessageCommandResult, uuid.NewString(), correlationID, result); err != nil {
		return err
	}
	if stopAgent {
		return errStopRequested
	}
	return nil
}

func (a *Agent) executeEndpointCommand(ctx context.Context, commandID, name string, rawPayload json.RawMessage, report CommandProgressReporter) (payload any, stopAgent bool, errorCode string, err error) {
	switch name {
	case devicev1.CapabilityEndpointPing:
		return map[string]any{"pong": true, "time": time.Now().UTC()}, false, "", nil
	case devicev1.CapabilityEndpointRefresh:
		metadata, err := localMetadata(a.config.DisplayName, a.executionMode)
		if err != nil {
			return nil, false, "metadata_failed", err
		}
		return metadata, false, "", nil
	case devicev1.CapabilityEndpointStop:
		return map[string]any{"stopping": true}, true, "", nil
	case devicev1.CapabilityInventoryRefresh:
		if a.inventory == nil {
			return nil, false, "inventory_unavailable", errors.New("device inventory collector is unavailable")
		}
		var requestPayload map[string]json.RawMessage
		if err := json.Unmarshal(rawPayload, &requestPayload); err != nil || len(requestPayload) != 0 {
			return nil, false, "invalid_payload", errors.New("inventory.refresh payload must be an empty object")
		}
		inventory, err := a.inventory.Collect(ctx)
		if err != nil {
			return nil, false, "inventory_failed", err
		}
		return inventory, false, "", nil
	case devicev1.CapabilityInstallationPreflight:
		var request devicev1.InstallationPreflightRequest
		if err := json.Unmarshal(rawPayload, &request); err != nil {
			return nil, false, "invalid_payload", err
		}
		evaluator := NewInstallationPreflightEvaluator(a.inventory)
		result, err := evaluator.Evaluate(ctx, request)
		if err != nil {
			return nil, false, "preflight_failed", err
		}
		return result, false, "", nil
	case devicev1.CapabilityGameInstallArchive:
		if a.installer == nil {
			return nil, false, "installer_unavailable", errors.New("archive installer is unavailable")
		}
		var request devicev1.ArchiveInstallRequest
		if err := json.Unmarshal(rawPayload, &request); err != nil {
			return nil, false, "invalid_payload", err
		}
		result, err := a.installer.Install(ctx, commandID, request, report)
		if err != nil {
			return nil, false, "install_failed", err
		}
		return result, false, "", nil
	case devicev1.CapabilityGameUninstall:
		if a.installer == nil {
			return nil, false, "installer_unavailable", errors.New("archive installer is unavailable")
		}
		var request devicev1.GameUninstallRequest
		if err := json.Unmarshal(rawPayload, &request); err != nil {
			return nil, false, "invalid_payload", err
		}
		result, err := a.installer.Uninstall(ctx, request, report)
		if err != nil {
			return nil, false, "uninstall_failed", err
		}
		return result, false, "", nil
	case devicev1.CapabilityGameInstallGogInno:
		if a.gogInstaller == nil {
			return nil, false, "unsupported_installer", errors.New("GOG Inno installer is unavailable")
		}
		var request devicev1.GogInnoInstallRequest
		if err := json.Unmarshal(rawPayload, &request); err != nil {
			return nil, false, "invalid_payload", err
		}
		result, err := a.gogInstaller.Install(ctx, commandID, request, report)
		if err != nil {
			code, payload := gogCommandFailure(err, "install_failed")
			return payload, false, code, err
		}
		return result, false, "", nil
	case devicev1.CapabilityGameUninstallGogInno:
		if a.gogInstaller == nil {
			return nil, false, "unsupported_installer", errors.New("GOG Inno installer is unavailable")
		}
		var request devicev1.GogInnoUninstallRequest
		if err := json.Unmarshal(rawPayload, &request); err != nil {
			return nil, false, "invalid_payload", err
		}
		result, err := a.gogInstaller.Uninstall(ctx, request, report)
		if err != nil {
			code, payload := gogCommandFailure(err, "uninstall_failed")
			return payload, false, code, err
		}
		return result, false, "", nil
	case devicev1.CapabilityGameCleanupGogInnoFailed:
		if a.gogInstaller == nil {
			return nil, false, "unsupported_installer", errors.New("GOG Inno installer is unavailable")
		}
		var request devicev1.GogInnoFailedCleanupRequest
		if err := json.Unmarshal(rawPayload, &request); err != nil {
			return nil, false, "invalid_payload", err
		}
		result, err := a.gogInstaller.CleanupFailed(ctx, request, report)
		if err != nil {
			code, payload := gogCommandFailure(err, "cleanup_failed")
			return payload, false, code, err
		}
		return result, false, "", nil
	case devicev1.CapabilityGameLaunch:
		if a.launcher == nil {
			return nil, false, "launcher_unavailable", errors.New("game launcher is unavailable")
		}
		var request devicev1.GameLaunchRequest
		if err := json.Unmarshal(rawPayload, &request); err != nil {
			return nil, false, "invalid_payload", err
		}
		result, err := a.launcher.Launch(ctx, request)
		if err != nil {
			return nil, false, "launch_failed", err
		}
		return result, false, "", nil
	case devicev1.CapabilityGameLaunchEmulator:
		if a.emulator == nil {
			return nil, false, "emulator_launcher_unavailable", errors.New("emulator launcher is unavailable")
		}
		var request devicev1.EmulatorLaunchRequest
		if err := json.Unmarshal(rawPayload, &request); err != nil {
			return nil, false, "invalid_payload", err
		}
		result, err := a.emulator.Launch(ctx, commandID, request, report)
		if err != nil {
			return nil, false, "emulator_launch_failed", err
		}
		return result, false, "", nil
	case devicev1.CapabilityEmulatorSetup:
		if a.emulatorSetup == nil {
			return nil, false, "emulator_setup_unavailable", errors.New("emulator setup manager is unavailable")
		}
		var request devicev1.EmulatorSetupRequest
		if err := json.Unmarshal(rawPayload, &request); err != nil {
			return nil, false, "invalid_payload", err
		}
		result, err := a.emulatorSetup.Setup(ctx, request, report)
		if err != nil {
			return nil, false, "emulator_setup_failed", err
		}
		return result, false, "", nil
	case devicev1.CapabilityGameValidateInstallations:
		if a.validator == nil {
			return nil, false, "validator_unavailable", errors.New("installation validator is unavailable")
		}
		var request devicev1.InstallationValidationRequest
		if err := json.Unmarshal(rawPayload, &request); err != nil {
			return nil, false, "invalid_payload", err
		}
		result, err := a.validator.Validate(ctx, request, report)
		if err != nil {
			return nil, false, "validation_failed", err
		}
		return result, false, "", nil
	default:
		return nil, false, "unsupported_command", fmt.Errorf("unsupported command %s", name)
	}
}

func (a *Agent) writeFailedResult(ctx context.Context, writer *deviceWriter, commandID, correlationID, code string, commandErr error, payload any) error {
	result := devicev1.CommandResult{
		CommandID: commandID,
		Status:    devicev1.CommandFailed,
		Error:     &devicev1.ProtocolError{Code: code, Message: commandErr.Error()},
	}
	if payload != nil {
		rawPayload, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		result.Payload = rawPayload
	}
	return writer.WriteMessage(ctx, devicev1.MessageCommandResult, uuid.NewString(), correlationID, result)
}

func gogCommandFailure(err error, fallbackCode string) (string, any) {
	var commandError *GogInnoCommandError
	if errors.As(err, &commandError) {
		if commandError.Payload != nil {
			return commandError.Code, commandError.Payload
		}
		return commandError.Code, nil
	}
	return fallbackCode, nil
}

type deviceWriter struct {
	connection *websocket.Conn
	mu         sync.Mutex
}

func (w *deviceWriter) WriteMessage(ctx context.Context, messageType devicev1.MessageType, messageID, correlationID string, payload any) error {
	envelope, err := devicev1.NewEnvelope(messageType, messageID, correlationID, time.Now().UTC(), payload)
	if err != nil {
		return err
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.connection.Write(ctx, websocket.MessageText, data)
}

func readEnvelope(ctx context.Context, connection *websocket.Conn) (devicev1.Envelope, error) {
	messageType, data, err := connection.Read(ctx)
	if err != nil {
		return devicev1.Envelope{}, err
	}
	if messageType != websocket.MessageText {
		return devicev1.Envelope{}, errors.New("device messages must be JSON text")
	}
	return devicev1.DecodeEnvelope(data)
}
