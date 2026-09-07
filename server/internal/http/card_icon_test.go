package http

import (
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// A library card omits the media gallery on purpose — it is a detail-only
// collection and putting it on every row would make a library page enormous.
// But an external frontend draws a small image beside each game and has nowhere
// else to get one, so it was making a second request per game, or going
// without. The Playnite plugin went without: 0 of 254 imported games had an
// icon, while the library held icons for 119 of them and logos for 221.
//
// One asset reference per card fixes that. These tests pin which one.

func TestCardCarriesAnIconWhenTheLibraryHasOne(t *testing.T) {
	media := []core.MediaRef{
		{AssetID: 1, Type: core.MediaTypeScreenshot},
		{AssetID: 2, Type: core.MediaTypeIcon},
		{AssetID: 3, Type: core.MediaTypeLogo},
	}

	selected := selectCardIconMedia(media)
	if selected == nil {
		t.Fatal("a game with an icon got none")
	}
	if selected.AssetID != 2 {
		t.Fatalf("asset %d was chosen, want the icon (2)", selected.AssetID)
	}
}

func TestALogoStandsInWhenThereIsNoIcon(t *testing.T) {
	// Most games are this case. Without the fallback the majority of a library
	// would show no icon at all.
	media := []core.MediaRef{
		{AssetID: 1, Type: core.MediaTypeCover},
		{AssetID: 7, Type: core.MediaTypeLogo},
	}

	selected := selectCardIconMedia(media)
	if selected == nil || selected.AssetID != 7 {
		t.Fatalf("a game with only a logo got %v, want the logo (7)", selected)
	}
}

func TestTheCoverIsNeverReusedAsAnIcon(t *testing.T) {
	// The card already carries a cover. Reusing tall box art as the icon shows
	// the same picture twice in every row.
	media := []core.MediaRef{
		{AssetID: 1, Type: core.MediaTypeCover},
		{AssetID: 2, Type: core.MediaTypeScreenshot},
	}

	if selected := selectCardIconMedia(media); selected != nil {
		t.Fatalf("asset %d was reused as an icon", selected.AssetID)
	}
}

func TestACardWithoutIconOrLogoAsksForNothing(t *testing.T) {
	if selected := selectCardIconMedia(nil); selected != nil {
		t.Fatalf("an icon was invented from an empty gallery: %v", selected)
	}
}

// The whole point is that this reaches the wire. A selector that works but is
// never called leaves every frontend exactly where it started, which is the
// defect this fixes.
func TestTheLibraryCardActuallyIncludesTheIconAsset(t *testing.T) {
	controller := &GameController{}
	game := &core.CanonicalGame{
		ID:    "canonical-1",
		Title: "Alpha",
		Media: []core.MediaRef{
			{AssetID: 11, Type: core.MediaTypeIcon, URL: "https://example.invalid/icon.png"},
			{AssetID: 12, Type: core.MediaTypeScreenshot, URL: "https://example.invalid/shot.png"},
		},
		SourceGames: []*core.SourceGame{{
			ID:       "scan:1",
			PluginID: "game-source-steam",
			Status:   "found",
		}},
	}

	card := controller.canonicalToLibraryGameWithIntegrationLabels(t.Context(), game, nil, nil)

	found := false
	for _, item := range card.Media {
		if item.AssetID == 11 {
			found = true
		}
	}
	if !found {
		t.Fatalf("the card carried %d media entries and none was the icon", len(card.Media))
	}
}
