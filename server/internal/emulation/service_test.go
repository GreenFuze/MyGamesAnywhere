package emulation

import (
	"context"
	"errors"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
)

type memoryRepository struct {
	defaults     map[core.Platform]string
	coreDefaults map[core.Platform]map[string]string
}

func (r *memoryRepository) ListCoreDefaults(context.Context, string) (map[core.Platform]map[string]string, error) {
	result := make(map[core.Platform]map[string]string, len(r.coreDefaults))
	for platform, defaults := range r.coreDefaults {
		result[platform] = make(map[string]string, len(defaults))
		for emulatorID, coreID := range defaults {
			result[platform][emulatorID] = coreID
		}
	}
	return result, nil
}

func (r *memoryRepository) SetCoreDefault(_ context.Context, _ string, platform core.Platform, emulatorID, coreID, _ string, _ time.Time) error {
	if r.coreDefaults == nil {
		r.coreDefaults = make(map[core.Platform]map[string]string)
	}
	if coreID == "" {
		delete(r.coreDefaults[platform], emulatorID)
		return nil
	}
	if r.coreDefaults[platform] == nil {
		r.coreDefaults[platform] = make(map[string]string)
	}
	r.coreDefaults[platform][emulatorID] = coreID
	return nil
}

func (r *memoryRepository) ListDefaults(context.Context, string) (map[core.Platform]string, error) {
	result := make(map[core.Platform]string, len(r.defaults))
	for platform, emulatorID := range r.defaults {
		result[platform] = emulatorID
	}
	return result, nil
}

func (r *memoryRepository) SetDefault(_ context.Context, _ string, platform core.Platform, emulatorID, _ string, _ time.Time) error {
	if emulatorID == "" {
		delete(r.defaults, platform)
	} else {
		r.defaults[platform] = emulatorID
	}
	return nil
}

type endpointStub struct {
	endpoint devices.Endpoint
	required []devicev1.AccessLevel
	err      error
}

func (s *endpointStub) AuthorizeEndpoint(_ context.Context, _, _ string, required devicev1.AccessLevel) error {
	s.required = append(s.required, required)
	return s.err
}

func (s *endpointStub) ListEndpoints(context.Context, string) ([]devices.Endpoint, error) {
	return []devices.Endpoint{s.endpoint}, nil
}

