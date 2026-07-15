package http

import (
	"context"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
)

type availabilityDeviceLister struct {
	endpoints []devices.Endpoint
}

func (l availabilityDeviceLister) ListEndpoints(context.Context, string) ([]devices.Endpoint, error) {
	return l.endpoints, nil
}

func TestAttachDeviceAvailabilityUsesInventoryAndRuntimeFacts(t *testing.T) {
	t.Parallel()
	now := time.Now()
	controller := &GameController{
		logger: noopLogger{},
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
	controller.attachDeviceAvailability(ctx, &response, &core.CanonicalGame{ID: "game-1", Platform: core.PlatformGenesis})
	if len(response.Devices) != 2 {
		t.Fatalf("devices = %#v", response.Devices)
	}
	ready := response.Devices[0]
	if ready.Status != "ready_for_setup" || !ready.RuntimeAvailable || ready.RequiredRuntimeID != "retroarch" || ready.FreeBytes != 40 || !ready.CanManage {
		t.Fatalf("ready device = %#v", ready)
	}
	if response.Devices[1].Status != "offline" || response.Devices[1].CanManage {
		t.Fatalf("offline device = %#v", response.Devices[1])
	}
}

func TestLocalRuntimeRequirementDoesNotGuessUnsupportedPlatforms(t *testing.T) {
	t.Parallel()
	if _, _, supported := localRuntimeRequirement(core.PlatformPS3); supported {
		t.Fatal("PS3 was marked supported without an implemented runtime route")
	}
}
