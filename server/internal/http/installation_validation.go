package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdhttp "net/http"
	"sort"
	"sync"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/devices"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/events"
	"github.com/go-chi/chi/v5"
)

const (
	installationValidationScheduleSetting = "installation_validation_schedule"
	defaultInstallationValidationInterval = 15
	minimumInstallationValidationInterval = 5
	maximumInstallationValidationInterval = 24 * 60
	installationValidationTick            = 15 * time.Second
	installationValidationInitialDelay    = time.Minute
	installationValidationRetryDelay      = time.Minute
)

type InstallationValidationScheduleConfig struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"interval_minutes"`
}

type InstallationValidationEndpointStatus struct {
	EndpointID      string `json:"endpoint_id"`
	DisplayName     string `json:"display_name"`
	State           string `json:"state"`
	NextRunAt       string `json:"next_run_at,omitempty"`
	LastStartedAt   string `json:"last_started_at,omitempty"`
	LastFinishedAt  string `json:"last_finished_at,omitempty"`
	LastStatus      string `json:"last_status,omitempty"`
	LastError       string `json:"last_error,omitempty"`
	ActiveCommandID string `json:"active_command_id,omitempty"`
	Installed       int    `json:"installed,omitempty"`
	Missing         int    `json:"missing,omitempty"`
	NeedsRepair     int    `json:"needs_repair,omitempty"`
	EligibleCount   int    `json:"eligible_count"`
}

type InstallationValidationScheduleStatus struct {
	InstallationValidationScheduleConfig
	Devices []InstallationValidationEndpointStatus `json:"devices"`
}

type installationValidationState struct {
	status InstallationValidationEndpointStatus
}

type InstallationValidationService struct {
	devices  *devices.Service
	profiles core.ProfileRepository
	settings core.SettingRepository
	logger   core.Logger
	eventBus *events.EventBus
	now      func() time.Time

	mu     sync.RWMutex
	states map[string]*installationValidationState
}

func NewInstallationValidationService(deviceService *devices.Service, profiles core.ProfileRepository, settings core.SettingRepository, logger core.Logger, eventBus *events.EventBus) (*InstallationValidationService, error) {
	if deviceService == nil || profiles == nil || settings == nil || logger == nil {
		return nil, errors.New("device service, profiles, settings, and logger are required")
	}
	return &InstallationValidationService{
		devices: deviceService, profiles: profiles, settings: settings, logger: logger,
		eventBus: eventBus, now: time.Now, states: make(map[string]*installationValidationState),
	}, nil
}

func (s *InstallationValidationService) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("installation validation context is required")
	}
	if err := s.tick(ctx); err != nil {
		return fmt.Errorf("initialize installation validation: %w", err)
	}
	go s.run(ctx)
	return nil
}

func (s *InstallationValidationService) run(ctx context.Context) {
	ticker := time.NewTicker(installationValidationTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				s.logger.Error("installation validation scheduler", err)
			}
		}
	}
}

func (s *InstallationValidationService) tick(ctx context.Context) error {
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
			s.logger.Error("installation validation profile", err, "profile_id", profile.ID)
		}
	}
	return nil
}

func (s *InstallationValidationService) tickProfile(ctx context.Context, profileID string) error {
	config, err := s.loadConfig(ctx)
	if err != nil {
		return err
	}
	endpoints, err := s.devices.ListEndpoints(ctx, profileID)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	for index := range endpoints {
		endpoint := &endpoints[index]
		eligible := eligibleInstallationCount(endpoint.Installations)
		state := s.ensureState(profileID, endpoint.ID, endpoint.DisplayName, eligible, now)
		if state.status.ActiveCommandID != "" {
			command, commandErr := s.devices.GetCommand(ctx, endpoint.ID, profileID, state.status.ActiveCommandID)
			if commandErr != nil {
				s.finish(profileID, endpoint.ID, nil, config, now, commandErr.Error())
				continue
			}
			if !terminalDeviceCommand(command.Status) {
				s.updateState(profileID, endpoint.ID, func(status *InstallationValidationEndpointStatus) {
					status.State = "running"
				})
				continue
			}
			s.finish(profileID, endpoint.ID, command, config, now, "")
			continue
		}
		if !config.Enabled {
			s.updateState(profileID, endpoint.ID, func(status *InstallationValidationEndpointStatus) {
				status.State, status.NextRunAt = "disabled", ""
			})
			continue
		}
		if eligible == 0 {
			s.updateState(profileID, endpoint.ID, func(status *InstallationValidationEndpointStatus) {
				status.State, status.NextRunAt = "no_installations", ""
			})
			continue
		}
		if endpoint.Status == devicev1.EndpointOffline || endpoint.Status == devicev1.EndpointBusy || endpoint.Status == devicev1.EndpointUpdateRequired || !hasEndpointCapability(endpoint.Capabilities, devicev1.CapabilityGameValidateInstallations) {
			s.updateState(profileID, endpoint.ID, func(status *InstallationValidationEndpointStatus) {
				status.State = "waiting"
				status.NextRunAt = now.Add(installationValidationRetryDelay).Format(time.RFC3339)
			})
			continue
		}
		nextRun, parseErr := time.Parse(time.RFC3339, state.status.NextRunAt)
		if parseErr != nil {
			return fmt.Errorf("parse installation validation next run: %w", parseErr)
		}
		if now.Before(nextRun) {
			continue
		}
		if _, err := s.start(ctx, profileID, endpoint.ID, "background", now); err != nil {
			s.updateState(profileID, endpoint.ID, func(status *InstallationValidationEndpointStatus) {
				status.State, status.LastStatus, status.LastError = "waiting", "failed", err.Error()
				status.NextRunAt = now.Add(installationValidationRetryDelay).Format(time.RFC3339)
			})
		}
	}
	return nil
}

func (s *InstallationValidationService) start(ctx context.Context, profileID, endpointID, trigger string, now time.Time) (*devices.Command, error) {
	state := s.getState(profileID, endpointID)
	if state != nil && state.status.ActiveCommandID != "" {
		return s.devices.GetCommand(ctx, endpointID, profileID, state.status.ActiveCommandID)
	}
	command, err := s.devices.DispatchInstallationValidation(ctx, endpointID, profileID, trigger)
	if err != nil {
		return nil, err
	}
	s.updateState(profileID, endpointID, func(status *InstallationValidationEndpointStatus) {
		status.State = "running"
		status.ActiveCommandID = command.ID
		status.LastStartedAt = now.UTC().Format(time.RFC3339)
		status.NextRunAt = ""
		status.LastError = ""
	})
	total := 0
	if current := s.getState(profileID, endpointID); current != nil {
		total = current.status.EligibleCount
	}
	events.PublishJSON(s.eventBus, "installation_validation_started", map[string]any{
		"profile_id": profileID, "endpoint_id": endpointID, "command_id": command.ID, "trigger": trigger,
		"total": total,
	})
	return command, nil
}

func (s *InstallationValidationService) RunNow(ctx context.Context, endpointID, profileID string) (*devices.Command, error) {
	endpoints, err := s.devices.ListEndpoints(ctx, profileID)
	if err != nil {
		return nil, err
	}
	for index := range endpoints {
		endpoint := &endpoints[index]
		if endpoint.ID != endpointID {
			continue
		}
		if endpoint.Status != devicev1.EndpointReady {
			return nil, fmt.Errorf("device must be ready before checking installed games")
		}
		if !hasEndpointCapability(endpoint.Capabilities, devicev1.CapabilityGameValidateInstallations) {
			return nil, fmt.Errorf("%w: %s", devices.ErrCapabilityMissing, devicev1.CapabilityGameValidateInstallations)
		}
		s.ensureState(profileID, endpoint.ID, endpoint.DisplayName, eligibleInstallationCount(endpoint.Installations), s.now().UTC())
		return s.start(ctx, profileID, endpointID, "manual", s.now().UTC())
	}
	return nil, devices.ErrEndpointNotFound
}

func (s *InstallationValidationService) Status(ctx context.Context) (*InstallationValidationScheduleStatus, error) {
	profileID := core.ProfileIDFromContext(ctx)
	if profileID == "" {
		return nil, core.ErrProfileRequired
	}
	config, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	endpoints, err := s.devices.ListEndpoints(ctx, profileID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	response := &InstallationValidationScheduleStatus{InstallationValidationScheduleConfig: config, Devices: []InstallationValidationEndpointStatus{}}
	for index := range endpoints {
		endpoint := &endpoints[index]
		state := s.ensureState(profileID, endpoint.ID, endpoint.DisplayName, eligibleInstallationCount(endpoint.Installations), now)
		response.Devices = append(response.Devices, state.status)
	}
	sort.Slice(response.Devices, func(i, j int) bool { return response.Devices[i].DisplayName < response.Devices[j].DisplayName })
	return response, nil
}

func (s *InstallationValidationService) UpdateConfig(ctx context.Context, config InstallationValidationScheduleConfig) (*InstallationValidationScheduleStatus, error) {
	profileID := core.ProfileIDFromContext(ctx)
	if profileID == "" {
		return nil, core.ErrProfileRequired
	}
	if err := validateInstallationValidationConfig(config); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	if err := s.settings.Upsert(ctx, &core.Setting{Key: installationValidationScheduleSetting, Value: string(raw), UpdatedAt: s.now()}); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	s.mu.Lock()
	for key, state := range s.states {
		if !stateKeyHasProfile(key, profileID) || state.status.ActiveCommandID != "" {
			continue
		}
		if config.Enabled {
			state.status.State = "scheduled"
			state.status.NextRunAt = now.Add(installationValidationInitialDelay).Format(time.RFC3339)
		} else {
			state.status.State = "disabled"
			state.status.NextRunAt = ""
		}
	}
	s.mu.Unlock()
	events.PublishJSON(s.eventBus, "installation_validation_schedule_updated", map[string]any{
		"profile_id": profileID, "enabled": config.Enabled, "interval_minutes": config.IntervalMinutes,
	})
	return s.Status(ctx)
}

func (s *InstallationValidationService) finish(profileID, endpointID string, command *devices.Command, config InstallationValidationScheduleConfig, now time.Time, fallbackError string) {
	s.updateState(profileID, endpointID, func(status *InstallationValidationEndpointStatus) {
		status.ActiveCommandID = ""
		status.LastFinishedAt = now.UTC().Format(time.RFC3339)
		status.State = "scheduled"
		status.LastError = fallbackError
		status.LastStatus = "failed"
		if command != nil {
			status.LastStatus = string(command.Status)
			status.LastError = command.ErrorMessage
			if command.Status == devicev1.CommandSucceeded {
				var result devicev1.InstallationValidationResult
				if json.Unmarshal(command.Result, &result) == nil {
					status.Installed, status.Missing, status.NeedsRepair = result.Installed, result.Missing, result.NeedsRepair
				}
			}
		}
		if config.Enabled {
			status.NextRunAt = now.Add(time.Duration(config.IntervalMinutes) * time.Minute).Format(time.RFC3339)
		} else {
			status.State, status.NextRunAt = "disabled", ""
		}
	})
}

func (s *InstallationValidationService) loadConfig(ctx context.Context) (InstallationValidationScheduleConfig, error) {
	config := InstallationValidationScheduleConfig{Enabled: true, IntervalMinutes: defaultInstallationValidationInterval}
	setting, err := s.settings.Get(ctx, installationValidationScheduleSetting)
	if err != nil {
		return config, err
	}
	if setting == nil || setting.Value == "" {
		return config, nil
	}
	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		return config, err
	}
	return config, validateInstallationValidationConfig(config)
}

func validateInstallationValidationConfig(config InstallationValidationScheduleConfig) error {
	if config.IntervalMinutes < minimumInstallationValidationInterval || config.IntervalMinutes > maximumInstallationValidationInterval {
		return fmt.Errorf("interval_minutes must be between %d and %d", minimumInstallationValidationInterval, maximumInstallationValidationInterval)
	}
	return nil
}

func (s *InstallationValidationService) ensureState(profileID, endpointID, displayName string, eligible int, now time.Time) installationValidationState {
	key := installationValidationStateKey(profileID, endpointID)
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[key]
	if state == nil {
		state = &installationValidationState{status: InstallationValidationEndpointStatus{
			EndpointID: endpointID, DisplayName: displayName, State: "scheduled", EligibleCount: eligible,
			NextRunAt: now.Add(installationValidationInitialDelay).Format(time.RFC3339),
		}}
		s.states[key] = state
	}
	state.status.DisplayName = displayName
	state.status.EligibleCount = eligible
	return *state
}

func (s *InstallationValidationService) getState(profileID, endpointID string) *installationValidationState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := s.states[installationValidationStateKey(profileID, endpointID)]
	if state == nil {
		return nil
	}
	copy := *state
	return &copy
}

func (s *InstallationValidationService) updateState(profileID, endpointID string, update func(*InstallationValidationEndpointStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[installationValidationStateKey(profileID, endpointID)]
	if state == nil {
		state = &installationValidationState{status: InstallationValidationEndpointStatus{EndpointID: endpointID}}
		s.states[installationValidationStateKey(profileID, endpointID)] = state
	}
	update(&state.status)
}

func eligibleInstallationCount(installations []devices.GameInstallation) int {
	count := 0
	for _, installation := range installations {
		switch installation.InstallState {
		case devicev1.InstallStateInstalled, devicev1.InstallStateMissing, devicev1.InstallStateNeedsRepair:
			count++
		}
	}
	return count
}

func hasEndpointCapability(capabilities []string, wanted string) bool {
	for _, capability := range capabilities {
		if capability == wanted {
			return true
		}
	}
	return false
}

func terminalDeviceCommand(status devicev1.CommandStatus) bool {
	switch status {
	case devicev1.CommandSucceeded, devicev1.CommandFailed, devicev1.CommandRejected,
		devicev1.CommandCanceled, devicev1.CommandExpired:
		return true
	default:
		return false
	}
}

func installationValidationStateKey(profileID, endpointID string) string {
	return profileID + "\x00" + endpointID
}
func stateKeyHasProfile(key, profileID string) bool {
	return len(key) > len(profileID) && key[:len(profileID)+1] == profileID+"\x00"
}

func (c *DeviceController) SetInstallationValidationService(service *InstallationValidationService) {
	c.validation = service
}

func (c *DeviceController) GetInstallationValidationStatus(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if c.validation == nil {
		stdhttp.Error(w, "installation checks are unavailable", stdhttp.StatusServiceUnavailable)
		return
	}
	status, err := c.validation.Status(r.Context())
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, status)
}

func (c *DeviceController) SetInstallationValidationSchedule(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if c.validation == nil {
		stdhttp.Error(w, "installation checks are unavailable", stdhttp.StatusServiceUnavailable)
		return
	}
	var config InstallationValidationScheduleConfig
	if err := decodeJSONBody(w, r, &config); err != nil {
		return
	}
	status, err := c.validation.UpdateConfig(r.Context(), config)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, status)
}

func (c *DeviceController) ValidateInstallations(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if c.validation == nil {
		stdhttp.Error(w, "installation checks are unavailable", stdhttp.StatusServiceUnavailable)
		return
	}
	command, err := c.validation.RunNow(r.Context(), chi.URLParam(r, "id"), core.ProfileIDFromContext(r.Context()))
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusAccepted, command)
}
