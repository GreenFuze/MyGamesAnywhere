package clientapp

import (
	"context"
	"os"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type preflightInventory struct{ runtimes []devicev1.RuntimeInventory }

func (i preflightInventory) Collect(context.Context) (devicev1.DeviceInventory, error) {
	return devicev1.DeviceInventory{SchemaVersion: 1, CapturedAt: time.Now(), Storage: []devicev1.StorageInventory{{ID: "test", Root: os.TempDir(), TotalBytes: 1, FreeBytes: 1}}, Runtimes: i.runtimes}, nil
}

func TestPreflightNativeInstallerDelegatesPrerequisites(t *testing.T) {
	evaluator := NewInstallationPreflightEvaluator(preflightInventory{})
	result, err := evaluator.Evaluate(context.Background(), devicev1.InstallationPreflightRequest{
		SchemaVersion: 1, GameID: "game-1", SourceGameID: "source-1", Category: devicev1.InstallationCategoryNativeInstaller, DestinationRoot: os.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanInstall || result.Checks[1].Status != devicev1.PreflightCheckInstallerManaged {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestPreflightAllowListedRuntimeDetection(t *testing.T) {
	evaluator := NewInstallationPreflightEvaluator(preflightInventory{runtimes: []devicev1.RuntimeInventory{{ID: "steam", Name: "Steam"}}})
	result, err := evaluator.Evaluate(context.Background(), devicev1.InstallationPreflightRequest{
		SchemaVersion: 1, GameID: "game-1", SourceGameID: "source-1", Category: devicev1.InstallationCategoryStorefront, DestinationRoot: os.TempDir(),
		Requirements: []devicev1.PrerequisiteRequirement{
			{ID: "storefront.steam", Name: "Steam", Kind: devicev1.PrerequisiteKindStorefront, Required: true},
			{ID: "storefront.xbox", Name: "Xbox", Kind: devicev1.PrerequisiteKindStorefront, Required: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Checks[1].Status != devicev1.PreflightCheckReady || result.Checks[2].Status != devicev1.PreflightCheckUnknown || !result.CanInstall {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestPreflightBlocksDefiniteInsufficientStorage(t *testing.T) {
	evaluator := NewInstallationPreflightEvaluator(preflightInventory{})
	evaluator.diskFree = func(string) (uint64, error) { return 99, nil }
	result, err := evaluator.Evaluate(context.Background(), devicev1.InstallationPreflightRequest{
		SchemaVersion: 1, GameID: "game-1", SourceGameID: "source-1", Category: devicev1.InstallationCategoryManagedArchive,
		DestinationRoot: os.TempDir(), RequiredStorageBytes: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CanInstall || result.Checks[0].Status != devicev1.PreflightCheckMissing {
		t.Fatalf("unexpected result: %#v", result)
	}
}
