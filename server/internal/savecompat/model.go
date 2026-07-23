// Package savecompat owns explicit, versioned save-format compatibility.
// Game titles are intentionally absent from this model: identity is never
// compatibility evidence.
package savecompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

const (
	maxIDLength          = 128
	maxVersionLength     = 64
	maxAttributionLength = 256
	maxEvidenceBytes     = 8192
	maxSnapshotFiles     = 4096
	maxSnapshotBytes     = 64 << 20
)

type Relationship string

const (
	RelationshipSameFormat Relationship = "same_format"
	RelationshipConverter  Relationship = "converter"
)

type FormatRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

func (f FormatRef) Validate() error {
	if err := validateSingleLine("format ID", f.ID, maxIDLength); err != nil {
		return err
	}
	return validateSingleLine("format version", f.Version, maxVersionLength)
}

func (f FormatRef) Equal(other FormatRef) bool {
	return f.ID == other.ID && f.Version == other.Version
}

type ConverterDefinition struct {
	ID                 string    `json:"id"`
	Version            string    `json:"version"`
	Source             FormatRef `json:"source"`
	Target             FormatRef `json:"target"`
	Attribution        string    `json:"attribution"`
	ImplementationKind string    `json:"implementation_kind"`
	Reversible         bool      `json:"reversible"`
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (d ConverterDefinition) Validate() error {
	if err := validateSingleLine("converter ID", d.ID, maxIDLength); err != nil {
		return err
	}
	if err := validateSingleLine("converter version", d.Version, maxVersionLength); err != nil {
		return err
	}
	if err := d.Source.Validate(); err != nil {
		return fmt.Errorf("source %w", err)
	}
	if err := d.Target.Validate(); err != nil {
		return fmt.Errorf("target %w", err)
	}
	if err := validateSingleLine("converter attribution", d.Attribution, maxAttributionLength); err != nil {
		return err
	}
	if d.ImplementationKind != "builtin" {
		return errors.New("only builtin save converters are supported")
	}
	return nil
}

func (d ConverterDefinition) ExecutableMatches(other ConverterDefinition) bool {
	return d.ID == other.ID &&
		d.Version == other.Version &&
		d.Source.Equal(other.Source) &&
		d.Target.Equal(other.Target) &&
		d.Attribution == other.Attribution &&
		d.ImplementationKind == other.ImplementationKind &&
		d.Reversible == other.Reversible
}

type CompatibilityRule struct {
	ID               string       `json:"id"`
	Source           FormatRef    `json:"source"`
	Target           FormatRef    `json:"target"`
	Relationship     Relationship `json:"relationship"`
	ConverterID      string       `json:"converter_id,omitempty"`
	ConverterVersion string       `json:"converter_version,omitempty"`
	EvidenceSource   string       `json:"evidence_source"`
	EvidenceVersion  string       `json:"evidence_version"`
	EvidenceJSON     string       `json:"evidence_json"`
	Reversible       bool         `json:"reversible"`
	Enabled          bool         `json:"enabled"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

func (r CompatibilityRule) Validate() error {
	if err := validateSingleLine("compatibility rule ID", r.ID, maxIDLength); err != nil {
		return err
	}
	if err := r.Source.Validate(); err != nil {
		return fmt.Errorf("source %w", err)
	}
	if err := r.Target.Validate(); err != nil {
		return fmt.Errorf("target %w", err)
	}
	if err := validateSingleLine("evidence source", r.EvidenceSource, maxAttributionLength); err != nil {
		return err
	}
	if err := validateSingleLine("evidence version", r.EvidenceVersion, maxVersionLength); err != nil {
		return err
	}
	if len(r.EvidenceJSON) > maxEvidenceBytes || !json.Valid([]byte(r.EvidenceJSON)) {
		return errors.New("compatibility evidence must be bounded valid JSON")
	}
	switch r.Relationship {
	case RelationshipSameFormat:
		if r.ConverterID != "" || r.ConverterVersion != "" {
			return errors.New("same-format compatibility cannot name a converter")
		}
	case RelationshipConverter:
		if err := validateSingleLine("converter ID", r.ConverterID, maxIDLength); err != nil {
			return err
		}
		if err := validateSingleLine("converter version", r.ConverterVersion, maxVersionLength); err != nil {
			return err
		}
	default:
		return errors.New("unsupported save compatibility relationship")
	}
	return nil
}

type Repository interface {
	UpsertConverter(context.Context, ConverterDefinition) error
	GetConverter(context.Context, string, string) (*ConverterDefinition, error)
	UpsertRule(context.Context, CompatibilityRule) error
	FindRule(context.Context, FormatRef, FormatRef) (*CompatibilityRule, error)
}

type SnapshotFile struct {
	Path string
	Data []byte
}

type Snapshot struct {
	Format FormatRef
	Files  []SnapshotFile
}

func (s Snapshot) Validate() error {
	if err := s.Format.Validate(); err != nil {
		return err
	}
	if len(s.Files) > maxSnapshotFiles {
		return errors.New("save snapshot contains too many files")
	}
	var total int64
	seen := make(map[string]bool, len(s.Files))
	for _, file := range s.Files {
		clean := path.Clean(strings.ReplaceAll(file.Path, "\\", "/"))
		if clean == "." || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("save snapshot path %q is unsafe", file.Path)
		}
		if clean != strings.ReplaceAll(file.Path, "\\", "/") || seen[clean] {
			return fmt.Errorf("save snapshot path %q is not canonical and unique", file.Path)
		}
		seen[clean] = true
		total += int64(len(file.Data))
		if total > maxSnapshotBytes {
			return errors.New("save snapshot exceeds the size limit")
		}
	}
	return nil
}

func (s Snapshot) Clone() Snapshot {
	result := Snapshot{Format: s.Format, Files: make([]SnapshotFile, len(s.Files))}
	for i, file := range s.Files {
		result.Files[i] = SnapshotFile{Path: file.Path, Data: append([]byte(nil), file.Data...)}
	}
	return result
}

func validateSingleLine(name, value string, maxLength int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > maxLength {
		return fmt.Errorf("%s exceeds %d characters", name, maxLength)
	}
	if value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s must be a trimmed single-line value", name)
	}
	return nil
}
