package v1

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const CommandLedgerSchemaVersion uint16 = 1

const (
	CommandReplayRecorded        = "recorded"
	CommandReplayAlreadyRecorded = "already_recorded"
	CommandReplayUnknown         = "unknown"
	CommandReplayConflict        = "conflict"
)

var sha256FingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// CommandRequestFingerprint identifies the semantic operation without its
// transport ID, idempotency key, or delivery timestamps.
func CommandRequestFingerprint(request CommandRequest) (string, error) {
	payload, err := canonicalJSON(request.Payload)
	if err != nil {
		return "", fmt.Errorf("canonicalize command payload: %w", err)
	}
	canonical := struct {
		Name            string               `json:"name"`
		SchemaVersion   uint16               `json:"schema_version"`
		RequiredLevel   AccessLevel          `json:"required_level"`
		Authorization   AuthorizationContext `json:"authorization"`
		AllowUserAction bool                 `json:"allow_user_action"`
		Payload         json.RawMessage      `json:"payload"`
	}{
		Name: request.Name, SchemaVersion: request.SchemaVersion,
		RequiredLevel: request.RequiredLevel, Authorization: request.Authorization,
		AllowUserAction: request.AllowUserAction, Payload: payload,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// CommandResultFingerprint allows the server to recognize an exact replay
// without applying command-specific side effects twice.
func CommandResultFingerprint(result CommandResult) (string, error) {
	if err := result.Validate(); err != nil {
		return "", err
	}
	var payload json.RawMessage
	if len(result.Payload) > 0 {
		var err error
		payload, err = canonicalJSON(result.Payload)
		if err != nil {
			return "", err
		}
	}
	canonical := struct {
		Status  CommandStatus   `json:"status"`
		Payload json.RawMessage `json:"payload,omitempty"`
		Error   *ProtocolError  `json:"error,omitempty"`
	}{Status: result.Status, Payload: payload, Error: result.Error}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func canonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

// CommandRequiresDurableReplay fails safe for future commands: only an
// explicit read-only allow-list bypasses the mutation ledger.
func CommandRequiresDurableReplay(name string) bool {
	switch name {
	case CapabilityEndpointPing, CapabilityEndpointRefresh, CapabilityInventoryRefresh,
		CapabilityInstallationPreflight, CapabilityGameValidateInstallations:
		return false
	default:
		return true
	}
}

type CommandReplayAck struct {
	CommandID   string `json:"command_id"`
	Disposition string `json:"disposition"`
	Message     string `json:"message,omitempty"`
}

func (a CommandReplayAck) Validate() error {
	if strings.TrimSpace(a.CommandID) == "" {
		return errors.New("command_id is required")
	}
	switch a.Disposition {
	case CommandReplayRecorded, CommandReplayAlreadyRecorded, CommandReplayUnknown, CommandReplayConflict:
	default:
		return fmt.Errorf("unsupported replay disposition %q", a.Disposition)
	}
	return nil
}

type CommandReplayComplete struct {
	LedgerSchemaVersion uint16    `json:"ledger_schema_version"`
	Replayed            uint16    `json:"replayed"`
	CompletedAt         time.Time `json:"completed_at"`
}

func (c CommandReplayComplete) Validate() error {
	if c.LedgerSchemaVersion != CommandLedgerSchemaVersion {
		return fmt.Errorf("unsupported command ledger schema %d", c.LedgerSchemaVersion)
	}
	if c.CompletedAt.IsZero() {
		return errors.New("completed_at is required")
	}
	return nil
}
