package devices

import (
	"errors"
	"testing"
	"time"
)

func TestClientLaunchRegistryLifecycle(t *testing.T) {
	t.Parallel()

	registry := NewClientLaunchRegistry()
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	token, launch, err := registry.Create("profile-1", now)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if launch.Status != ClientLaunchWaiting || token == "" {
		t.Fatalf("Create() = token %q, status %q", token, launch.Status)
	}
	if _, err := registry.Get(launch.ID, "profile-2", now); !errors.Is(err, ErrClientLaunchNotFound) {
		t.Fatalf("Get() wrong profile error = %v", err)
	}
	acknowledged, err := registry.Redeem(launch.ID, token, "endpoint-1", now)
	if err != nil {
		t.Fatalf("Redeem() error = %v", err)
	}
	if acknowledged.Status != ClientLaunchAcknowledged || acknowledged.EndpointID != "endpoint-1" {
		t.Fatalf("Redeem() = %#v", acknowledged)
	}
	if _, err := registry.Redeem(launch.ID, token, "endpoint-1", now); !errors.Is(err, ErrClientLaunchUsed) {
		t.Fatalf("second Redeem() error = %v", err)
	}
}

func TestClientLaunchRegistryExpires(t *testing.T) {
	t.Parallel()

	registry := NewClientLaunchRegistry()
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	token, launch, err := registry.Create("profile-1", now)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := registry.Redeem(launch.ID, token, "endpoint-1", now.Add(clientLaunchLifetime)); !errors.Is(err, ErrClientLaunchExpired) {
		t.Fatalf("Redeem() expired error = %v", err)
	}
	got, err := registry.Get(launch.ID, "profile-1", now.Add(clientLaunchLifetime))
	if err != nil || got.Status != ClientLaunchExpired {
		t.Fatalf("Get() expired = %#v, %v", got, err)
	}
}
