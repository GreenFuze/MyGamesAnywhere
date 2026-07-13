package v1

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type helloPayload struct {
	EndpointID string       `json:"endpoint_id"`
	Versions   VersionRange `json:"versions"`
}

func TestEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	envelope, err := NewEnvelope(MessageHello, "message-1", "", sentAt, helloPayload{
		EndpointID: "endpoint-1",
		Versions:   SupportedVersionRange(),
	})
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}
	payload, err := DecodePayload[helloPayload](envelope)
	if err != nil {
		t.Fatalf("DecodePayload() error = %v", err)
	}
	if payload.EndpointID != "endpoint-1" {
		t.Fatalf("EndpointID = %q, want endpoint-1", payload.EndpointID)
	}

	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	decoded, err := DecodeEnvelope(encoded)
	if err != nil {
		t.Fatalf("DecodeEnvelope() error = %v", err)
	}
	if decoded.MessageID != envelope.MessageID {
		t.Fatalf("decoded MessageID = %q, want %q", decoded.MessageID, envelope.MessageID)
	}
}

func TestNewEnvelopeRejectsUnmarshalablePayload(t *testing.T) {
	t.Parallel()

	_, err := NewEnvelope(MessageHeartbeat, "message-1", "", time.Now().UTC(), make(chan int))
	if err == nil {
		t.Fatal("NewEnvelope() error = nil, want marshal error")
	}
}

func TestEnvelopeValidateRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	valid := Envelope{
		ProtocolVersion: Version,
		Type:            MessageHeartbeat,
		MessageID:       "message-1",
		SentAt:          time.Now().UTC(),
		Payload:         []byte(`{}`),
	}
	tests := []struct {
		name   string
		mutate func(*Envelope)
	}{
		{name: "unsupported version", mutate: func(e *Envelope) { e.ProtocolVersion = 2 }},
		{name: "unknown type", mutate: func(e *Envelope) { e.Type = "unknown" }},
		{name: "missing message id", mutate: func(e *Envelope) { e.MessageID = " " }},
		{name: "missing sent at", mutate: func(e *Envelope) { e.SentAt = time.Time{} }},
		{name: "missing payload", mutate: func(e *Envelope) { e.Payload = nil }},
		{name: "null payload", mutate: func(e *Envelope) { e.Payload = []byte(`null`) }},
		{name: "invalid payload", mutate: func(e *Envelope) { e.Payload = []byte(`{`) }},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			tt.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("Envelope.Validate() error = nil, want error")
			}
		})
	}
}

func TestDecodeEnvelopeRejectsUnknownField(t *testing.T) {
	t.Parallel()

	data := `{"protocol_version":1,"type":"heartbeat","message_id":"message-1","sent_at":"2026-07-13T12:00:00Z","payload":{},"unexpected":true}`
	_, err := DecodeEnvelope([]byte(data))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("DecodeEnvelope() error = %v, want unknown field error", err)
	}
}

func TestDecodeEnvelopeRejectsTrailingValue(t *testing.T) {
	t.Parallel()

	data := `{"protocol_version":1,"type":"heartbeat","message_id":"message-1","sent_at":"2026-07-13T12:00:00Z","payload":{}} {}`
	_, err := DecodeEnvelope([]byte(data))
	if err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("DecodeEnvelope() error = %v, want multiple JSON values error", err)
	}
}

func TestDecodePayloadRejectsUnknownField(t *testing.T) {
	t.Parallel()

	envelope := Envelope{
		ProtocolVersion: Version,
		Type:            MessageHello,
		MessageID:       "message-1",
		SentAt:          time.Now().UTC(),
		Payload:         []byte(`{"endpoint_id":"endpoint-1","versions":{"min":1,"max":1},"unexpected":true}`),
	}
	_, err := DecodePayload[helloPayload](envelope)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("DecodePayload() error = %v, want unknown field error", err)
	}
}

func TestDecodePayloadRejectsInvalidEnvelope(t *testing.T) {
	t.Parallel()

	_, err := DecodePayload[helloPayload](Envelope{})
	if err == nil || !strings.Contains(err.Error(), "validate envelope") {
		t.Fatalf("DecodePayload() error = %v, want envelope validation error", err)
	}
}
