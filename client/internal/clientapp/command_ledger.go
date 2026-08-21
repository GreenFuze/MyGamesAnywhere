package clientapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/google/uuid"
)

const (
	commandLedgerSchemaVersion    = 1
	commandLedgerPolicyVersion    = 1
	commandLedgerRetention        = 30 * 24 * time.Hour
	commandLedgerReplaySafety     = 7 * 24 * time.Hour
	commandLedgerTargetPerBinding = 2048
	commandLedgerMaxAliases       = 8
)

var (
	ErrCommandIdempotencyConflict = errors.New("command idempotency identity conflicts with retained history")
	ErrCommandAlreadyRunning      = errors.New("command is already running")
)

type commandLedgerState string

const (
	commandLedgerRunning  commandLedgerState = "running"
	commandLedgerTerminal commandLedgerState = "terminal"
)

type commandLedgerOutcome struct {
	Status  devicev1.CommandStatus  `json:"status"`
	Payload json.RawMessage         `json:"payload,omitempty"`
	Error   *devicev1.ProtocolError `json:"error,omitempty"`
}

type commandLedgerRecord struct {
	BindingID          string                `json:"binding_id"`
	CommandIDs         []string              `json:"command_ids"`
	AcknowledgedIDs    []string              `json:"acknowledged_command_ids,omitempty"`
	IdempotencyKey     string                `json:"idempotency_key"`
	RequestFingerprint string                `json:"request_fingerprint"`
	Name               string                `json:"name"`
	CreatedAt          time.Time             `json:"created_at"`
	ExpiresAt          time.Time             `json:"expires_at"`
	State              commandLedgerState    `json:"state"`
	Outcome            *commandLedgerOutcome `json:"outcome,omitempty"`
	UpdatedAt          time.Time             `json:"updated_at"`
	CompletedAt        *time.Time            `json:"completed_at,omitempty"`
}

type commandLedgerDocument struct {
	SchemaVersion int                   `json:"schema_version"`
	PolicyVersion int                   `json:"policy_version"`
	Records       []commandLedgerRecord `json:"records"`
}

type CommandLedgerDecision struct {
	Execute bool
	Result  *devicev1.CommandResult
}

// CommandLedger is the per-OS-user durable at-most-once authority shared by
// every server binding agent in the tray process.
type CommandLedger struct {
	mu   sync.Mutex
	path string
	doc  commandLedgerDocument
}

func OpenCommandLedger(path string) (*CommandLedger, error) {
	return openCommandLedger(path, time.Now().UTC())
}

func openCommandLedger(path string, recoveredAt time.Time) (*CommandLedger, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("command ledger path is required")
	}
	ledger := &CommandLedger{path: path, doc: commandLedgerDocument{
		SchemaVersion: commandLedgerSchemaVersion, PolicyVersion: commandLedgerPolicyVersion,
		Records: []commandLedgerRecord{},
	}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ledger, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read command ledger: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ledger.doc); err != nil {
		return nil, fmt.Errorf("decode command ledger: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("command ledger contains trailing JSON")
	}
	if err := ledger.doc.validate(); err != nil {
		return nil, err
	}
	recovered := false
	for index := range ledger.doc.Records {
		record := &ledger.doc.Records[index]
		if record.State != commandLedgerRunning {
			continue
		}
		outcome := unknownCommandOutcome()
		record.State = commandLedgerTerminal
		record.Outcome = &outcome
		record.UpdatedAt = recoveredAt
		record.CompletedAt = timePointer(recoveredAt)
		recovered = true
	}
	ledger.pruneLocked(recoveredAt)
	if recovered {
		if err := ledger.saveLocked(); err != nil {
			return nil, fmt.Errorf("recover interrupted command ledger: %w", err)
		}
	}
	return ledger, nil
}

