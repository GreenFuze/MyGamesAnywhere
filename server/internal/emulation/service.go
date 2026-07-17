package emulation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
)

var (
	ErrInvalidPreference = errors.New("invalid emulator preference")
	ErrSetupUnavailable  = errors.New("emulator setup is unavailable")
)

type Repository interface {
	ListDefaults(ctx context.Context, endpointID string) (map[core.Platform]string, error)
	SetDefault(ctx context.Context, endpointID string, platform core.Platform, emulatorID, profileID string, updatedAt time.Time) error
	ListCoreDefaults(ctx context.Context, endpointID string) (map[core.Platform]map[string]string, error)
	SetCoreDefault(ctx context.Context, endpointID string, platform core.Platform, emulatorID, coreID, profileID string, updatedAt time.Time) error
}

type endpointService interface {
	AuthorizeEndpoint(ctx context.Context, endpointID, profileID string, required devicev1.AccessLevel) error
	ListEndpoints(ctx context.Context, profileID string) ([]devices.Endpoint, error)
}

type Option struct {
	Emulator
	Detected           bool         `json:"detected"`
	Version            string       `json:"version,omitempty"`
	State              string       `json:"state"`
	Reason             string       `json:"reason,omitempty"`
	CoreProbeState     string       `json:"core_probe_state,omitempty"`
	FirmwareProbeState string       `json:"firmware_probe_state,omitempty"`
	SelectedCore       string       `json:"selected_core,omitempty"`
	ResolvedCore       string       `json:"resolved_core,omitempty"`
	Cores              []CoreOption `json:"cores,omitempty"`
	SetupAvailable     bool         `json:"setup_available"`
	SaveProbeState     string       `json:"save_probe_state,omitempty"`
	SaveKinds          []string     `json:"save_kinds,omitempty"`
	SaveRouteOverrides bool         `json:"save_route_overrides,omitempty"`
}

type CoreOption struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Detected      bool             `json:"detected"`
	State         string           `json:"state"`
	Reason        string           `json:"reason,omitempty"`
	FirmwareState string           `json:"firmware_state"`
	Capabilities  []CapabilityFact `json:"capabilities"`
}

type PlatformConfiguration struct {
	Platform        core.Platform `json:"platform"`
	PlatformName    string        `json:"platform_name"`
	SelectedDefault string        `json:"selected_default,omitempty"`
	ResolvedDefault string        `json:"resolved_default,omitempty"`
	Emulators       []Option      `json:"emulators"`
}

type DeviceConfiguration struct {
	EndpointID string                  `json:"endpoint_id"`
	Platforms  []PlatformConfiguration `json:"platforms"`
}

type Service struct {
	repository Repository
	devices    endpointService
	catalog    *Catalog
	now        func() time.Time
}

func NewService(repository Repository, deviceService endpointService, catalog *Catalog) (*Service, error) {
	if repository == nil || deviceService == nil || catalog == nil {
		return nil, errors.New("emulator repository, device service, and catalog are required")
	}
	return &Service{repository: repository, devices: deviceService, catalog: catalog, now: time.Now}, nil
}

func (s *Service) Get(ctx context.Context, endpointID, profileID string) (DeviceConfiguration, error) {
	if err := s.devices.AuthorizeEndpoint(ctx, endpointID, profileID, devicev1.AccessView); err != nil {
		return DeviceConfiguration{}, err
	}
	endpoint, err := s.findEndpoint(ctx, endpointID, profileID)
	if err != nil {
		return DeviceConfiguration{}, err
	}
	return s.GetForEndpoint(ctx, endpoint, profileID)
}

// GetForEndpoint builds configuration from an endpoint already loaded by a
// caller while independently rechecking view authorization.
func (s *Service) GetForEndpoint(ctx context.Context, endpoint devices.Endpoint, profileID string) (DeviceConfiguration, error) {
	if err := s.devices.AuthorizeEndpoint(ctx, endpoint.ID, profileID, devicev1.AccessView); err != nil {
		return DeviceConfiguration{}, err
	}
	defaults, err := s.repository.ListDefaults(ctx, endpoint.ID)
	if err != nil {
		return DeviceConfiguration{}, err
	}
	coreDefaults, err := s.repository.ListCoreDefaults(ctx, endpoint.ID)
	if err != nil {
		return DeviceConfiguration{}, err
	}
	return s.configuration(endpoint, defaults, coreDefaults), nil
}

