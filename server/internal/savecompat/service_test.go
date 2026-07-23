package savecompat

import (
	"context"
	"errors"
	"testing"
)

type memoryRepository struct {
	converter *ConverterDefinition
	rule      *CompatibilityRule
}

func (r *memoryRepository) UpsertConverter(_ context.Context, definition ConverterDefinition) error {
	copy := definition
	r.converter = &copy
	return nil
}

func (r *memoryRepository) GetConverter(_ context.Context, id, version string) (*ConverterDefinition, error) {
	if r.converter == nil || r.converter.ID != id || r.converter.Version != version {
		return nil, nil
	}
	copy := *r.converter
	return &copy, nil
}

func (r *memoryRepository) UpsertRule(_ context.Context, rule CompatibilityRule) error {
	copy := rule
	r.rule = &copy
	return nil
}

func (r *memoryRepository) FindRule(_ context.Context, source, target FormatRef) (*CompatibilityRule, error) {
	if r.rule == nil || !r.rule.Source.Equal(source) || !r.rule.Target.Equal(target) {
		return nil, nil
	}
	copy := *r.rule
	return &copy, nil
}

type testConverter struct {
	definition ConverterDefinition
	convert    func(Snapshot) (Snapshot, error)
}

func (c testConverter) Definition() ConverterDefinition { return c.definition }

func (c testConverter) Convert(_ context.Context, source Snapshot) (Snapshot, error) {
	return c.convert(source)
}

type memoryAtomicWriter struct {
	snapshot Snapshot
	replaces int
	err      error
}

func (w *memoryAtomicWriter) Replace(_ context.Context, candidate Snapshot) error {
	if w.err != nil {
		return w.err
	}
	w.snapshot = candidate.Clone()
	w.replaces++
	return nil
}

func TestServiceCommitsOnlyAfterExactCodeBackedConversion(t *testing.T) {
	sourceFormat := FormatRef{ID: "scummvm:engine-save", Version: "1"}
	targetFormat := FormatRef{ID: "native:game-save", Version: "2"}
	definition := ConverterDefinition{
		ID: "builtin:scummvm-to-native", Version: "1.0.0",
		Source: sourceFormat, Target: targetFormat,
		Attribution: "MGA test fixture", ImplementationKind: "builtin",
		Enabled: true,
	}
	rule := CompatibilityRule{
		ID: "test-rule", Source: sourceFormat, Target: targetFormat,
		Relationship: RelationshipConverter,
		ConverterID:  definition.ID, ConverterVersion: definition.Version,
		EvidenceSource: "verified test fixture", EvidenceVersion: "1", EvidenceJSON: `{"fixture":"known-pair"}`,
		Enabled: true,
	}
	repository := &memoryRepository{converter: &definition, rule: &rule}
	converter := testConverter{definition: definition, convert: func(source Snapshot) (Snapshot, error) {
		source.Format = targetFormat
		source.Files[0].Data[0] = 'B'
		return source, nil
	}}
	service, err := NewService(repository, converter)
	if err != nil {
		t.Fatal(err)
	}
	input := Snapshot{Format: sourceFormat, Files: []SnapshotFile{{Path: "slot/save.dat", Data: []byte("A")}}}
	writer := &memoryAtomicWriter{}
	if err := service.ConvertAndCommit(context.Background(), input, targetFormat, writer); err != nil {
		t.Fatal(err)
	}
	if writer.replaces != 1 || !writer.snapshot.Format.Equal(targetFormat) || string(writer.snapshot.Files[0].Data) != "B" {
		t.Fatalf("committed snapshot = %+v", writer.snapshot)
	}
	if string(input.Files[0].Data) != "A" {
		t.Fatalf("converter mutated source snapshot: %q", input.Files[0].Data)
	}
}