func (d commandLedgerDocument) validate() error {
	if d.SchemaVersion != commandLedgerSchemaVersion {
		return fmt.Errorf("unsupported command ledger schema %d", d.SchemaVersion)
	}
	if d.PolicyVersion != commandLedgerPolicyVersion {
		return fmt.Errorf("unsupported command ledger policy %d", d.PolicyVersion)
	}
	commands := map[string]bool{}
	idempotency := map[string]bool{}
	for index, record := range d.Records {
		if err := record.validate(); err != nil {
			return fmt.Errorf("command ledger record %d: %w", index, err)
		}
		bindingKey := strings.ToLower(record.BindingID)
		idempotencyKey := bindingKey + "\x00" + record.IdempotencyKey
		if idempotency[idempotencyKey] {
			return fmt.Errorf("duplicate idempotency key %q", record.IdempotencyKey)
		}
		idempotency[idempotencyKey] = true
		for _, commandID := range record.CommandIDs {
			key := bindingKey + "\x00" + commandID
			if commands[key] {
				return fmt.Errorf("duplicate command ID %q", commandID)
			}
			commands[key] = true
		}
	}
	return nil
}

func (r commandLedgerRecord) validate() error {
	if _, err := uuid.Parse(r.BindingID); err != nil {
		return errors.New("binding_id must be a UUID")
	}
	if len(r.CommandIDs) == 0 || len(r.CommandIDs) > commandLedgerMaxAliases {
		return errors.New("command_ids count is invalid")
	}
	seen := map[string]bool{}
	for _, commandID := range r.CommandIDs {
		if strings.TrimSpace(commandID) == "" || seen[commandID] {
			return errors.New("command_ids contain an empty or duplicate value")
		}
		seen[commandID] = true
	}
	acknowledged := map[string]bool{}
	for _, commandID := range r.AcknowledgedIDs {
		if !seen[commandID] {
			return errors.New("acknowledged command ID is not retained")
		}
		if acknowledged[commandID] {
			return errors.New("acknowledged command IDs contain a duplicate value")
		}
		acknowledged[commandID] = true
	}
	if strings.TrimSpace(r.IdempotencyKey) == "" || len(r.RequestFingerprint) != 64 || strings.TrimSpace(r.Name) == "" {
		return errors.New("idempotency identity and command name are required")
	}
	if r.CreatedAt.IsZero() || r.ExpiresAt.IsZero() || !r.ExpiresAt.After(r.CreatedAt) || r.UpdatedAt.IsZero() {
		return errors.New("valid command timestamps are required")
	}
	switch r.State {
	case commandLedgerRunning:
		if r.Outcome != nil || r.CompletedAt != nil {
			return errors.New("running command cannot contain a terminal outcome")
		}
	case commandLedgerTerminal:
		if r.Outcome == nil || r.CompletedAt == nil || r.CompletedAt.IsZero() {
			return errors.New("terminal command requires outcome and completion time")
		}
		result := r.resultFor(r.CommandIDs[0])
		if err := result.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported command ledger state %q", r.State)
	}
	return nil
}

