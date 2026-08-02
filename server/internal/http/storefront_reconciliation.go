package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
)

const (
	storefrontReconciliationTick          = 30 * time.Second
	storefrontReconciliationInterval      = 15 * time.Minute
	storefrontReconciliationRetryInterval = time.Minute
)

type storefrontReconciliationState struct {
	activeCommandID string
	nextRunAt       time.Time
}

// StorefrontReconciliationService periodically dispatches the exact same
// candidate-bounded inventory command as the manual Scan storage and apps
// action. It intentionally keeps no product inventory of its own.
type StorefrontReconciliationService struct {
	devices  *devices.Service
	profiles core.ProfileRepository
	games    core.GameStore
	logger   core.Logger
	now      func() time.Time

	mu     sync.Mutex
	states map[string]storefrontReconciliationState
}

func NewStorefrontReconciliationService(deviceService *devices.Service, profiles core.ProfileRepository, games core.GameStore, logger core.Logger) (*StorefrontReconciliationService, error) {
	if deviceService == nil || profiles == nil || games == nil || logger == nil {
		return nil, errors.New("device service, profiles, game store, and logger are required")
	}
	return &StorefrontReconciliationService{
		devices: deviceService, profiles: profiles, games: games, logger: logger,
		now: time.Now, states: make(map[string]storefrontReconciliationState),
	}, nil
}

func (s *StorefrontReconciliationService) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("storefront reconciliation context is required")
	}
	if err := s.tick(ctx); err != nil {
		return fmt.Errorf("initialize storefront reconciliation: %w", err)
	}
	go s.run(ctx)
	return nil
}

func (s *StorefrontReconciliationService) run(ctx context.Context) {
	ticker := time.NewTicker(storefrontReconciliationTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				s.logger.Error("storefront reconciliation scheduler", err)
			}
		}
	}
}

func (s *StorefrontReconciliationService) tick(ctx context.Context) error {
	profiles, err := s.profiles.List(ctx)
	if err != nil {
		return fmt.Errorf("list profiles: %w", err)
	}
	for _, profile := range profiles {
		if profile == nil || profile.ID == "" {
			continue
		}
		profileCtx := core.WithProfile(ctx, profile)
		if err := s.tickProfile(profileCtx, profile.ID); err != nil {
			s.logger.Error("storefront reconciliation profile", err, "profile_id", profile.ID)
		}
	}
	return nil
}

func (s *StorefrontReconciliationService) tickProfile(ctx context.Context, profileID string) error {
	candidates, err := buildStorefrontCandidates(ctx, s.games)
	if err != nil {
		return fmt.Errorf("build storefront candidates: %w", err)
	}
	if len(candidates) == 0 {
		return nil
	}
	payload, err := json.Marshal(devicev1.InventoryRefreshRequest{StorefrontCandidates: candidates})
	if err != nil {
		return fmt.Errorf("encode storefront candidates: %w", err)
	}
	endpoints, err := s.devices.ListEndpoints(ctx, profileID)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	for index := range endpoints {
		endpoint := &endpoints[index]
		key := storefrontReconciliationKey(profileID, endpoint.ID)
		state := s.state(key)
		if state.activeCommandID != "" {
			command, commandErr := s.devices.GetCommand(ctx, endpoint.ID, profileID, state.activeCommandID)
			if commandErr != nil || terminalDeviceCommand(command.Status) {
				s.saveState(key, storefrontReconciliationState{nextRunAt: now.Add(storefrontReconciliationInterval)})
			}
			continue
		}
		if !state.nextRunAt.IsZero() && now.Before(state.nextRunAt) {
			continue
		}
		if endpoint.Status != devicev1.EndpointReady || !hasEndpointCapability(endpoint.Capabilities, devicev1.CapabilityInventoryRefresh) {
			s.saveState(key, storefrontReconciliationState{nextRunAt: now.Add(storefrontReconciliationRetryInterval)})
			continue
		}
		command, dispatchErr := s.devices.DispatchCommand(ctx, endpoint.ID, profileID, devicev1.CapabilityInventoryRefresh, payload)
		if dispatchErr != nil {
			s.saveState(key, storefrontReconciliationState{nextRunAt: now.Add(storefrontReconciliationRetryInterval)})
			s.logger.Error("dispatch storefront reconciliation", dispatchErr, "profile_id", profileID, "endpoint_id", endpoint.ID)
			continue
		}
		s.saveState(key, storefrontReconciliationState{activeCommandID: command.ID})
	}
	return nil
}

func storefrontReconciliationKey(profileID, endpointID string) string {
	return profileID + "\x00" + endpointID
}

func (s *StorefrontReconciliationService) state(key string) storefrontReconciliationState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.states[key]
}

func (s *StorefrontReconciliationService) saveState(key string, state storefrontReconciliationState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[key] = state
}
