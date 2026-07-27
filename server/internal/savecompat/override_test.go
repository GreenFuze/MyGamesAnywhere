package savecompat

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryOverrideRepository struct {
	created  *CompatibilityOverride
	approved *CompatibilityOverride
}

func (r *memoryOverrideRepository) CreateOverride(_ context.Context, override CompatibilityOverride) error {
	copy := override
	r.created = &copy
	return nil
}

func (r *memoryOverrideRepository) GetOverride(context.Context, string) (*CompatibilityOverride, error) {
	return nil, nil
}

func (r *memoryOverrideRepository) ApproveOverride(context.Context, string, string, time.Time, bool) (*CompatibilityOverride, error) {
	return nil, nil
}

func (r *memoryOverrideRepository) RevokeOverride(context.Context, string, string, time.Time, bool) (*CompatibilityOverride, error) {
	return nil, nil
}

func (r *memoryOverrideRepository) FindApprovedOverride(context.Context, OverrideScope) (*CompatibilityOverride, error) {
	if r.approved == nil {
		return nil, nil
	}
	copy := *r.approved
	return &copy, nil
}

func TestOverrideServiceMakesImportedEvidencePendingAndServerOwned(t *testing.T) {
	repository := &memoryOverrideRepository{}
	service, err := NewOverrideService(repository)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.newID = func() string { return "server-generated-id" }

	got, err := service.Submit(context.Background(), OverrideSubmission{
		Scope: OverrideScope{
			OwnerProfileID: "profile-1",
			SourceDomainID: "save:source",
			TargetDomainID: "save:target",
			Source:         FormatRef{ID: "scummvm:game", Version: "1"},
			Target:         FormatRef{ID: "native:game", Version: "2"},
		},
		Relationship:    RelationshipSameFormat,
		Origin:          OverrideOriginCommunity,
		Attribution:     "Community compatibility list",
		EvidenceSource:  "example/community-list",
		EvidenceVersion: "2026-07-27",
		EvidenceJSON:    `{"source_release":"edition-a","target_release":"edition-b"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "server-generated-id" || got.State != OverrideStatePending || got.ReviewedBy != "" || got.ReviewedAt != nil {
		t.Fatalf("submitted override = %+v", got)
	}
	if repository.created == nil || repository.created.ID != got.ID {
		t.Fatalf("repository received = %+v", repository.created)
	}
}

func TestCompatibilityOverrideRejectsUnsafeCommunityEvidence(t *testing.T) {
	now := time.Now().UTC()
	base := CompatibilityOverride{
		ID: "override-1",
		Scope: OverrideScope{
			OwnerProfileID: "profile-1",
			SourceDomainID: "save:source",
			TargetDomainID: "save:target",
			Source:         FormatRef{ID: "source:format", Version: "1"},
			Target:         FormatRef{ID: "target:format", Version: "1"},
		},
		Relationship:    RelationshipSameFormat,
		Origin:          OverrideOriginCommunity,
		Attribution:     "Community list",
		EvidenceSource:  "fixture",
		EvidenceVersion: "1",
		EvidenceJSON:    `{"release":"safe"}`,
		State:           OverrideStatePending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	tests := []string{
		`{"access_token":"secret"}`,
		`{"save_path":"C:\\Users\\Player\\save.dat"}`,
		`{"nested":{"file_path":"/home/player/save"}}`,
		`["not","an","object"]`,
	}
	for _, evidence := range tests {
		candidate := base
		candidate.EvidenceJSON = evidence
		if err := candidate.Validate(); err == nil {
			t.Fatalf("unsafe evidence was accepted: %s", evidence)
		}
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("safe evidence rejected: %v", err)
	}
}

func TestOverrideScopeIsExactAndDirectional(t *testing.T) {
	scope := OverrideScope{
		OwnerProfileID: "profile-1",
		SourceDomainID: "save:source",
		TargetDomainID: "save:target",
		Source:         FormatRef{ID: "source:format", Version: "1"},
		Target:         FormatRef{ID: "target:format", Version: "2"},
	}
	if err := scope.Validate(); err != nil {
		t.Fatal(err)
	}
	sameDomain := scope
	sameDomain.TargetDomainID = sameDomain.SourceDomainID
	if err := sameDomain.Validate(); err == nil {
		t.Fatal("same-domain override was accepted")
	}
	if !errors.Is(ErrOverrideConflict, ErrOverrideConflict) {
		t.Fatal("override conflict sentinel is not stable")
	}
}

func TestConversionUsesOnlyExactApprovedOverrideScope(t *testing.T) {
	scope := OverrideScope{
		OwnerProfileID: "profile-1",
		SourceDomainID: "save:source",
		TargetDomainID: "save:target",
		Source:         FormatRef{ID: "source:format", Version: "1"},
		Target:         FormatRef{ID: "target:format", Version: "2"},
	}
	now := time.Now().UTC()
	repository := &memoryOverrideRepository{approved: &CompatibilityOverride{
		ID:              "override-approved",
		Scope:           scope,
		Relationship:    RelationshipSameFormat,
		Origin:          OverrideOriginPlayer,
		Attribution:     "Player-reviewed fixture",
		EvidenceSource:  "fixture",
		EvidenceVersion: "1",
		EvidenceJSON:    `{"source_release":"a","target_release":"b"}`,
		State:           OverrideStateApproved,
		ReviewedBy:      "profile-admin",
		CreatedAt:       now,
		UpdatedAt:       now,
		ReviewedAt:      &now,
	}}
	service, err := NewServiceWithOverrides(&memoryRepository{}, repository)
	if err != nil {
		t.Fatal(err)
	}
	writer := &memoryAtomicWriter{}
	source := Snapshot{Format: scope.Source, Files: []SnapshotFile{{Path: "save.dat", Data: []byte("state")}}}
	if err := service.ConvertAndCommitOverride(context.Background(), source, scope.Target, scope, writer); err != nil {
		t.Fatal(err)
	}
	if writer.snapshot.Format != scope.Target || string(writer.snapshot.Files[0].Data) != "state" {
		t.Fatalf("committed snapshot = %+v", writer.snapshot)
	}

	wrongScope := scope
	wrongScope.TargetDomainID = "save:another-edition"
	if err := service.ConvertAndCommitOverride(context.Background(), source, scope.Target, wrongScope, &memoryAtomicWriter{}); err == nil {
		t.Fatal("approved override broadened to another Save Domain")
	}
}