func (l *CommandLedger) Begin(bindingID string, request devicev1.CommandRequest, now time.Time) (CommandLedgerDecision, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := uuid.Parse(bindingID); err != nil {
		return CommandLedgerDecision{}, errors.New("binding_id must be a UUID")
	}
	fingerprint, err := devicev1.CommandRequestFingerprint(request)
	if err != nil {
		return CommandLedgerDecision{}, err
	}
	for index := range l.doc.Records {
		record := &l.doc.Records[index]
		if !strings.EqualFold(record.BindingID, bindingID) {
			continue
		}
		matchesID := ledgerContains(record.CommandIDs, request.CommandID)
		matchesKey := record.IdempotencyKey == request.IdempotencyKey
		if !matchesID && !matchesKey {
			continue
		}
		if !matchesID && matchesKey {
			if record.RequestFingerprint != fingerprint {
				return CommandLedgerDecision{}, ErrCommandIdempotencyConflict
			}
			if len(record.CommandIDs) >= commandLedgerMaxAliases {
				return CommandLedgerDecision{}, errors.New("command idempotency alias limit exceeded")
			}
			previousUpdatedAt := record.UpdatedAt
			record.CommandIDs = append(record.CommandIDs, request.CommandID)
			record.UpdatedAt = now
			if err := l.saveLocked(); err != nil {
				record.CommandIDs = record.CommandIDs[:len(record.CommandIDs)-1]
				record.UpdatedAt = previousUpdatedAt
				return CommandLedgerDecision{}, err
			}
		}
		if record.IdempotencyKey != request.IdempotencyKey || record.RequestFingerprint != fingerprint {
			return CommandLedgerDecision{}, ErrCommandIdempotencyConflict
		}
		if record.State == commandLedgerRunning {
			return CommandLedgerDecision{}, ErrCommandAlreadyRunning
		}
		result := record.resultFor(request.CommandID)
		return CommandLedgerDecision{Result: &result}, nil
	}
	record := commandLedgerRecord{
		BindingID: bindingID, CommandIDs: []string{request.CommandID}, IdempotencyKey: request.IdempotencyKey,
		RequestFingerprint: fingerprint, Name: request.Name, CreatedAt: request.CreatedAt,
		ExpiresAt: request.ExpiresAt, State: commandLedgerRunning, UpdatedAt: now,
	}
	previousRecords := append([]commandLedgerRecord(nil), l.doc.Records...)
	l.pruneLocked(now)
	l.doc.Records = append(l.doc.Records, record)
	if err := l.saveLocked(); err != nil {
		l.doc.Records = previousRecords
		return CommandLedgerDecision{}, err
	}
	return CommandLedgerDecision{Execute: true}, nil
}

func (l *CommandLedger) Complete(bindingID, commandID string, result devicev1.CommandResult, completedAt time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	index := l.findByCommandLocked(bindingID, commandID)
	if index < 0 {
		return errors.New("command ledger record not found")
	}
	record := &l.doc.Records[index]
	if record.State == commandLedgerTerminal {
		stored := record.resultFor(commandID)
		left, _ := devicev1.CommandResultFingerprint(stored)
		right, err := devicev1.CommandResultFingerprint(result)
		if err != nil {
			return err
		}
		if left == right {
			return nil
		}
		return ErrCommandIdempotencyConflict
	}
	result.CommandID = commandID
	result.IdempotencyKey = record.IdempotencyKey
	result.RequestFingerprint = record.RequestFingerprint
	if err := result.Validate(); err != nil {
		return err
	}
	previous := *record
	record.State = commandLedgerTerminal
	record.Outcome = &commandLedgerOutcome{Status: result.Status, Payload: append(json.RawMessage(nil), result.Payload...), Error: cloneProtocolError(result.Error)}
	record.UpdatedAt = completedAt
	record.CompletedAt = timePointer(completedAt)
	if err := l.saveLocked(); err != nil {
		*record = previous
		return err
	}
	return nil
}

func (l *CommandLedger) MarkInterrupted(bindingID, commandID string, interruptedAt time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	index := l.findByCommandLocked(bindingID, commandID)
	if index < 0 {
		return nil
	}
	record := &l.doc.Records[index]
	if record.State != commandLedgerRunning {
		return nil
	}
	previous := *record
	outcome := unknownCommandOutcome()
	record.State, record.Outcome = commandLedgerTerminal, &outcome
	record.UpdatedAt, record.CompletedAt = interruptedAt, timePointer(interruptedAt)
	if err := l.saveLocked(); err != nil {
		*record = previous
		return err
	}
	return nil
}

func (l *CommandLedger) ReplayCandidates(bindingID string, now time.Time) []devicev1.CommandResult {
	l.mu.Lock()
	defer l.mu.Unlock()
	var results []devicev1.CommandResult
	for _, record := range l.doc.Records {
		if !strings.EqualFold(record.BindingID, bindingID) || record.State != commandLedgerTerminal {
			continue
		}
		for _, commandID := range record.CommandIDs {
			acknowledged := ledgerContains(record.AcknowledgedIDs, commandID)
			if acknowledged && !now.Before(record.ExpiresAt.Add(commandLedgerReplaySafety)) {
				continue
			}
			results = append(results, record.resultFor(commandID))
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].CommandID < results[j].CommandID })
	return results
}

