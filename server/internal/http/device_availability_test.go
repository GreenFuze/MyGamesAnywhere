package http

import (
	"context"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/emulation"
)

type availabilityDeviceLister struct {
	endpoints []devices.Endpoint
}

type availabilityEmulationProvider struct {
	configurations map[string]emulation.DeviceConfiguration
}

func (p availabilityEmulationProvider) Get(_ context.Context, endpointID, _ string) (emulation.DeviceConfiguration, error) {
	return p.configurations[endpointID], nil
}

func (p availabilityEmulationProvider) GetForEndpoint(_ context.Context, endpoint devices.Endpoint, _ string) (emulation.DeviceConfiguration, error) {
	return p.configurations[endpoint.ID], nil
}

func (l availabilityDeviceLister) ListEndpoints(context.Context, string) ([]devices.Endpoint, error) {
	return l.endpoints, nil
}

func TestAttachDeviceAvailabilityUsesInventoryAndRuntimeFacts(t *testing.T) {
	t.Parallel()
	now := time.Now()
	controller := &GameController{
		logger: noopLogger{},
		emulation: availabilityEmulationProvider{configurations: map[string]emulation.DeviceConfiguration{
			"device-ready": {EndpointID: "device-ready", Platforms: []emulation.PlatformConfiguration{{
				Platform: core.PlatformGenesis, ResolvedDefault: "retroarch",
				Emulators: []emulation.Option{{Emulator: emulation.Emulator{ID: "retroarch", Name: "RetroArch"}, Detected: true, State: "needs_setup", Reason: "Choose a core"}},
			}}},
		}},
		deviceLister: availabilityDeviceLister{endpoints: []devices.Endpoint{
			{
				ID: "device-ready", DisplayName: "Living room PC", OSUser: "alice", Platform: "windows",
				Status: devicev1.EndpointReady, AccessLevel: devicev1.AccessManage,
				Inventory: &devicev1.DeviceInventory{
					SchemaVersion: devicev1.InventorySchemaVersion,
					CapturedAt:    now,
					Storage:       []devicev1.StorageInventory{{ID: "c", Root: `C:\`, TotalBytes: 100, FreeBytes: 40}},
					Runtimes:      []devicev1.RuntimeInventory{{ID: "retroarch", Name: "RetroArch"}},
				},
			},
			{ID: "device-offline", DisplayName: "Office PC", OSUser: "bob", Platform: "windows", Status: devicev1.EndpointOffline, AccessLevel: devicev1.AccessView},
		}},
	}
	ctx := core.WithProfile(context.Background(), &core.Profile{ID: "profile-1"})
	response := GameDetailResponse{}
	controller.attachDeviceAvailability(ctx, &response, &core.CanonicalGame{ID: "game-1", Platform: core.PlatformGenesis, SourceGames: []*core.SourceGame{{ID: "source", RawTitle: "Game", Status: "found", GroupKind: core.GroupKindSelfContained, RootPath: `C:\Games\Game`, Files: []core.GameFile{{Path: "game.rom"}}}}})
	if len(response.Devices) != 2 {
		t.Fatalf("devices = %#v", response.Devices)
	}
	ready := response.Devices[0]
	if ready.Status != "needs_setup" || len(ready.EmulatorRoutes) != 1 || ready.EmulatorRoutes[0].EmulatorID != "retroarch" || ready.FreeBytes != 40 || !ready.CanManage {
		t.Fatalf("ready device = %#v", ready)
	}
	if response.Devices[1].Status != "offline" || response.Devices[1].CanManage {
		t.Fatalf("offline device = %#v", response.Devices[1])
	}
}

func TestAttachDeviceAvailabilityMapsFailedCleanupStateAndCapability(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	controller := &GameController{
		logger: noopLogger{},
		deviceLister: availabilityDeviceLister{endpoints: []devices.Endpoint{{
			ID: "device-1", DisplayName: "PC", OSUser: "alice", Platform: "windows", Status: devicev1.EndpointReady,
			AccessLevel: devicev1.AccessManage, Capabilities: []string{devicev1.CapabilityGameCleanupGogInnoFailed},
			Installations: []devices.GameInstallation{{
				GameID: "game-1", SourceGameID: "source-1", InstallPath: `C:\Games\Failed`, InstallKind: devicev1.InstallKindGogInno,
				InstallState: devicev1.InstallStateIgnoredFailure, CleanupMarkerID: "marker", CleanupIgnoredAt: &now,
			}},
		}}},
	}
	response := GameDetailResponse{}
	controller.attachDeviceAvailability(core.WithProfile(context.Background(), &core.Profile{ID: "profile-1"}), &response,
		&core.CanonicalGame{ID: "game-1", Platform: core.PlatformWindowsPC})
	if len(response.Devices) != 1 {
		t.Fatalf("devices = %#v", response.Devices)
	}
	device := response.Devices[0]
	if !device.Installed || device.Status != devicev1.InstallStateIgnoredFailure || device.InstallState != devicev1.InstallStateIgnoredFailure ||
		!device.FailedCleanupSupported || device.CleanupMarkerID != "marker" || device.CleanupIgnoredAt == "" {
		t.Fatalf("failed device availability = %#v", device)
	}
}
