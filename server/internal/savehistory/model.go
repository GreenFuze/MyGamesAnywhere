// Package savehistory owns profile-scoped Save Domain retention metadata.
package savehistory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultRetainVersions = 10
	DefaultRetainDays     = 30
)

type Policy struct {
	ProfileID      string    `json:"-"`
	DomainID       string    `json:"domain_id"`
	RetainVersions int       `json:"retain_versions"`
	RetainDays     int       `json:"retain_days"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func DefaultPolicy(profileID, domainID string) Policy {
	return Policy{ProfileID: profileID, DomainID: domainID, RetainVersions: DefaultRetainVersions, RetainDays: DefaultRetainDays}
}

func (p Policy) Validate() error {
	if err := validateID("profile ID", p.ProfileID); err != nil {
		return err
	}
	if err := validateID("Save Domain ID", p.DomainID); err != nil {
		return err
	}
	if p.RetainVersions < 1 || p.RetainVersions > 50 {
		return errors.New("retained save versions must be between 1 and 50")
	}
	if p.RetainDays < 1 || p.RetainDays > 365 {
		return errors.New("save retention days must be between 1 and 365")
	}
	return nil
}

type Version struct {
	ID              string     `json:"id"`
	ProfileID       string     `json:"-"`
	DomainID        string     `json:"domain_id"`
	CanonicalGameID string     `json:"-"`
	SourceGameID    string     `json:"-"`
	Runtime         string     `json:"-"`
	SlotID          string     `json:"-"`
	IntegrationID   string     `json:"-"`
	ManifestHash    string     `json:"manifest_hash"`
	OriginLabel     string     `json:"origin_label"`
	RouteLabel      string     `json:"route_label"`
	AcceptedAt      time.Time  `json:"accepted_at"`
	ReportedAt      *time.Time `json:"reported_at,omitempty"`
	FileCount       int        `json:"file_count"`
	TotalSize       int64      `json:"total_size"`
	PayloadKey      string     `json:"-"`
	CreatedAt       time.Time  `json:"-"`
}

func (v Version) Validate() error {
	for name, value := range map[string]string{
		"version ID": v.ID, "profile ID": v.ProfileID, "Save Domain ID": v.DomainID,
		"game ID": v.CanonicalGameID, "source game ID": v.SourceGameID,
		"runtime": v.Runtime, "slot ID": v.SlotID, "integration ID": v.IntegrationID,
		"payload key": v.PayloadKey,
	} {
		if err := validateID(name, value); err != nil {
			return err
		}
	}
	if len(v.ManifestHash) != 64 {
		return errors.New("save history manifest hash is invalid")
	}
	if err := validateLabel("origin label", v.OriginLabel); err != nil {
		return err
	}
	if err := validateLabel("route label", v.RouteLabel); err != nil {
		return err
	}
	if v.AcceptedAt.IsZero() || v.FileCount < 0 || v.TotalSize < 0 {
		return errors.New("save history evidence is invalid")
	}
	return nil
}

type Repository interface {
	GetPolicy(context.Context, string, string) (Policy, error)
	UpsertPolicy(context.Context, Policy) error
	RecordVersion(context.Context, Version, Policy) ([]Version, error)
	ListVersions(context.Context, string, string) ([]Version, error)
	GetVersion(context.Context, string, string) (*Version, error)
}

func validateID(name, value string) error {
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || len(value) > 128 || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is invalid", name)
	}
	return nil
}

func validateLabel(name, value string) error {
	return validateID(name, value)
}