func TestServiceConverterFailureAndPanicPreserveLastGoodSnapshot(t *testing.T) {
	sourceFormat := FormatRef{ID: "emulator:a", Version: "1"}
	targetFormat := FormatRef{ID: "emulator:b", Version: "1"}
	definition := ConverterDefinition{
		ID: "builtin:a-to-b", Version: "1", Source: sourceFormat, Target: targetFormat,
		Attribution: "MGA test fixture", ImplementationKind: "builtin", Enabled: true,
	}
	rule := CompatibilityRule{
		ID: "a-to-b", Source: sourceFormat, Target: targetFormat, Relationship: RelationshipConverter,
		ConverterID: definition.ID, ConverterVersion: definition.Version,
		EvidenceSource: "verified test fixture", EvidenceVersion: "1", EvidenceJSON: `{}`, Enabled: true,
	}
	input := Snapshot{Format: sourceFormat, Files: []SnapshotFile{{Path: "save.dat", Data: []byte("source")}}}
	lastGood := Snapshot{Format: targetFormat, Files: []SnapshotFile{{Path: "save.dat", Data: []byte("last-good")}}}

	for _, test := range []struct {
		name    string
		convert func(Snapshot) (Snapshot, error)
	}{
		{"error", func(Snapshot) (Snapshot, error) { return Snapshot{}, errors.New("broken conversion") }},
		{"panic", func(Snapshot) (Snapshot, error) { panic("broken converter") }},
		{"invalid output", func(source Snapshot) (Snapshot, error) {
			source.Format = targetFormat
			source.Files[0].Path = "../outside.dat"
			return source, nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &memoryRepository{converter: &definition, rule: &rule}
			service, err := NewService(repository, testConverter{definition: definition, convert: test.convert})
			if err != nil {
				t.Fatal(err)
			}
			writer := &memoryAtomicWriter{snapshot: lastGood.Clone()}
			if err := service.ConvertAndCommit(context.Background(), input, targetFormat, writer); err == nil {
				t.Fatal("failed converter returned success")
			}
			if writer.replaces != 0 || string(writer.snapshot.Files[0].Data) != "last-good" {
				t.Fatalf("failed converter changed destination: %+v", writer.snapshot)
			}
			if string(input.Files[0].Data) != "source" {
				t.Fatalf("failed converter changed source: %q", input.Files[0].Data)
			}
		})
	}
}

func TestServiceRequiresDirectionalEnabledEvidenceAndMatchingMetadata(t *testing.T) {
	sourceFormat := FormatRef{ID: "browser:emulatorjs", Version: "1"}
	targetFormat := FormatRef{ID: "local:retroarch", Version: "2"}
	definition := ConverterDefinition{
		ID: "builtin:browser-to-retroarch", Version: "1", Source: sourceFormat, Target: targetFormat,
		Attribution: "MGA test fixture", ImplementationKind: "builtin", Enabled: true,
	}
	rule := CompatibilityRule{
		ID: "browser-to-retroarch", Source: sourceFormat, Target: targetFormat,
		Relationship: RelationshipConverter, ConverterID: definition.ID, ConverterVersion: definition.Version,
		EvidenceSource: "verified test fixture", EvidenceVersion: "1", EvidenceJSON: `{}`, Enabled: true,
	}
	input := Snapshot{Format: sourceFormat, Files: []SnapshotFile{{Path: "save.dat", Data: []byte("save")}}}
	converter := testConverter{definition: definition, convert: func(source Snapshot) (Snapshot, error) {
		source.Format = targetFormat
		return source, nil
	}}
	repository := &memoryRepository{converter: &definition, rule: &rule}
	service, err := NewService(repository, converter)
	if err != nil {
		t.Fatal(err)
	}

	if err := service.ConvertAndCommit(context.Background(), Snapshot{Format: targetFormat}, sourceFormat, &memoryAtomicWriter{}); err == nil {
		t.Fatal("reverse direction was accepted without an explicit rule")
	}
	rule.Enabled = false
	if err := service.ConvertAndCommit(context.Background(), input, targetFormat, &memoryAtomicWriter{}); err == nil {
		t.Fatal("disabled rule was accepted")
	}
	rule.Enabled = true
	definition.Attribution = "changed persisted metadata"
	if err := service.ConvertAndCommit(context.Background(), input, targetFormat, &memoryAtomicWriter{}); err == nil {
		t.Fatal("persisted/code converter metadata mismatch was accepted")
	}
}

func TestServiceCopiesExplicitSameFormatRuleWithoutConverter(t *testing.T) {
	format := FormatRef{ID: "scummvm:engine-save", Version: "1"}
	rule := CompatibilityRule{
		ID: "same-scummvm-format", Source: format, Target: format,
		Relationship:   RelationshipSameFormat,
		EvidenceSource: "ScummVM format contract", EvidenceVersion: "2026.1", EvidenceJSON: `{}`,
		Reversible: true, Enabled: true,
	}
	service, err := NewService(&memoryRepository{rule: &rule})
	if err != nil {
		t.Fatal(err)
	}
	input := Snapshot{Format: format, Files: []SnapshotFile{{Path: "save.s01", Data: []byte("save")}}}
	writer := &memoryAtomicWriter{}
	if err := service.ConvertAndCommit(context.Background(), input, format, writer); err != nil {
		t.Fatal(err)
	}
	writer.snapshot.Files[0].Data[0] = 'X'
	if string(input.Files[0].Data) != "save" {
		t.Fatal("same-format commit shared mutable snapshot data")
	}
}