func (l *CommandLedger) Acknowledge(bindingID string, ack devicev1.CommandReplayAck, acknowledgedAt time.Time) error {
	if err := ack.Validate(); err != nil {
		return err
	}
	if ack.Disposition != devicev1.CommandReplayRecorded && ack.Disposition != devicev1.CommandReplayAlreadyRecorded {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	index := l.findByCommandLocked(bindingID, ack.CommandID)
	if index < 0 {
		return nil
	}
	record := &l.doc.Records[index]
	if ledgerContains(record.AcknowledgedIDs, ack.CommandID) {
		return nil
	}
	previousLength := len(record.AcknowledgedIDs)
	previousUpdatedAt := record.UpdatedAt
	record.AcknowledgedIDs = append(record.AcknowledgedIDs, ack.CommandID)
	record.UpdatedAt = acknowledgedAt
	if err := l.saveLocked(); err != nil {
		record.AcknowledgedIDs = record.AcknowledgedIDs[:previousLength]
		record.UpdatedAt = previousUpdatedAt
		return err
	}
	return nil
}

func (l *CommandLedger) findByCommandLocked(bindingID, commandID string) int {
	for index := range l.doc.Records {
		record := &l.doc.Records[index]
		if strings.EqualFold(record.BindingID, bindingID) && ledgerContains(record.CommandIDs, commandID) {
			return index
		}
	}
	return -1
}

func (l *CommandLedger) pruneLocked(now time.Time) {
	kept := l.doc.Records[:0]
	for _, record := range l.doc.Records {
		if record.State == commandLedgerTerminal && record.CompletedAt != nil && !now.Before(record.CompletedAt.Add(commandLedgerRetention)) {
			continue
		}
		kept = append(kept, record)
	}
	l.doc.Records = kept
	counts := map[string]int{}
	for _, record := range l.doc.Records {
		counts[strings.ToLower(record.BindingID)]++
	}
	for bindingID, count := range counts {
		if count <= commandLedgerTargetPerBinding {
			continue
		}
		eligible := make([]int, 0)
		for index, record := range l.doc.Records {
			if strings.EqualFold(record.BindingID, bindingID) && record.State == commandLedgerTerminal && !now.Before(record.ExpiresAt.Add(commandLedgerReplaySafety)) {
				eligible = append(eligible, index)
			}
		}
		sort.Slice(eligible, func(i, j int) bool {
			return l.doc.Records[eligible[i]].CompletedAt.Before(*l.doc.Records[eligible[j]].CompletedAt)
		})
		remove := map[int]bool{}
		for _, index := range eligible {
			if count <= commandLedgerTargetPerBinding {
				break
			}
			remove[index] = true
			count--
		}
		if len(remove) > 0 {
			compacted := l.doc.Records[:0]
			for index, record := range l.doc.Records {
				if !remove[index] {
					compacted = append(compacted, record)
				}
			}
			l.doc.Records = compacted
		}
	}
}

func (r commandLedgerRecord) resultFor(commandID string) devicev1.CommandResult {
	if r.Outcome == nil {
		return devicev1.CommandResult{}
	}
	return devicev1.CommandResult{
		CommandID: commandID, IdempotencyKey: r.IdempotencyKey, RequestFingerprint: r.RequestFingerprint,
		Status: r.Outcome.Status, Payload: append(json.RawMessage(nil), r.Outcome.Payload...), Error: cloneProtocolError(r.Outcome.Error),
	}
}

func unknownCommandOutcome() commandLedgerOutcome {
	return commandLedgerOutcome{Status: devicev1.CommandFailed, Error: &devicev1.ProtocolError{
		Code: "command_outcome_unknown", Message: "The previous MGA Client session ended before it could prove the command outcome. MGA will not repeat the action automatically.", Retryable: false,
	}}
}

func cloneProtocolError(value *devicev1.ProtocolError) *devicev1.ProtocolError {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func ledgerContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func timePointer(value time.Time) *time.Time {
	copy := value
	return &copy
}

func (l *CommandLedger) saveLocked() error {
	if err := l.doc.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(l.doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	temporary := l.path + ".tmp"
	if err := os.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, l.path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace command ledger: %w", err)
	}
	return nil
}
