package gamesvc

import (
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func TestContentRelationshipProjectorLinksChildByProviderScopedExternalID(t *testing.T) {
	parent := canonicalContentGame("parent", "Doom", core.GameKindBaseGame, "metadata-launchbox", "100", "")
	child := canonicalContentGame("child", "Doom: Episode", core.GameKindDLC, "metadata-launchbox", "101", "100")

	projection := NewContentRelationshipProjector().Project(parent.ID, []*core.CanonicalGame{parent, child})
	if len(projection.AddOns) != 1 || projection.AddOns[0].ID != child.ID {
		t.Fatalf("add-ons = %+v, want child", projection.AddOns)
	}

	childProjection := NewContentRelationshipProjector().Project(child.ID, []*core.CanonicalGame{parent, child})
	if childProjection.State != ContentRelationshipStateKnown || childProjection.Parent == nil || childProjection.Parent.ID != parent.ID {
		t.Fatalf("child projection = %+v, want known parent", childProjection)
	}
}

func TestContentRelationshipProjectorDoesNotCrossProviderBoundaries(t *testing.T) {
	parent := canonicalContentGame("parent", "Doom", core.GameKindBaseGame, "metadata-launchbox", "100", "")
	child := canonicalContentGame("child", "Doom: Episode", core.GameKindDLC, "metadata-other", "101", "100")

	projection := NewContentRelationshipProjector().Project(child.ID, []*core.CanonicalGame{parent, child})
	if projection.Parent != nil || projection.State != ContentRelationshipStateUnlinked {
		t.Fatalf("projection = %+v, want unlinked", projection)
	}
}

func TestContentRelationshipProjectorRefusesAmbiguousParent(t *testing.T) {
	parentA := canonicalContentGame("parent-a", "Doom A", core.GameKindBaseGame, "metadata-launchbox", "100", "")
	parentB := canonicalContentGame("parent-b", "Doom B", core.GameKindBaseGame, "metadata-launchbox", "100", "")
	child := canonicalContentGame("child", "Doom: Episode", core.GameKindExpansion, "metadata-launchbox", "101", "100")

	projection := NewContentRelationshipProjector().Project(child.ID, []*core.CanonicalGame{parentA, parentB, child})
	if projection.Parent != nil || projection.State != ContentRelationshipStateAmbiguous {
		t.Fatalf("projection = %+v, want ambiguous", projection)
	}
	if len(NewContentRelationshipProjector().Project(parentA.ID, []*core.CanonicalGame{parentA, parentB, child}).AddOns) != 0 {
		t.Fatal("ambiguous child must not be attached to either parent")
	}
}

func TestContentRelationshipProjectorPrefersManualRelationshipEvidence(t *testing.T) {
	parentA := canonicalContentGame("parent-a", "Doom A", core.GameKindBaseGame, "metadata-launchbox", "100", "")
	parentB := canonicalContentGame("parent-b", "Doom B", core.GameKindBaseGame, "metadata-launchbox", "200", "")
	child := canonicalContentGame("child", "Doom: Episode", core.GameKindDLC, "metadata-launchbox", "101", "100")
	child.SourceGames[0].ResolverMatches = append(child.SourceGames[0].ResolverMatches, core.ResolverMatch{
		PluginID:        "metadata-launchbox",
		ExternalID:      "101",
		ParentGameID:    "200",
		ManualSelection: true,
	})

	projection := NewContentRelationshipProjector().Project(child.ID, []*core.CanonicalGame{parentA, parentB, child})
	if projection.Parent == nil || projection.Parent.ID != parentB.ID || projection.State != ContentRelationshipStateKnown {
		t.Fatalf("projection = %+v, want manually selected parent-b", projection)
	}
}

func TestContentRelationshipProjectorIgnoresRemovedSourceEvidence(t *testing.T) {
	parent := canonicalContentGame("parent", "Doom", core.GameKindBaseGame, "metadata-launchbox", "100", "")
	parent.SourceGames[0].Status = "not_found"
	child := canonicalContentGame("child", "Doom: Episode", core.GameKindDLC, "metadata-launchbox", "101", "100")

	projection := NewContentRelationshipProjector().Project(child.ID, []*core.CanonicalGame{parent, child})
	if projection.Parent != nil || projection.State != ContentRelationshipStateUnlinked {
		t.Fatalf("projection = %+v, want removed parent evidence ignored", projection)
	}
}

func canonicalContentGame(id, title string, kind core.GameKind, provider, externalID, parentID string) *core.CanonicalGame {
	match := core.ResolverMatch{
		PluginID:     provider,
		Title:        title,
		Kind:         string(kind),
		ExternalID:   externalID,
		ParentGameID: parentID,
	}
	return &core.CanonicalGame{
		ID:       id,
		Title:    title,
		Platform: core.PlatformWindowsPC,
		Kind:     kind,
		ExternalIDs: []core.ExternalID{{
			Source:     provider,
			ExternalID: externalID,
		}},
		SourceGames: []*core.SourceGame{{
			ID:              "source-" + id,
			Status:          "found",
			Kind:            kind,
			ResolverMatches: []core.ResolverMatch{match},
		}},
	}
}
