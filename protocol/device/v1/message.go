package v1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// MessageType identifies the typed payload carried by an Envelope.
type MessageType string

const (
	MessageHello              MessageType = "hello"
	MessageAuthChallenge      MessageType = "auth.challenge"
	MessageAuthResponse       MessageType = "auth.response"
	MessageConnectionAccepted MessageType = "connection.accepted"
	MessageHeartbeat          MessageType = "heartbeat"
	MessageInventoryReport    MessageType = "inventory.report"
	MessageCommandRequest     MessageType = "command.request"
	MessageCommandAccepted    MessageType = "command.accepted"
	MessageCommandRejected    MessageType = "command.rejected"
	MessageCommandProgress    MessageType = "command.progress"
	MessageCommandResult      MessageType = "command.result"
	MessageCommandCancel      MessageType = "command.cancel"
	MessageProtocolError      MessageType = "protocol.error"
)

var knownMessageTypes = map[MessageType]struct{}{
	MessageHello:              {},
	MessageAuthChallenge:      {},
	MessageAuthResponse:       {},
	MessageConnectionAccepted: {},
	MessageHeartbeat:          {},
	MessageInventoryReport:    {},
	MessageCommandRequest:     {},
	MessageCommandAccepted:    {},
	MessageCommandRejected:    {},
	MessageCommandProgress:    {},
	MessageCommandResult:      {},
	MessageCommandCancel:      {},
	MessageProtocolError:      {},
}

// Envelope is the common JSON wrapper for every device protocol message.
type Envelope struct {
	ProtocolVersion ProtocolVersion `json:"protocol_version"`
	Type            MessageType     `json:"type"`
	MessageID       string          `json:"message_id"`
	CorrelationID   string          `json:"correlation_id,omitempty"`
	SentAt          time.Time       `json:"sent_at"`
	Payload         json.RawMessage `json:"payload"`
}

// NewEnvelope constructs and validates an envelope from a typed payload.
func NewEnvelope(messageType MessageType, messageID, correlationID string, sentAt time.Time, payload any) (Envelope, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal %s payload: %w", messageType, err)
	}
	envelope := Envelope{
		ProtocolVersion: Version,
		Type:            messageType,
		MessageID:       messageID,
		CorrelationID:   correlationID,
		SentAt:          sentAt,
		Payload:         rawPayload,
	}
	if err := envelope.Validate(); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// Validate rejects malformed or unsupported envelope data.
func (e Envelope) Validate() error {
	if e.ProtocolVersion != Version {
		return fmt.Errorf("unsupported protocol version %d", e.ProtocolVersion)
	}
	if _, ok := knownMessageTypes[e.Type]; !ok {
		return fmt.Errorf("unknown message type %q", e.Type)
	}
	if strings.TrimSpace(e.MessageID) == "" {
		return errors.New("message_id is required")
	}
	if e.SentAt.IsZero() {
		return errors.New("sent_at is required")
	}
	payload := bytes.TrimSpace(e.Payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) {
		return errors.New("payload is required")
	}
	if !json.Valid(payload) {
		return errors.New("payload must contain valid JSON")
	}
	return nil
}

// DecodeEnvelope strictly decodes one envelope and rejects unknown fields or
// trailing JSON values.
func DecodeEnvelope(data []byte) (Envelope, error) {
	var envelope Envelope
	if err := decodeStrict(data, &envelope); err != nil {
		return Envelope{}, fmt.Errorf("decode envelope: %w", err)
	}
	if err := envelope.Validate(); err != nil {
		return Envelope{}, fmt.Errorf("validate envelope: %w", err)
	}
	return envelope, nil
}

// DecodePayload strictly decodes an envelope payload into its typed form.
func DecodePayload[T any](envelope Envelope) (T, error) {
	var payload T
	if err := envelope.Validate(); err != nil {
		return payload, fmt.Errorf("validate envelope: %w", err)
	}
	if err := decodeStrict(envelope.Payload, &payload); err != nil {
		return payload, fmt.Errorf("decode %s payload: %w", envelope.Type, err)
	}
	return payload, nil
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return fmt.Errorf("read trailing JSON: %w", err)
	}
	return nil
}
