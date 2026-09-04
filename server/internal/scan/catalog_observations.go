package scan

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/catalog"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// CatalogObserver records what a provider said about a title, so that a change
// over time is recoverable.
//
// This is the only writer into the catalog. Without it, availability history
// never accumulates and questions like "was this on Game Pass when I played
// it?" have no answer at all.
type CatalogObserver interface {
	Observe(ctx context.Context, command catalog.ObservationCommand) (*catalog.Offer, error)
}

// SetCatalogObserver wires observation recording. Leaving it unset keeps the
// scan working exactly as before, minus the history.
func (o *Orchestrator) SetCatalogObserver(observer CatalogObserver) {
	o.catalogObserver = observer
}

// recordCatalogObservations writes one observation per source game a provider
// described.
//
// It runs after persistence because an offer belongs to a canonical game, and
// canonical ids only exist once the scan has been reconciled. Offers are keyed
// by (plugin, external id) rather than source-game id: persistence rebinds a
// scanned row onto an existing one by that natural key, so the id we generated
// during the scan is not necessarily the id that was stored.
//
// A failure here is logged and never fails the scan. History is valuable, but
// it is not worth losing a completed scan over.
func (o *Orchestrator) recordCatalogObservations(
	ctx context.Context,
	canonicalGames []*core.CanonicalGame,
	offers map[string]map[string]StorefrontOffer,
) {
	if o.catalogObserver == nil || len(offers) == 0 {
		return
	}

	observedAt := time.Now().UTC()
	recorded := 0
	failed := 0

	for _, canonical := range canonicalGames {
		if canonical == nil || strings.TrimSpace(canonical.ID) == "" {
			continue
		}
		for _, sourceGame := range canonical.SourceGames {
			if sourceGame == nil {
				continue
			}
			byExternalID, ok := offers[sourceGame.IntegrationID]
			if !ok {
				continue
			}
			offer, ok := byExternalID[sourceGame.ExternalID]
			if !ok {
				continue
			}

			command, err := buildObservationCommand(canonical, sourceGame, offer, observedAt)
			if err != nil {
				failed++
				o.logger.Warn("orchestrator: skipping catalog observation",
					"source_game_id", sourceGame.ID, "error", err)
				continue
			}
			if _, err := o.catalogObserver.Observe(ctx, *command); err != nil {
				failed++
				o.logger.Warn("orchestrator: failed to record catalog observation",
					"source_game_id", sourceGame.ID, "error", err)
				continue
			}
			recorded++
		}
	}

	if recorded > 0 || failed > 0 {
		o.logger.Info("orchestrator: recorded catalog observations", "recorded", recorded, "failed", failed)
	}
}

// buildObservationCommand turns one provider report into a catalog observation.
func buildObservationCommand(
	canonical *core.CanonicalGame,
	sourceGame *core.SourceGame,
	offer StorefrontOffer,
	observedAt time.Time,
) (*catalog.ObservationCommand, error) {
	sku := strings.TrimSpace(offer.SKU)
	if sku == "" {
		sku = strings.TrimSpace(sourceGame.ExternalID)
	}
	platform := strings.TrimSpace(offer.Platform)
	if platform == "" {
		platform = strings.TrimSpace(string(sourceGame.Platform))
	}

	evidence, err := json.Marshal(catalogEvidence{
		ExternalID:           offer.ExternalID,
		IsGamePass:           offer.IsGamePass,
		XcloudAvailable:      offer.XcloudAvailable,
		LastPlayedAt:         strings.TrimSpace(offer.LastPlayedAt),
		Played:               offer.Played(),
		AchievementsUnlocked: offer.AchievementsUnlocked,
		AchievementsTotal:    offer.AchievementsTotal,
		GamerscoreEarned:     offer.GamerscoreEarned,
		GamerscoreTotal:      offer.GamerscoreTotal,
	})
	if err != nil {
		return nil, err
	}

	command := &catalog.ObservationCommand{
		CanonicalGameID: canonical.ID,
		SourceGameID:    sourceGame.ID,
		IntegrationID:   sourceGame.IntegrationID,
		Provider:        sourceGame.PluginID,
		SKU:             sku,
		Platform:        platform,
		Entitlement:     entitlementOrUnknown(offer.Entitlement),
		Delivery:        catalog.DeliveryStorefront,
		Availability:    availabilityOrUnknown(offer.Availability),
		EvidenceSource:  sourceGame.PluginID,
		EvidenceJSON:    evidence,
		ObservedAt:      observedAt,
	}
	if err := command.Validate(); err != nil {
		return nil, err
	}
	return command, nil
}

// catalogEvidence is what the observation is based on, kept verbatim so a later
// reader can see why MGA concluded what it did.
type catalogEvidence struct {
	ExternalID           string `json:"external_id,omitempty"`
	IsGamePass           bool   `json:"is_game_pass"`
	XcloudAvailable      bool   `json:"xcloud_available"`
	LastPlayedAt         string `json:"last_played_at,omitempty"`
	Played               bool   `json:"played"`
	AchievementsUnlocked int    `json:"achievements_unlocked,omitempty"`
	AchievementsTotal    int    `json:"achievements_total,omitempty"`
	GamerscoreEarned     int    `json:"gamerscore_earned,omitempty"`
	GamerscoreTotal      int    `json:"gamerscore_total,omitempty"`
}

// entitlementOrUnknown refuses to upgrade an unrecognised claim.
//
// A provider that says nothing, or says something this server does not
// understand, leaves MGA not knowing — which is different from, and must never
// be rendered as, the player owning the game.
func entitlementOrUnknown(value string) catalog.Entitlement {
	entitlement := catalog.Entitlement(strings.TrimSpace(strings.ToLower(value)))
	if entitlement.Valid() {
		return entitlement
	}
	return catalog.EntitlementUnknown
}

// availabilityOrUnknown is the same refusal for availability. Crucially,
// unknown is not "unavailable": a provider that stops reporting must not make
// MGA announce that a game has been removed.
func availabilityOrUnknown(value string) catalog.Availability {
	availability := catalog.Availability(strings.TrimSpace(strings.ToLower(value)))
	if availability.Valid() {
		return availability
	}
	return catalog.AvailabilityUnknown
}
