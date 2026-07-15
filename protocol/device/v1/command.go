package v1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// CommandStatus is one state in the server-observed command lifecycle.
type CommandStatus string

const (
	CommandAuthorized CommandStatus = "authorized"
	CommandDispatched CommandStatus = "dispatched"
	CommandAccepted   CommandStatus = "accepted"
	CommandRunning    CommandStatus = "running"
	CommandSucceeded  CommandStatus = "succeeded"
	CommandFailed     CommandStatus = "failed"
	CommandRejected   CommandStatus = "rejected"
	CommandCanceled   CommandStatus = "canceled"
	CommandExpired    CommandStatus = "expired"
)

var commandTransitions = map[CommandStatus]map[CommandStatus]struct{}{
	CommandAuthorized: {
		CommandDispatched: {},
		CommandCanceled:   {},
		CommandExpired:    {},
	},
	CommandDispatched: {
		CommandAccepted: {},
		CommandRejected: {},
		CommandCanceled: {},
		CommandExpired:  {},
	},
	CommandAccepted: {
		CommandRunning:  {},
		CommandCanceled: {},
		CommandExpired:  {},
	},
	CommandRunning: {
		CommandSucceeded: {},
		CommandFailed:    {},
		CommandCanceled:  {},
		CommandExpired:   {},
	},
	CommandSucceeded: {},
	CommandFailed:    {},
	CommandRejected:  {},
	CommandCanceled:  {},
	CommandExpired:   {},
}

var commandNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)+$`)

// Validate rejects unknown command statuses.
func (s CommandStatus) Validate() error {
	if _, ok := commandTransitions[s]; !ok {
		return fmt.Errorf("unknown command status %q", s)
	}
	return nil
}

// IsTerminal reports whether no further lifecycle transition is allowed.
func (s CommandStatus) IsTerminal() (bool, error) {
	if err := s.Validate(); err != nil {
		return false, err
	}
	return len(commandTransitions[s]) == 0, nil
}

// ValidateTransition rejects lifecycle transitions that could hide duplicated
// or out-of-order execution.
func ValidateTransition(from, to CommandStatus) error {
	if err := from.Validate(); err != nil {
		return fmt.Errorf("validate source status: %w", err)
	}
	if err := to.Validate(); err != nil {
		return fmt.Errorf("validate target status: %w", err)
	}
	if _, ok := commandTransitions[from][to]; !ok {
		return fmt.Errorf("invalid command transition %s -> %s", from, to)
	}
	return nil
}

// AuthorizationContext is the server-validated grant attached to a command.
type AuthorizationContext struct {
	ProfileID    string      `json:"profile_id"`
	GrantedLevel AccessLevel `json:"granted_level"`
}

// CommandRequest is the typed payload of command.request.
type CommandRequest struct {
	CommandID       string               `json:"command_id"`
	IdempotencyKey  string               `json:"idempotency_key"`
	Name            string               `json:"name"`
	SchemaVersion   uint16               `json:"schema_version"`
	RequiredLevel   AccessLevel          `json:"required_level"`
	Authorization   AuthorizationContext `json:"authorization"`
	CreatedAt       time.Time            `json:"created_at"`
	ExpiresAt       time.Time            `json:"expires_at"`
	AllowUserAction bool                 `json:"allow_user_action"`
	Payload         json.RawMessage      `json:"payload"`
}

// ValidateAt validates command structure, authority, and expiry against now.
func (r CommandRequest) ValidateAt(now time.Time) error {
	if strings.TrimSpace(r.CommandID) == "" {
		return errors.New("command_id is required")
	}
	if strings.TrimSpace(r.IdempotencyKey) == "" {
		return errors.New("idempotency_key is required")
	}
	if !commandNamePattern.MatchString(r.Name) {
		return fmt.Errorf("invalid typed command name %q", r.Name)
	}
	if r.SchemaVersion == 0 {
		return errors.New("schema_version must be greater than zero")
	}
	if err := r.RequiredLevel.Validate(); err != nil {
		return fmt.Errorf("validate required access: %w", err)
	}
	if strings.TrimSpace(r.Authorization.ProfileID) == "" {
		return errors.New("authorization profile_id is required")
	}
	allowed, err := r.Authorization.GrantedLevel.Allows(r.RequiredLevel)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("granted access %s does not allow required access %s", r.Authorization.GrantedLevel, r.RequiredLevel)
	}
	if r.CreatedAt.IsZero() {
		return errors.New("created_at is required")
	}
	if r.ExpiresAt.IsZero() {
		return errors.New("expires_at is required")
	}
	if !r.ExpiresAt.After(r.CreatedAt) {
		return errors.New("expires_at must be after created_at")
	}
	if now.IsZero() {
		return errors.New("validation time is required")
	}
	if !now.Before(r.ExpiresAt) {
		return fmt.Errorf("command expired at %s", r.ExpiresAt.Format(time.RFC3339Nano))
	}
	payload := bytes.TrimSpace(r.Payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) {
		return errors.New("command payload is required")
	}
	if !json.Valid(payload) || payload[0] != '{' {
		return errors.New("command payload must be a JSON object")
	}
	return nil
}

// CommandProgress is the typed payload of command.progress.
type CommandProgress struct {
	CommandID    string `json:"command_id"`
	Sequence     uint64 `json:"sequence"`
	Phase        string `json:"phase"`
	Percent      *uint8 `json:"percent,omitempty"`
	Stage        string `json:"stage,omitempty"`
	StagePercent *uint8 `json:"stage_percent,omitempty"`
	Message      string `json:"message,omitempty"`
}

// CommandStatusUpdate identifies a command acknowledgement or cancellation.
type CommandStatusUpdate struct {
	CommandID string `json:"command_id"`
}

func (u CommandStatusUpdate) Validate() error {
	if strings.TrimSpace(u.CommandID) == "" {
		return errors.New("command_id is required")
	}
	return nil
}

// Validate rejects malformed or out-of-range progress reports.
func (p CommandProgress) Validate() error {
	if strings.TrimSpace(p.CommandID) == "" {
		return errors.New("command_id is required")
	}
	if p.Sequence == 0 {
		return errors.New("sequence must be greater than zero")
	}
	if strings.TrimSpace(p.Phase) == "" {
		return errors.New("phase is required")
	}
	if p.Percent != nil && *p.Percent > 100 {
		return fmt.Errorf("percent must be between 0 and 100, got %d", *p.Percent)
	}
	if p.StagePercent != nil {
		if strings.TrimSpace(p.Stage) == "" {
			return errors.New("stage is required when stage_percent is provided")
		}
		if *p.StagePercent > 100 {
			return fmt.Errorf("stage_percent must be between 0 and 100, got %d", *p.StagePercent)
		}
	}
	return nil
}

// ProtocolError is a sanitized, machine-readable failure.
type ProtocolError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// CommandResult is the typed payload of command.result.
type CommandResult struct {
	CommandID string          `json:"command_id"`
	Status    CommandStatus   `json:"status"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     *ProtocolError  `json:"error,omitempty"`
}

// Validate rejects non-terminal or internally inconsistent results.
func (r CommandResult) Validate() error {
	if strings.TrimSpace(r.CommandID) == "" {
		return errors.New("command_id is required")
	}
	terminal, err := r.Status.IsTerminal()
	if err != nil {
		return err
	}
	if !terminal {
		return fmt.Errorf("command result status %s is not terminal", r.Status)
	}
	if r.Status == CommandFailed || r.Status == CommandRejected {
		if r.Error == nil || strings.TrimSpace(r.Error.Code) == "" || strings.TrimSpace(r.Error.Message) == "" {
			return errors.New("failed or rejected result requires an error code and message")
		}
	} else if r.Error != nil {
		return fmt.Errorf("command result status %s must not include an error", r.Status)
	}
	if len(bytes.TrimSpace(r.Payload)) > 0 && !json.Valid(r.Payload) {
		return errors.New("result payload must contain valid JSON")
	}
	return nil
}
