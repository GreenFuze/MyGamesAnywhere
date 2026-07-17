// Package savedomain classifies route-specific save ownership without probing
// arbitrary files or claiming access to provider-managed cloud saves.
package savedomain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type Access string

const (
	AccessMGAManaged     Access = "mga_managed"
	AccessLocalFiles     Access = "local_files"
	AccessProviderAPI    Access = "provider_api"
	AccessProviderOpaque Access = "provider_opaque"
	AccessUnsupported    Access = "unsupported"
	AccessUnknown        Access = "unknown"
)

type Status string

const (
	StatusAvailable       Status = "available"
	StatusProviderManaged Status = "provider_managed"
	StatusNeedsAdapter    Status = "needs_adapter"
	StatusUnsupported     Status = "unsupported"
	StatusUnknown         Status = "unknown"
)

type Transfer string

const (
	TransferSameDomainOnly  Transfer = "same_domain_only"
	TransferConverterNeeded Transfer = "converter_required"
	TransferUnavailable     Transfer = "unavailable"
	TransferUnknown         Transfer = "unknown"
)

type Capability struct {
	DomainID string   `json:"domain_id"`
	Access   Access   `json:"access"`
	Status   Status   `json:"status"`
	Manager  string   `json:"manager"`
	Label    string   `json:"label"`
	Detail   string   `json:"detail"`
	MGARead  bool     `json:"mga_read"`
	MGAWrite bool     `json:"mga_write"`
	Transfer Transfer `json:"transfer"`
}

type Source struct {
	SourceGameID     string
	PluginID         string
	IntegrationLabel string
}

// Resolver is deliberately stateless. Keeping classification behind an object
// gives future provider and converter registries a single bounded extension point.
type Resolver struct{}

func NewResolver() *Resolver { return &Resolver{} }

func (r *Resolver) Source(source Source) Capability {
	if provider, ok := opaqueProvider(source.PluginID); ok {
		return providerCapability(domainID("source", source.SourceGameID, provider), provider, provider+" copy")
	}
	if isLocalContentProvider(source.PluginID) {
		return localCapability(domainID("source", source.SourceGameID), sourceLabel(source), "MGA needs a verified save adapter before it can back up this copy's local saves.")
	}
	return unknownCapability(domainID("source", source.SourceGameID), sourceLabel(source))
}

func (r *Resolver) Browser(source Source, runtime string) Capability {
	return Capability{
		DomainID: domainID("browser", source.SourceGameID, runtime), Access: AccessMGAManaged,
		Status: StatusAvailable, Manager: "mga", Label: "Browser play",
		Detail:  "MGA can back up saves created by this browser play option when a save backup connection is selected.",
		MGARead: true, MGAWrite: true, Transfer: TransferSameDomainOnly,
	}
}

func (r *Resolver) XCloud(source Source) Capability {
	return providerCapability(domainID("xcloud", source.SourceGameID, "xbox"), "Xbox", "Xbox Cloud Gaming")
}

func (r *Resolver) Installed(source Source, endpointID string) Capability {
	if provider, ok := opaqueProvider(source.PluginID); ok {
		return providerCapability(domainID("installed", endpointID, source.SourceGameID, provider), provider, provider+" on this device")
	}
	return localCapability(
		domainID("installed", endpointID, source.SourceGameID),
		"Installed on this device",
		"This copy may use local save files. MGA needs a verified adapter before it can back them up.",
	)
}

func (r *Resolver) Emulator(source Source, endpointID, emulatorID, coreID string) Capability {
	label := "Played with " + displayID(emulatorID)
	return localCapability(
		domainID("emulator", endpointID, source.SourceGameID, emulatorID, coreID),
		label,
		"This emulator keeps local saves. MGA needs a verified emulator adapter before it can back them up or share them with another play option.",
	)
}

func localCapability(id, label, detail string) Capability {
	return Capability{
		DomainID: id, Access: AccessLocalFiles, Status: StatusNeedsAdapter, Manager: "device",
		Label: label, Detail: detail, Transfer: TransferConverterNeeded,
	}
}

func providerCapability(id, provider, label string) Capability {
	return Capability{
		DomainID: id, Access: AccessProviderOpaque, Status: StatusProviderManaged, Manager: "provider",
		Label:    label,
		Detail:   "If this game uses " + provider + " cloud saves, " + provider + " manages them. MGA cannot access or replace them.",
		Transfer: TransferUnavailable,
	}
}

func unknownCapability(id, label string) Capability {
	return Capability{
		DomainID: id, Access: AccessUnknown, Status: StatusUnknown, Manager: "unknown",
		Label: label, Detail: "MGA does not yet know how this play option stores saves.", Transfer: TransferUnknown,
	}
}

func opaqueProvider(pluginID string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(pluginID)) {
	case "game-source-steam":
		return "Steam", true
	case "game-source-xbox":
		return "Xbox", true
	case "game-source-epic":
		return "Epic Games", true
	default:
		return "", false
	}
}

func isLocalContentProvider(pluginID string) bool {
	switch strings.ToLower(strings.TrimSpace(pluginID)) {
	case "game-source-google-drive", "game-source-gdrive", "game-source-smb", "game-source-local":
		return true
	default:
		return false
	}
}

func sourceLabel(source Source) string {
	if label := strings.TrimSpace(source.IntegrationLabel); label != "" {
		return label + " copy"
	}
	return "Game copy"
}

func displayID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "an emulator"
	}
	return strings.ReplaceAll(value, "_", " ")
}

func domainID(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "save:" + hex.EncodeToString(hash[:])
}
