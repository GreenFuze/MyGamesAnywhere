// Package profileprovision creates the default connections a brand-new profile
// needs so its first scan can identify games without any manual setup.
//
// Only plugins that are genuinely zero-setup are provisioned: no required
// config, no external account sign-in, and a capability that is either
// read-only metadata enrichment or local-only save storage. Game sources and
// cloud sync destinations always remain an explicit player choice.
package profileprovision

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/plugins"
	"github.com/google/uuid"
)

// eligibleCapabilities are the only capabilities allowed to be provisioned
// automatically. Anything else (a game source, a settings-sync destination)
// expresses player intent and must be added deliberately.
var eligibleCapabilities = map[string]bool{
	"metadata":  true,
	"save_sync": true,
}

// oauthCallbackMethod marks a plugin that requires an external account sign-in.
// Such a plugin is never provisioned silently, even with no required config,
// because it would appear connected while being unusable.
const oauthCallbackMethod = "auth.oauth.callback"

// emptyConfigJSON is the persisted config for a zero-setup connection.
const emptyConfigJSON = "{}"

// pluginLabels mirrors the web interface's PLUGIN_LABELS so an auto-created
// connection is named exactly like a hand-created one.
var pluginLabels = map[string]string{
	"metadata-steam":       "Steam Metadata",
	"metadata-gog":         "GOG Metadata",
	"metadata-hltb":        "HowLongToBeat",
	"metadata-launchbox":   "LaunchBox",
	"metadata-mame-dat":    "MAME DAT",
	"save-sync-local-disk": "Local Disk Save Sync",
}

// PluginCatalog is the subset of the plugin host this package needs.
type PluginCatalog interface {
	ListPlugins() []plugins.PluginInfo
}

// Provisioner adds default zero-setup connections to a profile.
type Provisioner struct {
	catalog PluginCatalog
	repo    core.IntegrationRepository
	logger  core.Logger
	now     func() time.Time
	newID   func() string
}

// New builds a Provisioner. A nil catalog or repository disables provisioning
// rather than failing profile creation.
func New(catalog PluginCatalog, repo core.IntegrationRepository, logger core.Logger) *Provisioner {
	return &Provisioner{
		catalog: catalog,
		repo:    repo,
		logger:  logger,
		now:     time.Now,
		newID:   uuid.NewString,
	}
}

// EligiblePlugins returns the discovered plugins that qualify for automatic
// provisioning, ordered by plugin ID so provisioning is deterministic.
func (p *Provisioner) EligiblePlugins() []plugins.PluginInfo {
	if p == nil || p.catalog == nil {
		return nil
	}

	eligible := make([]plugins.PluginInfo, 0, len(p.catalog.ListPlugins()))
	for _, info := range p.catalog.ListPlugins() {
		if IsEligible(info) {
			eligible = append(eligible, info)
		}
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].PluginID < eligible[j].PluginID })
	return eligible
}

// IsEligible reports whether a plugin can be connected with no player input.
func IsEligible(info plugins.PluginInfo) bool {
	if strings.TrimSpace(info.PluginID) == "" || len(info.Capabilities) == 0 {
		return false
	}

	// Every capability must be an auto-provisionable one.
	for _, capability := range info.Capabilities {
		if !eligibleCapabilities[capability] {
			return false
		}
	}

	// An account sign-in is player intent, not zero setup.
	for _, method := range info.Provides {
		if method == oauthCallbackMethod {
			return false
		}
	}

	return !hasRequiredConfig(info.ConfigSchema)
}

// hasRequiredConfig reports whether any declared config field is required.
func hasRequiredConfig(schema map[string]any) bool {
	for _, raw := range schema {
		field, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if required, _ := field["required"].(bool); required {
			return true
		}
	}
	return false
}

// ProvisionDefaults connects every eligible plugin to the given profile and
// returns what it created. It is idempotent: a plugin the profile already has
// is left untouched, and an existing connection is never modified.
//
// Provisioning runs in the new profile's own scope, not the caller's.
func (p *Provisioner) ProvisionDefaults(ctx context.Context, profileID string) ([]*core.Integration, error) {
	if p == nil || p.catalog == nil || p.repo == nil {
		return nil, nil
	}
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return nil, fmt.Errorf("profile ID is required to provision default connections")
	}

	eligible := p.EligiblePlugins()
	if len(eligible) == 0 {
		return nil, nil
	}

	// Scope every repository call to the profile being provisioned.
	profileCtx := core.WithProfile(ctx, &core.Profile{ID: profileID})

	existing, err := p.repo.List(profileCtx)
	if err != nil {
		return nil, fmt.Errorf("list existing connections for profile %s: %w", profileID, err)
	}
	alreadyConnected := make(map[string]bool, len(existing))
	for _, integration := range existing {
		if integration != nil {
			alreadyConnected[integration.PluginID] = true
		}
	}

	created := make([]*core.Integration, 0, len(eligible))
	for _, info := range eligible {
		if alreadyConnected[info.PluginID] {
			continue
		}

		now := p.now()
		integration := &core.Integration{
			ID:              p.newID(),
			ProfileID:       profileID,
			PluginID:        info.PluginID,
			Label:           Label(info.PluginID),
			ConfigJSON:      emptyConfigJSON,
			IntegrationType: info.Capabilities[0],
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := p.repo.Create(profileCtx, integration); err != nil {
			return created, fmt.Errorf("create default %s connection for profile %s: %w", info.PluginID, profileID, err)
		}
		created = append(created, integration)
	}

	if p.logger != nil && len(created) > 0 {
		ids := make([]string, 0, len(created))
		for _, integration := range created {
			ids = append(ids, integration.PluginID)
		}
		p.logger.Info("provisioned default profile connections",
			"profile_id", profileID, "plugin_ids", strings.Join(ids, ","))
	}
	return created, nil
}

// Label returns the display name used for an auto-created connection.
func Label(pluginID string) string {
	if label, ok := pluginLabels[pluginID]; ok {
		return label
	}
	return humanizeIdentifier(pluginID)
}

// humanizeIdentifier mirrors the web interface's humanizeIdentifier fallback.
func humanizeIdentifier(value string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(value), func(r rune) bool {
		return r == '-' || r == '_'
	})
	words := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if len(part) <= 3 {
			words = append(words, strings.ToUpper(part))
			continue
		}
		words = append(words, strings.ToUpper(part[:1])+part[1:])
	}
	return strings.Join(words, " ")
}
