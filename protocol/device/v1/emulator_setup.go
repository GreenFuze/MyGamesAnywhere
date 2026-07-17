package v1

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const EmulatorSetupSchemaVersion uint16 = 1

var emulatorSetupIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type EmulatorSetupRequest struct {
	EmulatorID string `json:"emulator_id"`
	Action     string `json:"action"`
}

func (r EmulatorSetupRequest) Validate() error {
	if r.EmulatorID != strings.TrimSpace(r.EmulatorID) || !emulatorSetupIDPattern.MatchString(r.EmulatorID) {
		return fmt.Errorf("invalid emulator_id %q", r.EmulatorID)
	}
	if r.Action != "install" && r.Action != "update" {
		return fmt.Errorf("unsupported emulator setup action %q", r.Action)
	}
	return nil
}

type EmulatorSetupResult struct {
	EmulatorID      string `json:"emulator_id"`
	Action          string `json:"action"`
	State           string `json:"state"`
	PreviousVersion string `json:"previous_version,omitempty"`
	CurrentVersion  string `json:"current_version,omitempty"`
	Changed         bool   `json:"changed"`
}

func (r EmulatorSetupResult) Validate() error {
	if err := (EmulatorSetupRequest{EmulatorID: r.EmulatorID, Action: r.Action}).Validate(); err != nil {
		return err
	}
	switch r.State {
	case "installed", "updated", "already_current", "completed":
	default:
		return errors.New("invalid emulator setup result state")
	}
	return nil
}
