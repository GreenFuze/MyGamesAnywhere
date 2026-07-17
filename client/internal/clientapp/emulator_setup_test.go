package clientapp

import (
	"context"
	"reflect"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type setupInventorySequence struct {
	values []devicev1.DeviceInventory
	index  int
}

func (s *setupInventorySequence) Collect(context.Context) (devicev1.DeviceInventory, error) {
	value := s.values[s.index]
	if s.index < len(s.values)-1 {
		s.index++
	}
	return value, nil
}

type recordingSetupRunner struct {
	packageID string
	action    string
}

func (r *recordingSetupRunner) Run(_ context.Context, packageID, action string) error {
	r.packageID, r.action = packageID, action
	return nil
}

func TestManagedEmulatorSetupUsesCompiledPackageAndRefreshesInventory(t *testing.T) {
	inventory := &setupInventorySequence{values: []devicev1.DeviceInventory{
		{SchemaVersion: 2, CapturedAt: time.Now()},
		{SchemaVersion: 2, CapturedAt: time.Now(), Runtimes: []devicev1.RuntimeInventory{{ID: "retroarch", Name: "RetroArch", Version: "1.22.2"}}},
	}}
	manager, err := NewManagedEmulatorSetupManager(inventory)
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingSetupRunner{}
	manager.runner = runner
	result, err := manager.Setup(context.Background(), devicev1.EmulatorSetupRequest{EmulatorID: "retroarch", Action: "install"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual([]string{runner.packageID, runner.action}, []string{"Libretro.RetroArch", "install"}) || result.State != "installed" || !result.Changed {
		t.Fatalf("runner=%#v result=%#v", runner, result)
	}
}

func TestManagedEmulatorSetupRejectsUnknownEmulatorBeforeRunner(t *testing.T) {
	manager, _ := NewManagedEmulatorSetupManager(&setupInventorySequence{values: []devicev1.DeviceInventory{{}}})
	runner := &recordingSetupRunner{}
	manager.runner = runner
	_, err := manager.Setup(context.Background(), devicev1.EmulatorSetupRequest{EmulatorID: "custom", Action: "install"}, nil)
	if err == nil || runner.packageID != "" {
		t.Fatalf("err=%v runner=%#v", err, runner)
	}
}