func (s *Service) SetCoreDefault(ctx context.Context, endpointID, profileID string, platform core.Platform, emulatorID, coreID string) (DeviceConfiguration, error) {
	if err := s.devices.AuthorizeEndpoint(ctx, endpointID, profileID, devicev1.AccessOwner); err != nil {
		return DeviceConfiguration{}, err
	}
	emulatorID = strings.ToLower(strings.TrimSpace(emulatorID))
	coreID = strings.ToLower(strings.TrimSpace(coreID))
	if platform == "" || !s.catalog.Supports(platform, emulatorID) || (coreID != "" && !s.catalog.SupportsCore(platform, emulatorID, coreID)) {
		return DeviceConfiguration{}, fmt.Errorf("%w: %s cannot use %s core %q", ErrInvalidPreference, platform, emulatorID, coreID)
	}
	if err := s.repository.SetCoreDefault(ctx, endpointID, platform, emulatorID, coreID, profileID, s.now().UTC()); err != nil {
		return DeviceConfiguration{}, err
	}
	return s.Get(ctx, endpointID, profileID)
}

func (s *Service) PrepareSetup(ctx context.Context, endpointID, profileID, emulatorID, action string) (devicev1.EmulatorSetupRequest, error) {
	if err := s.devices.AuthorizeEndpoint(ctx, endpointID, profileID, devicev1.AccessOwner); err != nil {
		return devicev1.EmulatorSetupRequest{}, err
	}
	request := devicev1.EmulatorSetupRequest{EmulatorID: strings.ToLower(strings.TrimSpace(emulatorID)), Action: strings.ToLower(strings.TrimSpace(action))}
	if err := request.Validate(); err != nil {
		return devicev1.EmulatorSetupRequest{}, err
	}
	emulator, ok := s.catalog.Emulator(request.EmulatorID)
	if !ok || emulator.SetupProvider != "winget" {
		return devicev1.EmulatorSetupRequest{}, ErrSetupUnavailable
	}
	endpoint, err := s.findEndpoint(ctx, endpointID, profileID)
	if err != nil {
		return devicev1.EmulatorSetupRequest{}, err
	}
	if endpoint.Status != devicev1.EndpointReady && endpoint.Status != devicev1.EndpointBusy {
		return devicev1.EmulatorSetupRequest{}, devices.ErrEndpointOffline
	}
	if !contains(endpoint.Capabilities, devicev1.CapabilityEmulatorSetup) || !hasPackageManager(endpoint.Inventory, "winget") {
		return devicev1.EmulatorSetupRequest{}, fmt.Errorf("%w: update MGA Client or Windows Package Manager", ErrSetupUnavailable)
	}
	return request, nil
}

func (s *Service) SetDefault(ctx context.Context, endpointID, profileID string, platform core.Platform, emulatorID string) (DeviceConfiguration, error) {
	if err := s.devices.AuthorizeEndpoint(ctx, endpointID, profileID, devicev1.AccessOwner); err != nil {
		return DeviceConfiguration{}, err
	}
	emulatorID = strings.ToLower(strings.TrimSpace(emulatorID))
	if platform == "" || (emulatorID != "" && !s.catalog.Supports(platform, emulatorID)) {
		return DeviceConfiguration{}, fmt.Errorf("%w: %s cannot use %q", ErrInvalidPreference, platform, emulatorID)
	}
	if err := s.repository.SetDefault(ctx, endpointID, platform, emulatorID, profileID, s.now().UTC()); err != nil {
		return DeviceConfiguration{}, err
	}
	return s.Get(ctx, endpointID, profileID)
}

