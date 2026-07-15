package v1

import (
	"encoding/json"
	"testing"
	"time"
)

func TestValidateTransition(t *testing.T) {
	t.Parallel()

	valid := [][2]CommandStatus{
		{CommandAuthorized, CommandDispatched},
		{CommandDispatched, CommandAccepted},
		{CommandAccepted, CommandRunning},
		{CommandRunning, CommandSucceeded},
	}
	for _, transition := range valid {
		if err := ValidateTransition(transition[0], transition[1]); err != nil {
			t.Fatalf("ValidateTransition(%s, %s) error = %v", transition[0], transition[1], err)
		}
	}
	if err := ValidateTransition(CommandSucceeded, CommandRunning); err == nil {
		t.Fatal("terminal transition error = nil, want error")
	}
}

func TestCommandRequestValidateAt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	request := validCommandRequest(now)
	if err := request.ValidateAt(now); err != nil {
		t.Fatalf("ValidateAt() error = %v", err)
	}
}

func TestCommandRequestRejectsInsufficientAccess(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	request := validCommandRequest(now)
	request.Authorization.GrantedLevel = AccessPlay
	request.RequiredLevel = AccessManage
	if err := request.ValidateAt(now); err == nil {
		t.Fatal("ValidateAt() error = nil, want insufficient access error")
	}
}

func TestCommandRequestRejectsExpiredCommand(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	request := validCommandRequest(now)
	if err := request.ValidateAt(request.ExpiresAt); err == nil {
		t.Fatal("ValidateAt() error = nil, want expiry error")
	}
}

func TestCommandRequestRejectsMalformedFields(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*CommandRequest)
	}{
		{name: "missing command id", mutate: func(r *CommandRequest) { r.CommandID = "" }},
		{name: "missing idempotency key", mutate: func(r *CommandRequest) { r.IdempotencyKey = "" }},
		{name: "untyped command name", mutate: func(r *CommandRequest) { r.Name = "install" }},
		{name: "zero schema", mutate: func(r *CommandRequest) { r.SchemaVersion = 0 }},
		{name: "missing profile", mutate: func(r *CommandRequest) { r.Authorization.ProfileID = "" }},
		{name: "missing created at", mutate: func(r *CommandRequest) { r.CreatedAt = time.Time{} }},
		{name: "missing expires at", mutate: func(r *CommandRequest) { r.ExpiresAt = time.Time{} }},
		{name: "inverted expiry", mutate: func(r *CommandRequest) { r.ExpiresAt = r.CreatedAt }},
		{name: "missing validation time", mutate: func(_ *CommandRequest) {}},
		{name: "missing payload", mutate: func(r *CommandRequest) { r.Payload = nil }},
		{name: "array payload", mutate: func(r *CommandRequest) { r.Payload = json.RawMessage(`[]`) }},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			request := validCommandRequest(now)
			tt.mutate(&request)
			validationTime := now
			if tt.name == "missing validation time" {
				validationTime = time.Time{}
			}
			if err := request.ValidateAt(validationTime); err == nil {
				t.Fatal("ValidateAt() error = nil, want error")
			}
		})
	}
}

func TestCommandProgressValidate(t *testing.T) {
	t.Parallel()

	percent := uint8(100)
	valid := CommandProgress{CommandID: "command-1", Sequence: 1, Phase: "download", Percent: &percent}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	overflow := uint8(101)
	tests := []CommandProgress{
		{Sequence: 1, Phase: "download"},
		{CommandID: "command-1", Phase: "download"},
		{CommandID: "command-1", Sequence: 1},
		{CommandID: "command-1", Sequence: 1, Phase: "download", Percent: &overflow},
	}
	for _, progress := range tests {
		if err := progress.Validate(); err == nil {
			t.Fatalf("CommandProgress.Validate(%+v) error = nil, want error", progress)
		}
	}
}

func TestCommandProgressRequiresStageForStagePercent(t *testing.T) {
	percent := uint8(50)
	progress := CommandProgress{CommandID: "command-1", Sequence: 1, Phase: "downloading", StagePercent: &percent}
	if err := progress.Validate(); err == nil {
		t.Fatal("Validate() accepted stage_percent without stage")
	}
	progress.Stage = "download"
	if err := progress.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestCommandResultRequiresErrorForFailure(t *testing.T) {
	t.Parallel()

	result := CommandResult{CommandID: "command-1", Status: CommandFailed}
	if err := result.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing protocol error")
	}
	result.Error = &ProtocolError{Code: "install_failed", Message: "installation failed"}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestCommandResultValidateConsistency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result CommandResult
	}{
		{name: "missing command id", result: CommandResult{Status: CommandSucceeded}},
		{name: "non-terminal status", result: CommandResult{CommandID: "command-1", Status: CommandRunning}},
		{name: "error on success", result: CommandResult{CommandID: "command-1", Status: CommandSucceeded, Error: &ProtocolError{Code: "bad", Message: "bad"}}},
		{name: "invalid payload", result: CommandResult{CommandID: "command-1", Status: CommandSucceeded, Payload: json.RawMessage(`{`)}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.result.Validate(); err == nil {
				t.Fatal("CommandResult.Validate() error = nil, want error")
			}
		})
	}

	result := CommandResult{CommandID: "command-1", Status: CommandSucceeded, Payload: json.RawMessage(`{"installed":true}`)}
	if err := result.Validate(); err != nil {
		t.Fatalf("successful CommandResult.Validate() error = %v", err)
	}
}

func validCommandRequest(now time.Time) CommandRequest {
	return CommandRequest{
		CommandID:      "command-1",
		IdempotencyKey: "idempotency-1",
		Name:           "game.install",
		SchemaVersion:  1,
		RequiredLevel:  AccessManage,
		Authorization: AuthorizationContext{
			ProfileID:    "profile-1",
			GrantedLevel: AccessOwner,
		},
		CreatedAt: now.Add(-time.Second),
		ExpiresAt: now.Add(time.Minute),
		Payload:   json.RawMessage(`{"game_id":"game-1"}`),
	}
}
