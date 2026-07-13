package clientapp

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/buildinfo"
	clientconfig "github.com/GreenFuze/MyGamesAnywhere/client/internal/config"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/coder/websocket"
	"github.com/google/uuid"
)

type Agent struct {
	config     clientconfig.Config
	privateKey ed25519.PrivateKey
	buildInfo  buildinfo.Info
	active     atomic.Int32
}

func NewAgent(config clientconfig.Config, privateKey ed25519.PrivateKey, info buildinfo.Info) (*Agent, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("valid endpoint private key is required")
	}
	return &Agent{config: config, privateKey: privateKey, buildInfo: info}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	delay := time.Second
	for {
		if err := a.runConnection(ctx); err != nil && ctx.Err() == nil {
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
	metadata, err := localMetadata(a.config.DisplayName)
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
	connectedContext, cancel := context.WithCancel(ctx)
	defer cancel()
	errorsChannel := make(chan error, 2)
	go func() {
		errorsChannel <- a.heartbeatLoop(connectedContext, writer, time.Duration(accepted.HeartbeatSeconds)*time.Second)
	}()
	go func() { errorsChannel <- a.readLoop(connectedContext, connection, writer) }()
	select {
	case <-ctx.Done():
		return nil
	case err := <-errorsChannel:
		return err
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
	progress := devicev1.CommandProgress{CommandID: request.CommandID, Sequence: 1, Phase: "executing"}
	if err := writer.WriteMessage(ctx, devicev1.MessageCommandProgress, uuid.NewString(), correlationID, progress); err != nil {
		return err
	}
	var payload any
	switch request.Name {
	case devicev1.CapabilityEndpointPing:
		payload = map[string]any{"pong": true, "time": time.Now().UTC()}
	case devicev1.CapabilityEndpointRefresh:
		metadata, err := localMetadata(a.config.DisplayName)
		if err != nil {
			return a.writeFailedResult(ctx, writer, request.CommandID, correlationID, "metadata_failed", err)
		}
		payload = metadata
	default:
		return a.writeFailedResult(ctx, writer, request.CommandID, correlationID, "unsupported_command", fmt.Errorf("unsupported command %s", request.Name))
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	result := devicev1.CommandResult{CommandID: request.CommandID, Status: devicev1.CommandSucceeded, Payload: rawPayload}
	return writer.WriteMessage(ctx, devicev1.MessageCommandResult, uuid.NewString(), correlationID, result)
}

func (a *Agent) writeFailedResult(ctx context.Context, writer *deviceWriter, commandID, correlationID, code string, commandErr error) error {
	result := devicev1.CommandResult{
		CommandID: commandID,
		Status:    devicev1.CommandFailed,
		Error:     &devicev1.ProtocolError{Code: code, Message: commandErr.Error()},
	}
	return writer.WriteMessage(ctx, devicev1.MessageCommandResult, uuid.NewString(), correlationID, result)
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