func TestConfigurationPreservesMultipleEmulatorsAndSeparatesDefault(t *testing.T) {
	catalog, err := NewCatalog([]Emulator{
		{ID: "first", Name: "First", Platforms: []core.Platform{core.PlatformPS1}, AdapterState: "ready"},
		{ID: "second", Name: "Second", Platforms: []core.Platform{core.PlatformPS1}, AdapterState: "ready"},
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := &memoryRepository{defaults: map[core.Platform]string{core.PlatformPS1: "second"}}
	endpoint := &endpointStub{endpoint: devices.Endpoint{
		ID: "endpoint", Status: devicev1.EndpointReady, Capabilities: []string{devicev1.CapabilityGameLaunchEmulator},
		Inventory: &devicev1.DeviceInventory{Runtimes: []devicev1.RuntimeInventory{{ID: "first", Name: "First"}, {ID: "second", Name: "Second"}}},
	}}
	service, _ := NewService(repository, endpoint, catalog)

	configuration, err := service.Get(context.Background(), "endpoint", "profile")
	if err != nil {
		t.Fatal(err)
	}
	platform := configuration.Platforms[0]
	if len(platform.Emulators) != 2 || platform.SelectedDefault != "second" || platform.ResolvedDefault != "second" {
		t.Fatalf("configuration = %#v", platform)
	}
	if endpoint.required[0] != devicev1.AccessView {
		t.Fatalf("read authorization = %q", endpoint.required[0])
	}
}

func TestSetDefaultRequiresOwnerAndRejectsIncompatibleEmulator(t *testing.T) {
	repository := &memoryRepository{defaults: map[core.Platform]string{}}
	endpoint := &endpointStub{endpoint: devices.Endpoint{ID: "endpoint"}}
	service, _ := NewService(repository, endpoint, NewDefaultCatalog())

	_, err := service.SetDefault(context.Background(), "endpoint", "profile", core.PlatformPS2, "duckstation")
	if !errors.Is(err, ErrInvalidPreference) {
		t.Fatalf("SetDefault error = %v, want invalid preference", err)
	}
	if len(endpoint.required) != 1 || endpoint.required[0] != devicev1.AccessOwner {
		t.Fatalf("write authorization = %v", endpoint.required)
	}
}

func TestSelectedUnavailableDefaultFallsBackWithoutRewriting(t *testing.T) {
	repository := &memoryRepository{defaults: map[core.Platform]string{core.PlatformPS1: "duckstation"}}
	endpoint := &endpointStub{endpoint: devices.Endpoint{
		ID: "endpoint", Status: devicev1.EndpointReady, Capabilities: []string{devicev1.CapabilityGameLaunchEmulator},
		Inventory: &devicev1.DeviceInventory{Runtimes: []devicev1.RuntimeInventory{{ID: "retroarch", Name: "RetroArch"}}},
	}}
	service, _ := NewService(repository, endpoint, NewDefaultCatalog())
	configuration, err := service.Get(context.Background(), "endpoint", "profile")
	if err != nil {
		t.Fatal(err)
	}
	for _, platform := range configuration.Platforms {
		if platform.Platform == core.PlatformPS1 {
			if platform.SelectedDefault != "duckstation" || platform.ResolvedDefault != "" {
				t.Fatalf("PS1 defaults = %#v", platform)
			}
			if repository.defaults[core.PlatformPS1] != "duckstation" {
				t.Fatal("stored default was rewritten")
			}
			return
		}
	}
	t.Fatal("PS1 platform missing")
}

func TestRetroArchReadinessUsesDetectedCoreAndPersistsPerPlatformChoice(t *testing.T) {
	repository := &memoryRepository{defaults: map[core.Platform]string{}, coreDefaults: map[core.Platform]map[string]string{}}
	endpoint := &endpointStub{endpoint: devices.Endpoint{
		ID: "endpoint", Status: devicev1.EndpointReady, Capabilities: []string{devicev1.CapabilityGameLaunchEmulator},
		Inventory: &devicev1.DeviceInventory{Runtimes: []devicev1.RuntimeInventory{{
			ID: "retroarch", Name: "RetroArch", CoreProbeState: "complete",
			Components: []devicev1.RuntimeComponentInventory{{Kind: "core", ID: "fceumm", Name: "FCEUmm"}, {Kind: "core", ID: "mesen", Name: "Mesen"}},
		}}, SaveAdapters: []devicev1.SaveAdapterInventory{{ID: "retroarch", Name: "RetroArch", ProbeState: "complete", SaveKinds: []string{"save_ram", "save_state"}, RouteOverrides: true}}},
	}}
	service, _ := NewService(repository, endpoint, NewDefaultCatalog())
	configuration, err := service.SetCoreDefault(context.Background(), "endpoint", "profile", core.PlatformNES, "retroarch", "mesen")
	if err != nil {
		t.Fatal(err)
	}
	for _, platform := range configuration.Platforms {
		if platform.Platform != core.PlatformNES {
			continue
		}
		for _, option := range platform.Emulators {
			if option.ID == "retroarch" {
				if option.State != "ready" || option.SelectedCore != "mesen" || option.ResolvedCore != "mesen" || option.SaveProbeState != "complete" || !option.SaveRouteOverrides || len(option.SaveKinds) != 2 {
					t.Fatalf("RetroArch option = %#v", option)
				}
				return
			}
		}
	}
	t.Fatal("NES RetroArch option missing")
}

func TestPrepareSetupRequiresOwnerCapabilityAndWinget(t *testing.T) {
	repository := &memoryRepository{defaults: map[core.Platform]string{}}
	endpoint := &endpointStub{endpoint: devices.Endpoint{
		ID: "endpoint", Status: devicev1.EndpointReady, Capabilities: []string{devicev1.CapabilityEmulatorSetup},
		Inventory: &devicev1.DeviceInventory{PackageManagers: []devicev1.PackageManagerInventory{{ID: "winget", Name: "Windows Package Manager"}}},
	}}
	service, _ := NewService(repository, endpoint, NewDefaultCatalog())
	request, err := service.PrepareSetup(context.Background(), "endpoint", "profile", "retroarch", "install")
	if err != nil {
		t.Fatal(err)
	}
	if request.EmulatorID != "retroarch" || request.Action != "install" {
		t.Fatalf("request = %#v", request)
	}
	if endpoint.required[0] != devicev1.AccessOwner {
		t.Fatalf("authorization = %v", endpoint.required)
	}
	endpoint.endpoint.Inventory.PackageManagers = nil
	if _, err := service.PrepareSetup(context.Background(), "endpoint", "profile", "retroarch", "install"); !errors.Is(err, ErrSetupUnavailable) {
		t.Fatalf("missing winget error = %v", err)
	}
}

func TestRequiredFirmwareUsesOnlyTypedInventoryFact(t *testing.T) {
	spec := CoreSpec{FirmwarePolicy: "user_required", FirmwareRequirementID: "ps1_bios"}
	runtime := devicev1.RuntimeInventory{FirmwareProbeState: "complete"}
	if got := firmwareStateForCore(core.PlatformPS1, spec, runtime); got != "missing" {
		t.Fatalf("firmware without fact = %q", got)
	}
	runtime.Components = []devicev1.RuntimeComponentInventory{{Kind: "firmware", ID: "ps1_bios", Name: "PlayStation BIOS set"}}
	if got := firmwareStateForCore(core.PlatformPS1, spec, runtime); got != "present" {
		t.Fatalf("firmware with fact = %q", got)
	}
	runtime.FirmwareProbeState = "partial"
	runtime.Components = nil
	if got := firmwareStateForCore(core.PlatformPS1, spec, runtime); got != "unknown" {
		t.Fatalf("partial firmware probe = %q", got)
	}
}

func TestMissingRetroArchStillListsCompatibleCoreChoices(t *testing.T) {
	repository := &memoryRepository{defaults: map[core.Platform]string{}}
	endpoint := &endpointStub{endpoint: devices.Endpoint{ID: "endpoint", Status: devicev1.EndpointReady}}
	service, _ := NewService(repository, endpoint, NewDefaultCatalog())
	configuration, err := service.Get(context.Background(), "endpoint", "profile")
	if err != nil {
		t.Fatal(err)
	}
	for _, platform := range configuration.Platforms {
		if platform.Platform != core.PlatformNES {
			continue
		}
		for _, option := range platform.Emulators {
			if option.ID == "retroarch" {
				if option.Detected || len(option.Cores) == 0 {
					t.Fatalf("missing RetroArch option = %#v", option)
				}
				return
			}
		}
	}
	t.Fatal("NES RetroArch option missing")
}
