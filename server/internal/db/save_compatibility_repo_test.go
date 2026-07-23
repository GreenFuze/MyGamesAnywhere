package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/savecompat"
)

func TestSaveCompatibilityRepositoryPersistsExactDirectionalRules(t *testing.T) {
	database := NewSQLiteDatabaseWithMigrationOptions(
		testLogger{},
		testDBConfig{dbPath: filepath.Join(t.TempDir(), "mga.sqlite")},
		core.MigrationOptions{BackupBeforeMigrate: false},
	)
	if err := database.Connect(); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	repository := NewSaveCompatibilityRepository(database)
	ctx := context.Background()
	source := savecompat.FormatRef{ID: "scummvm:engine-save", Version: "1"}
	target := savecompat.FormatRef{ID: "native:game-save", Version: "2"}
	converter := savecompat.ConverterDefinition{
		ID: "builtin:scummvm-to-native", Version: "1.0.0", Source: source, Target: target,
		Attribution: "Verified MGA fixture", ImplementationKind: "builtin", Reversible: false, Enabled: true,
	}
	if err := repository.UpsertConverter(ctx, converter); err != nil {
		t.Fatal(err)
	}
	rule := savecompat.CompatibilityRule{
		ID: "scummvm-to-native-v1", Source: source, Target: target,
		Relationship: savecompat.RelationshipConverter,
		ConverterID:  converter.ID, ConverterVersion: converter.Version,
		EvidenceSource: "Verified release pair", EvidenceVersion: "2026-07-23",
		EvidenceJSON: `{"source_release":"fixture-a","target_release":"fixture-b"}`,
		Enabled:      true,
	}
	if err := repository.UpsertRule(ctx, rule); err != nil {
		t.Fatal(err)
	}

	gotConverter, err := repository.GetConverter(ctx, converter.ID, converter.Version)
	if err != nil || gotConverter == nil || !gotConverter.ExecutableMatches(converter) || !gotConverter.Enabled {
		t.Fatalf("converter=%+v err=%v", gotConverter, err)
	}
	gotRule, err := repository.FindRule(ctx, source, target)
	if err != nil || gotRule == nil || gotRule.ID != rule.ID || !gotRule.Enabled {
		t.Fatalf("rule=%+v err=%v", gotRule, err)
	}
	reverse, err := repository.FindRule(ctx, target, source)
	if err != nil || reverse != nil {
		t.Fatalf("reverse rule=%+v err=%v", reverse, err)
	}

	rule.Enabled = false
	if err := repository.UpsertRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	gotRule, err = repository.FindRule(ctx, source, target)
	if err != nil || gotRule == nil || gotRule.Enabled {
		t.Fatalf("disabled rule=%+v err=%v", gotRule, err)
	}
}