func (s *Service) RequireReady(ctx context.Context, endpointID, profileID string, platform core.Platform, emulatorID string) (Option, error) {
	if err := s.devices.AuthorizeEndpoint(ctx, endpointID, profileID, devicev1.AccessPlay); err != nil {
		return Option{}, err
	}
	if !s.catalog.Supports(platform, emulatorID) {
		return Option{}, fmt.Errorf("%w: %s cannot use %q", ErrInvalidPreference, platform, emulatorID)
	}
	endpoint, err := s.findEndpoint(ctx, endpointID, profileID)
	if err != nil {
		return Option{}, err
	}
	coreDefaults, err := s.repository.ListCoreDefaults(ctx, endpointID)
	if err != nil {
		return Option{}, err
	}
	configuration := s.configuration(endpoint, nil, coreDefaults)
	for _, platformConfiguration := range configuration.Platforms {
		if platformConfiguration.Platform != platform {
			continue
		}
		for _, option := range platformConfiguration.Emulators {
			if option.ID == emulatorID {
				if option.State != "ready" {
					return Option{}, fmt.Errorf("emulator route is not ready: %s", option.Reason)
				}
				return option, nil
			}
		}
	}
	return Option{}, fmt.Errorf("%w: emulator route not found", ErrInvalidPreference)
}

func (s *Service) findEndpoint(ctx context.Context, endpointID, profileID string) (devices.Endpoint, error) {
	endpoints, err := s.devices.ListEndpoints(ctx, profileID)
	if err != nil {
		return devices.Endpoint{}, err
	}
	for _, endpoint := range endpoints {
		if endpoint.ID == endpointID {
			return endpoint, nil
		}
	}
	return devices.Endpoint{}, devices.ErrEndpointNotFound
}

func (s *Service) configuration(endpoint devices.Endpoint, defaults map[core.Platform]string, coreDefaults map[core.Platform]map[string]string) DeviceConfiguration {
	detected := make(map[string]devicev1.RuntimeInventory)
	saveAdapters := make(map[string]devicev1.SaveAdapterInventory)
	if endpoint.Inventory != nil {
		for _, runtime := range endpoint.Inventory.Runtimes {
			detected[strings.ToLower(runtime.ID)] = runtime
		}
		for _, adapter := range endpoint.Inventory.SaveAdapters {
			saveAdapters[strings.ToLower(adapter.ID)] = adapter
		}
	}
	clientSupportsLaunch := contains(endpoint.Capabilities, devicev1.CapabilityGameLaunchEmulator)
	clientSupportsSetup := contains(endpoint.Capabilities, devicev1.CapabilityEmulatorSetup)
	wingetAvailable := hasPackageManager(endpoint.Inventory, "winget")
	configuration := DeviceConfiguration{EndpointID: endpoint.ID}
	for _, platform := range s.catalog.Platforms() {
		entry := PlatformConfiguration{Platform: platform, PlatformName: platformName(platform), SelectedDefault: defaults[platform]}
		for _, emulator := range s.catalog.ForPlatform(platform) {
			runtime, found := detected[emulator.ID]
			saveAdapter := saveAdapters[emulator.ID]
			option := Option{Emulator: emulator, Detected: found, Version: runtime.Version, State: "needs_setup", CoreProbeState: runtime.CoreProbeState, FirmwareProbeState: runtime.FirmwareProbeState, SetupAvailable: emulator.SetupProvider == "winget" && clientSupportsSetup && wingetAvailable, SaveProbeState: saveAdapter.ProbeState, SaveKinds: saveAdapter.SaveKinds, SaveRouteOverrides: saveAdapter.RouteOverrides}
			if coreDefaults[platform] != nil {
				option.SelectedCore = coreDefaults[platform][emulator.ID]
			}
			if emulator.AdapterState == "core_required" {
				option.Cores = s.coreOptions(platform, emulator, runtime)
				option.ResolvedCore = resolveCore(option.SelectedCore, option.Cores)
			}
			switch {
			case !found:
				option.State, option.Reason = "unavailable", "Not found for this device user"
			case endpoint.Status == devicev1.EndpointUpdateRequired:
				option.Reason = "Update MGA Client before playing on this device"
			case endpoint.Status != devicev1.EndpointReady && endpoint.Status != devicev1.EndpointBusy:
				option.State, option.Reason = "unavailable", "Connect MGA Client to play on this device"
			case !clientSupportsLaunch:
				option.Reason = "Update MGA Client to enable local emulator play"
			case emulator.AdapterState == "core_required":
				if option.ResolvedCore == "" {
					option.Reason = "Add a compatible core with RetroArch's Online Updater"
				} else {
					option.State = "ready"
				}
			case emulator.AdapterState != "ready":
				option.Reason = emulator.SetupHint
			default:
				option.State = "ready"
			}
			entry.Emulators = append(entry.Emulators, option)
		}
		entry.ResolvedDefault = resolveDefault(entry.SelectedDefault, entry.Emulators)
		configuration.Platforms = append(configuration.Platforms, entry)
	}
	return configuration
}

func (s *Service) coreOptions(platform core.Platform, emulator Emulator, runtime devicev1.RuntimeInventory) []CoreOption {
	detected := make(map[string]bool)
	for _, component := range runtime.Components {
		if component.Kind == "core" {
			detected[component.ID] = true
		}
	}
	result := make([]CoreOption, 0)
	for _, spec := range s.catalog.CoresFor(platform, emulator.ID) {
		firmwareState := firmwareStateForCore(platform, spec, runtime)
		option := CoreOption{
			ID: spec.ID, Name: spec.Name, Detected: detected[spec.ID], FirmwareState: firmwareState,
			State: "unavailable", Capabilities: []CapabilityFact{{ID: "retroachievements", State: spec.RetroAchievementsState}},
		}
		switch {
		case !option.Detected:
			option.Reason = "Core not installed"
		case spec.AdapterState != "ready":
			option.State, option.Reason = "needs_setup", "Typed launch support is not ready"
		case firmwareState == "not_required" || firmwareState == "present":
			option.State = "ready"
		case firmwareState == "missing":
			option.State, option.Reason = "needs_setup", "Add legally obtained firmware in RetroArch"
		default:
			option.State, option.Reason = "needs_setup", "Firmware requirements are not verified yet"
		}
		result = append(result, option)
	}
	return result
}

func firmwareStateForCore(platform core.Platform, spec CoreSpec, runtime devicev1.RuntimeInventory) string {
	policy := spec.FirmwarePolicy
	if policy == "platform_dependent" {
		if platform != core.PlatformSegaCD {
			policy = "not_required"
		} else {
			policy = "user_required"
		}
	}
	switch policy {
	case "not_required":
		return "not_required"
	case "user_required":
		for _, component := range runtime.Components {
			if component.Kind == "firmware" && component.ID == spec.FirmwareRequirementID {
				return "present"
			}
		}
		if runtime.FirmwareProbeState == "complete" {
			return "missing"
		}
		return "unknown"
	default:
		return "unknown"
	}
}

func resolveCore(selected string, options []CoreOption) string {
	for _, option := range options {
		if option.ID == selected && option.State == "ready" {
			return selected
		}
	}
	for _, option := range options {
		if option.State == "ready" {
			return option.ID
		}
	}
	return ""
}

func hasPackageManager(inventory *devicev1.DeviceInventory, wanted string) bool {
	if inventory == nil {
		return false
	}
	for _, manager := range inventory.PackageManagers {
		if manager.ID == wanted {
			return true
		}
	}
	return false
}

func resolveDefault(selected string, options []Option) string {
	for _, option := range options {
		if option.ID == selected && option.State == "ready" {
			return selected
		}
	}
	for _, option := range options {
		if option.State == "ready" {
			return option.ID
		}
	}
	return ""
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func platformName(platform core.Platform) string {
	names := map[core.Platform]string{
		core.PlatformMSDOS: "MS-DOS", core.PlatformArcade: "Arcade", core.PlatformNES: "Nintendo Entertainment System",
		core.PlatformSNES: "Super Nintendo", core.PlatformGB: "Game Boy", core.PlatformGBC: "Game Boy Color",
		core.PlatformGBA: "Game Boy Advance", core.PlatformN64: "Nintendo 64", core.PlatformGenesis: "Sega Genesis / Mega Drive",
		core.PlatformSegaMasterSystem: "Sega Master System", core.PlatformGameGear: "Sega Game Gear", core.PlatformSegaCD: "Sega CD",
		core.PlatformSega32X: "Sega 32X", core.PlatformPS1: "PlayStation", core.PlatformPS2: "PlayStation 2", core.PlatformScummVM: "ScummVM",
	}
	if name := names[platform]; name != "" {
		return name
	}
	return string(platform)
}
