package clientapp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/buildinfo"
	clientconfig "github.com/GreenFuze/MyGamesAnywhere/client/internal/config"
	"github.com/GreenFuze/MyGamesAnywhere/client/internal/desktop"
	"github.com/GreenFuze/MyGamesAnywhere/client/internal/identity"
	clientruntime "github.com/GreenFuze/MyGamesAnywhere/client/internal/runtime"
	"github.com/GreenFuze/MyGamesAnywhere/client/internal/singleinstance"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/google/uuid"
)

type PairOptions struct {
	ServerURL   string
	Code        string
	DisplayName string
}

type StartOptions struct {
	ServerURL     string
	LaunchID      string
	Token         string
	ExecutionMode devicev1.ClientExecutionMode
}

type BindingStatus struct {
	BindingID        string
	ServerURL        string
	EndpointID       string
	ClientInstanceID string
	DisplayName      string
	PrivateKeyReady  bool
}

type Status struct {
	Paired   bool
	Bindings []BindingStatus
}

type BindingDoctorResult struct {
	Status       BindingStatus
	ServerHealth string
}

type DoctorResult struct {
	Paired   bool
	Bindings []BindingDoctorResult
}

type UnpairOptions struct {
	ServerURL            string
	All                  bool
	ReleaseInstallations bool
}

type Service struct {
	layout      clientruntime.Layout
	configs     *clientconfig.Store
	buildInfo   buildinfo.Info
	httpClient  *http.Client
	logger      *log.Logger
	logFile     *os.File
	ownership   *OwnershipCatalog
	saveDomains *SaveDomainCatalog
	operations  *InstallationCoordinator
}

func New(dataDir string, info buildinfo.Info, extraLogWriters ...io.Writer) (*Service, error) {
	layout, err := clientruntime.Resolve(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve client runtime: %w", err)
	}
	if err := layout.Ensure(); err != nil {
		return nil, fmt.Errorf("prepare client runtime: %w", err)
	}
	configs, err := clientconfig.NewStore(layout.ConfigPath)
	if err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(layout.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open client log: %w", err)
	}
	ownership, err := OpenOwnershipCatalog(layout.OwnershipPath)
	if err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("open installation ownership catalog: %w", err)
	}
	saveDomains, err := OpenSaveDomainCatalog(layout.SaveAuthorityPath)
	if err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("open save domain authority catalog: %w", err)
	}
	writers := []io.Writer{logFile}
	for _, writer := range extraLogWriters {
		if writer != nil && writer != io.Discard {
			writers = append(writers, writer)
		}
	}
	return &Service{
		layout:      layout,
		configs:     configs,
		buildInfo:   info,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
		logger:      log.New(io.MultiWriter(writers...), "", log.Ldate|log.Ltime|log.LUTC),
		logFile:     logFile,
		ownership:   ownership,
		saveDomains: saveDomains,
		operations:  NewInstallationCoordinator(),
	}, nil
}

// Close releases process-level client resources.
func (s *Service) Close() error {
	if s == nil || s.logFile == nil {
		return nil
	}
	return s.logFile.Close()
}

// Logf appends a diagnostic message to the per-user client log.
func (s *Service) Logf(format string, values ...any) {
	if s != nil && s.logger != nil {
		s.logger.Printf(format, values...)
	}
}

func (s *Service) Pair(ctx context.Context, options PairOptions) (clientconfig.Binding, error) {
	s.Logf("pairing requested for server %s", strings.TrimSpace(options.ServerURL))
	serverURL, err := validateServerURL(options.ServerURL)
	if err != nil {
		return clientconfig.Binding{}, err
	}
	document, err := s.loadBindings()
	if errors.Is(err, clientconfig.ErrNotPaired) {
		document = clientconfig.Document{SchemaVersion: clientconfig.SchemaVersion}
	} else if err != nil {
		return clientconfig.Binding{}, err
	}
	metadata, err := localMetadata(options.DisplayName, currentExecutionMode())
	if err != nil {
		return clientconfig.Binding{}, err
	}
	if existing, found := findBinding(document.Bindings, serverURL); found {
		return s.addProfileGrant(ctx, document, existing, strings.TrimSpace(options.Code), metadata)
	}
	if len(document.Bindings) >= clientconfig.MaxBindings {
		return clientconfig.Binding{}, fmt.Errorf("MGA Client already has the maximum of %d server bindings", clientconfig.MaxBindings)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return clientconfig.Binding{}, fmt.Errorf("generate endpoint identity: %w", err)
	}
	instanceID := uuid.NewString()
	binding := clientconfig.Binding{BindingID: uuid.NewString(), ServerURL: serverURL, ClientInstanceID: instanceID, DisplayName: metadata.DisplayName}
	identityStore := s.identityStore(binding)
	if err := identityStore.Save(privateKey); err != nil {
		return clientconfig.Binding{}, fmt.Errorf("protect endpoint private key: %w", err)
	}
	paired := false
	defer func() {
		if !paired {
			_ = identityStore.Clear()
		}
	}()
	pairingRequest := devicev1.PairingRequest{
		Code:             strings.TrimSpace(options.Code),
		ClientInstanceID: instanceID,
		PublicKey:        base64.RawURLEncoding.EncodeToString(publicKey),
		ClientVersion:    s.buildInfo.Version,
		Versions:         devicev1.SupportedVersionRange(),
		Metadata:         metadata,
	}
	pairingResponse, err := s.submitPairing(ctx, serverURL, pairingRequest)
	if err != nil {
		return clientconfig.Binding{}, err
	}
	binding.WebSocketURL = pairingResponse.WebSocketURL
	binding.EndpointID = pairingResponse.EndpointID
	document.Bindings = append(document.Bindings, binding)
	if err := s.configs.Save(document); err != nil {
		return clientconfig.Binding{}, fmt.Errorf("save paired client config: %w", err)
	}
	paired = true
	s.Logf("paired endpoint %s as %s", binding.EndpointID, binding.DisplayName)
	return binding, nil
}

func (s *Service) addProfileGrant(
	ctx context.Context,
	document clientconfig.Document,
	binding clientconfig.Binding,
	code string,
	metadata devicev1.EndpointMetadata,
) (clientconfig.Binding, error) {
	privateKey, err := s.identityStore(binding).Load()
	if err != nil {
		return clientconfig.Binding{}, fmt.Errorf("load existing endpoint identity: %w", err)
	}
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return clientconfig.Binding{}, errors.New("existing endpoint identity is not Ed25519")
	}
	request := devicev1.PairingRequest{
		Code:               code,
		ClientInstanceID:   binding.ClientInstanceID,
		PublicKey:          base64.RawURLEncoding.EncodeToString(publicKey),
		ClientVersion:      s.buildInfo.Version,
		Versions:           devicev1.SupportedVersionRange(),
		Metadata:           metadata,
		ExistingEndpointID: binding.EndpointID,
	}
	signingBytes, err := request.SigningBytes()
	if err != nil {
		return clientconfig.Binding{}, err
	}
	request.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, signingBytes))
	response, err := s.submitPairing(ctx, binding.ServerURL, request)
	if err != nil {
		return clientconfig.Binding{}, err
	}
	if response.EndpointID != binding.EndpointID {
		return clientconfig.Binding{}, errors.New("MGA Server returned a different endpoint identity while adding profile access")
	}
	binding.WebSocketURL = response.WebSocketURL
	for index := range document.Bindings {
		if document.Bindings[index].BindingID == binding.BindingID {
			document.Bindings[index] = binding
			if err := s.configs.Save(document); err != nil {
				return clientconfig.Binding{}, fmt.Errorf("save refreshed client binding: %w", err)
			}
			s.Logf("added profile grant for existing endpoint %s", binding.EndpointID)
			return binding, nil
		}
	}
	return clientconfig.Binding{}, errors.New("existing MGA Client binding disappeared before it could be saved")
}

func (s *Service) submitPairing(ctx context.Context, serverURL string, pairingRequest devicev1.PairingRequest) (devicev1.PairingResponse, error) {
	if err := pairingRequest.Validate(); err != nil {
		return devicev1.PairingResponse{}, err
	}
	body, err := json.Marshal(pairingRequest)
	if err != nil {
		return devicev1.PairingResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/devices/pair", bytes.NewReader(body))
	if err != nil {
		return devicev1.PairingResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := s.httpClient.Do(request)
	if err != nil {
		return devicev1.PairingResponse{}, fmt.Errorf("pair with MGA Server: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024))
		return devicev1.PairingResponse{}, fmt.Errorf("pair with MGA Server: %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	var pairingResponse devicev1.PairingResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pairingResponse); err != nil {
		return devicev1.PairingResponse{}, fmt.Errorf("decode pairing response: %w", err)
	}
	if err := pairingResponse.Validate(); err != nil {
		return devicev1.PairingResponse{}, err
	}
	return pairingResponse, nil
}

func (s *Service) Status() (Status, error) {
	document, err := s.loadBindings()
	if errors.Is(err, clientconfig.ErrNotPaired) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	status := Status{Paired: true, Bindings: make([]BindingStatus, 0, len(document.Bindings))}
	for _, binding := range document.Bindings {
		_, keyErr := s.identityStore(binding).Load()
		status.Bindings = append(status.Bindings, BindingStatus{
			BindingID: binding.BindingID, ServerURL: binding.ServerURL, EndpointID: binding.EndpointID,
			ClientInstanceID: binding.ClientInstanceID, DisplayName: binding.DisplayName,
			PrivateKeyReady: keyErr == nil,
		})
	}
	return status, nil
}

func (s *Service) Doctor(ctx context.Context) (DoctorResult, error) {
	status, err := s.Status()
	if err != nil {
		return DoctorResult{}, err
	}
	result := DoctorResult{Paired: status.Paired}
	if !status.Paired {
		return result, nil
	}
	for _, binding := range status.Bindings {
		item := BindingDoctorResult{Status: binding, ServerHealth: "not checked"}
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, binding.ServerURL+"/health", nil)
		if requestErr != nil {
			return DoctorResult{}, requestErr
		}
		response, requestErr := s.httpClient.Do(request)
		if requestErr != nil {
			item.ServerHealth = requestErr.Error()
		} else {
			if response.StatusCode == http.StatusOK {
				item.ServerHealth = "OK"
			} else {
				item.ServerHealth = response.Status
			}
			response.Body.Close()
		}
		result.Bindings = append(result.Bindings, item)
	}
	return result, nil
}

func (s *Service) RunAgent(ctx context.Context) error {
	return s.runAgentWithMode(ctx, currentExecutionMode(), false)
}

// RunAgentReplacingExisting restarts an existing tray process so a newly
// paired server binding becomes active immediately.
func (s *Service) RunAgentReplacingExisting(ctx context.Context) error {
	return s.runAgentWithMode(ctx, currentExecutionMode(), true)
}

// RunAgentWithMode runs the per-user agent and reports the supplied actual
// runtime mode. Start uses it after it has verified elevation when requested.
func (s *Service) RunAgentWithMode(ctx context.Context, executionMode devicev1.ClientExecutionMode) error {
	return s.runAgentWithMode(ctx, executionMode, false)
}

func (s *Service) runAgentWithMode(ctx context.Context, executionMode devicev1.ClientExecutionMode, replaceExisting bool) error {
	if executionMode == "" {
		executionMode = devicev1.ClientExecutionModeStandard
	}
	if err := executionMode.Validate(); err != nil {
		return err
	}
	document, err := s.loadBindings()
	if err != nil {
		return err
	}
	lock, err := s.acquireAgentLock(ctx, replaceExisting)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := s.ownership.RecoverInterrupted(); err != nil {
		return fmt.Errorf("recover interrupted installation ownership: %w", err)
	}
	control, err := singleinstance.OpenSignal(s.instanceLockName() + "-Restart")
	if err != nil {
		return fmt.Errorf("open agent restart signal: %w", err)
	}
	defer control.Close()
	hostContext, stopHost := context.WithCancel(ctx)
	defer stopHost()
	go func() {
		if waitErr := control.Wait(hostContext); waitErr == nil {
			s.Logf("agent restart requested after server binding change")
			stopHost()
		}
	}()
	agents := make([]*Agent, 0, len(document.Bindings))
	desktopBindings := make([]desktop.BindingOption, 0, len(document.Bindings))
	for _, binding := range document.Bindings {
		privateKey, keyErr := s.identityStore(binding).Load()
		if keyErr != nil {
			return fmt.Errorf("load endpoint identity for %s: %w", binding.ServerURL, keyErr)
		}
		ownership, ownershipErr := NewInstallationOwnership(binding.BindingID, binding.ServerURL, len(document.Bindings), s.ownership, s.operations)
		if ownershipErr != nil {
			return fmt.Errorf("prepare installation ownership for %s: %w", binding.ServerURL, ownershipErr)
		}
		ownership.saveDomains = s.saveDomains
		ownership.saveRoot = s.layout.SaveDomainsRoot
		agent, agentErr := NewOwnedAgentWithExecutionMode(binding, privateKey, s.buildInfo, s.logger, executionMode, ownership)
		if agentErr != nil {
			return fmt.Errorf("prepare agent for %s: %w", binding.ServerURL, agentErr)
		}
		agents = append(agents, agent)
		serverURL := binding.ServerURL
		desktopBindings = append(desktopBindings, desktop.BindingOption{
			ServerURL: serverURL,
			Unpair:    func() error { return s.clearBinding(serverURL, false) },
			ReleaseAndUnpair: func() error {
				if err := s.releaseAllOwnedByServer(serverURL); err != nil {
					return err
				}
				return s.clearBinding(serverURL, false)
			},
		})
	}
	bindingLabels := make(map[string]string, len(document.Bindings))
	for _, binding := range document.Bindings {
		bindingLabels[strings.ToLower(binding.BindingID)] = binding.ServerURL
	}
	desktopInstallations := make([]desktop.InstallationOption, 0)
	for _, record := range s.ownership.List() {
		if record.State != OwnershipOwned {
			continue
		}
		record := record
		ownerLabel := bindingLabels[strings.ToLower(record.OwnerBindingID)]
		if ownerLabel == "" {
			ownerLabel = "Disconnected MGA server"
		}
		desktopInstallations = append(desktopInstallations, desktop.InstallationOption{LocalInstallationID: record.LocalInstallationID, Title: record.Title, Path: record.InstallPath, OwnerLabel: ownerLabel, Release: func() error {
			return s.ReleaseInstallation(ReleaseInstallationOptions{LocalInstallationID: record.LocalInstallationID})
		}})
	}
	host, err := desktop.NewHost(desktop.Options{
		DisplayName:   document.Bindings[0].DisplayName,
		LogPath:       s.layout.LogPath,
		Version:       s.buildInfo.Version,
		Bindings:      desktopBindings,
		Installations: desktopInstallations,
	})
	if err != nil {
		return err
	}
	s.Logf("agent host starting with %d server binding(s)", len(agents))
	err = host.Run(hostContext, func(runContext context.Context) error { return runAgents(runContext, agents) })
	if err != nil && !errors.Is(err, context.Canceled) {
		s.Logf("agent stopped with error: %v", err)
		return err
	}
	s.Logf("agent stopped")
	return nil
}

func (s *Service) acquireAgentLock(ctx context.Context, replaceExisting bool) (*singleinstance.Lock, error) {
	lock, err := singleinstance.Acquire(s.instanceLockName())
	if err == nil || !replaceExisting || !errors.Is(err, singleinstance.ErrAlreadyRunning) {
		return lock, err
	}
	control, controlErr := singleinstance.OpenSignal(s.instanceLockName() + "-Restart")
	if controlErr != nil {
		return nil, fmt.Errorf("open existing agent restart signal: %w", controlErr)
	}
	if controlErr = control.Notify(); controlErr != nil {
		_ = control.Close()
		return nil, fmt.Errorf("request existing agent restart: %w", controlErr)
	}
	defer control.Close()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout.C:
			return nil, errors.New("existing MGA Client did not stop after restart request")
		case <-ticker.C:
			lock, err = singleinstance.Acquire(s.instanceLockName())
			if err == nil {
				return lock, nil
			}
			if !errors.Is(err, singleinstance.ErrAlreadyRunning) {
				return nil, err
			}
		}
	}
}

// Start acknowledges a browser launch challenge before ensuring the per-user
// agent is running. An existing agent is success: the acknowledgement still
// lets the browser associate itself with the correct endpoint.
func (s *Service) Start(ctx context.Context, options StartOptions) error {
	requestedMode := options.ExecutionMode
	if requestedMode == "" {
		requestedMode = devicev1.ClientExecutionModeStandard
	}
	if err := requestedMode.Validate(); err != nil {
		return err
	}
	if requestedMode == devicev1.ClientExecutionModeElevated && currentExecutionMode() != devicev1.ClientExecutionModeElevated {
		return relaunchElevated(startURI(options, requestedMode))
	}
	if err := s.acknowledgeLaunch(ctx, options); err != nil {
		return err
	}
	if err := s.RunAgentWithMode(ctx, currentExecutionMode()); err != nil && !errors.Is(err, singleinstance.ErrAlreadyRunning) {
		return err
	}
	return nil
}

func (s *Service) acknowledgeLaunch(ctx context.Context, options StartOptions) error {
	document, err := s.loadBindings()
	if err != nil {
		return err
	}
	serverURL, err := validateServerURL(options.ServerURL)
	if err != nil {
		return err
	}
	binding, found := findBinding(document.Bindings, serverURL)
	if !found {
		servers := make([]string, 0, len(document.Bindings))
		for _, existing := range document.Bindings {
			servers = append(servers, existing.ServerURL)
		}
		return &ServerBindingNotFoundError{RequestedServer: serverURL, BoundServers: servers}
	}
	privateKey, err := s.identityStore(binding).Load()
	if err != nil {
		return fmt.Errorf("load endpoint identity: %w", err)
	}
	launchRequest := devicev1.ClientLaunchRequest{
		LaunchID:      strings.TrimSpace(options.LaunchID),
		Token:         strings.TrimSpace(options.Token),
		EndpointID:    binding.EndpointID,
		ExecutionMode: normalizedExecutionMode(options.ExecutionMode),
	}
	signingBytes, err := launchRequest.SigningBytes()
	if err != nil {
		return err
	}
	launchRequest.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, signingBytes))
	body, err := json.Marshal(launchRequest)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/devices/client-launches/redeem", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := s.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("acknowledge MGA Client launch: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024))
		return fmt.Errorf("acknowledge MGA Client launch: %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	return nil
}

func (s *Service) Unpair(options UnpairOptions) error {
	if options.All && strings.TrimSpace(options.ServerURL) != "" {
		return errors.New("choose either one server or all bindings")
	}
	document, err := s.loadBindings()
	if errors.Is(err, clientconfig.ErrNotPaired) {
		return nil
	}
	if err != nil {
		return err
	}
	if !options.All && strings.TrimSpace(options.ServerURL) == "" && len(document.Bindings) > 1 {
		return errors.New("multiple server bindings exist; choose --server or --all")
	}
	lock, err := singleinstance.Acquire(s.instanceLockName())
	if err != nil {
		return fmt.Errorf("stop the running MGA Client agent before unpairing: %w", err)
	}
	defer lock.Close()
	if options.ReleaseInstallations {
		if options.All {
			for _, binding := range document.Bindings {
				if err := s.releaseAllOwnedByServer(binding.ServerURL); err != nil {
					return err
				}
			}
		} else {
			target := options.ServerURL
			if strings.TrimSpace(target) == "" && len(document.Bindings) == 1 {
				target = document.Bindings[0].ServerURL
			}
			if err := s.releaseAllOwnedByServer(target); err != nil {
				return err
			}
		}
	}
	return s.clearBinding(options.ServerURL, options.All)
}

func (s *Service) clearBinding(serverURL string, all bool) error {
	document, err := s.loadBindings()
	if errors.Is(err, clientconfig.ErrNotPaired) {
		return nil
	}
	if err != nil {
		return err
	}
	removed := document.Bindings
	remaining := make([]clientconfig.Binding, 0, len(document.Bindings))
	if !all {
		target := strings.TrimSpace(serverURL)
		if target == "" && len(document.Bindings) == 1 {
			target = document.Bindings[0].ServerURL
		}
		normalized, normalizeErr := validateServerURL(target)
		if normalizeErr != nil {
			return normalizeErr
		}
		removed = nil
		for _, binding := range document.Bindings {
			if samePairedServerURL(normalized, binding.ServerURL) {
				removed = append(removed, binding)
			} else {
				remaining = append(remaining, binding)
			}
		}
		if len(removed) == 0 {
			return fmt.Errorf("MGA Client is not paired with %s", normalized)
		}
	}
	if len(remaining) == 0 {
		if err := s.configs.Clear(); err != nil {
			return fmt.Errorf("clear client configuration: %w", err)
		}
	} else {
		document.Bindings = remaining
		if err := s.configs.Save(document); err != nil {
			return fmt.Errorf("save remaining client bindings: %w", err)
		}
	}
	for _, binding := range removed {
		if err := s.identityStore(binding).Clear(); err != nil {
			return fmt.Errorf("clear endpoint private key for %s: %w", binding.ServerURL, err)
		}
	}
	return nil
}

// ServerBindingNotFoundError reports a browser launch from an unpaired server.
type ServerBindingNotFoundError struct {
	RequestedServer string
	BoundServers    []string
}

func (e *ServerBindingNotFoundError) Error() string {
	if len(e.BoundServers) == 0 {
		return fmt.Sprintf("MGA Client is not paired with %s; return to MGA and choose Pair this Windows user", e.RequestedServer)
	}
	return fmt.Sprintf("MGA Client is not paired with %s; current bindings: %s. Return to that MGA Server and choose Pair this Windows user", e.RequestedServer, strings.Join(e.BoundServers, ", "))
}

func (s *Service) loadBindings() (clientconfig.Document, error) {
	result, err := s.configs.Load()
	if err != nil {
		return clientconfig.Document{}, err
	}
	if err := validateUniqueBindingOrigins(result.Document.Bindings); err != nil {
		return clientconfig.Document{}, err
	}
	if result.MigrationFrom == 0 {
		return result.Document, nil
	}
	for _, binding := range result.Document.Bindings {
		if _, err := s.identityStore(binding).Load(); err != nil {
			return clientconfig.Document{}, fmt.Errorf("verify endpoint identity for %s before config migration: %w", binding.ServerURL, err)
		}
	}
	if err := s.configs.Save(result.Document); err != nil {
		return clientconfig.Document{}, fmt.Errorf("migrate client config to schema %d: %w", clientconfig.SchemaVersion, err)
	}
	s.Logf("migrated client configuration from schema %d to %d", result.MigrationFrom, clientconfig.SchemaVersion)
	return result.Document, nil
}

func validateUniqueBindingOrigins(bindings []clientconfig.Binding) error {
	for index, binding := range bindings {
		for previous := 0; previous < index; previous++ {
			if samePairedServerURL(binding.ServerURL, bindings[previous].ServerURL) {
				return fmt.Errorf("duplicate equivalent server bindings %q and %q", bindings[previous].ServerURL, binding.ServerURL)
			}
		}
	}
	return nil
}

func (s *Service) identityStore(binding clientconfig.Binding) identity.Store {
	if binding.LegacyIdentity {
		return identity.NewStore(s.layout.PrivateKeyPath)
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(binding.ClientInstanceID)))
	name := hex.EncodeToString(digest[:]) + ".dpapi"
	return identity.NewStore(filepath.Join(s.layout.IdentityDir, name))
}

func (s *Service) instanceLockName() string {
	digest := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(s.layout.DataDir))))
	return "MGAClient-" + hex.EncodeToString(digest[:16])
}

func findBinding(bindings []clientconfig.Binding, serverURL string) (clientconfig.Binding, bool) {
	for _, binding := range bindings {
		if samePairedServerURL(serverURL, binding.ServerURL) {
			return binding, true
		}
	}
	return clientconfig.Binding{}, false
}

func runAgents(ctx context.Context, agents []*Agent) error {
	if len(agents) == 0 {
		return clientconfig.ErrNotPaired
	}
	results := make(chan error, len(agents))
	for _, agent := range agents {
		agent := agent
		go func() { results <- agent.Run(ctx) }()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-results:
		return err
	}
}

func validateServerURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("server must be an absolute http or https URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("server URL must not contain credentials, query, or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return strings.TrimRight(parsed.String(), "/"), nil
}

func samePairedServerURL(launchURL, pairedURL string) bool {
	launch, launchErr := url.Parse(launchURL)
	paired, pairedErr := url.Parse(pairedURL)
	if launchErr != nil || pairedErr != nil || launch.Host == "" || paired.Host == "" {
		return false
	}
	if !strings.EqualFold(launch.Scheme, paired.Scheme) || effectiveServerPort(launch) != effectiveServerPort(paired) {
		return false
	}
	if launch.EscapedPath() != paired.EscapedPath() {
		return false
	}
	if strings.EqualFold(launch.Hostname(), paired.Hostname()) {
		return true
	}
	return isLoopbackServerHost(launch.Hostname()) && isLoopbackServerHost(paired.Hostname())
}

func effectiveServerPort(server *url.URL) string {
	if port := server.Port(); port != "" {
		return port
	}
	if strings.EqualFold(server.Scheme, "https") {
		return "443"
	}
	return "80"
}

func isLoopbackServerHost(host string) bool {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true
	}
	address := net.ParseIP(strings.TrimSpace(host))
	return address != nil && address.IsLoopback()
}

func localMetadata(displayName string, executionMode devicev1.ClientExecutionMode) (devicev1.EndpointMetadata, error) {
	hostName, err := os.Hostname()
	if err != nil {
		return devicev1.EndpointMetadata{}, fmt.Errorf("read host name: %w", err)
	}
	currentUser, err := user.Current()
	if err != nil {
		return devicev1.EndpointMetadata{}, fmt.Errorf("read OS user: %w", err)
	}
	userName := strings.TrimSpace(currentUser.Username)
	if displayName = strings.TrimSpace(displayName); displayName == "" {
		displayName = hostName + " / " + userName
	}
	metadata := devicev1.EndpointMetadata{
		DisplayName:   displayName,
		HostName:      hostName,
		OSUser:        userName,
		Platform:      runtime.GOOS,
		Arch:          runtime.GOARCH,
		ExecutionMode: normalizedExecutionMode(executionMode),
		Capabilities: []string{
			devicev1.CapabilityEndpointPing,
			devicev1.CapabilityEndpointRefresh,
			devicev1.CapabilityEndpointStop,
			devicev1.CapabilityGameInstallArchive,
			devicev1.CapabilityGameUninstall,
			devicev1.CapabilityGameInstallGogInno,
			devicev1.CapabilityGameUninstallGogInno,
			devicev1.CapabilityGameCleanupGogInnoFailed,
			devicev1.CapabilityGameValidateInstallations,
			devicev1.CapabilityGameUseExisting,
			devicev1.CapabilityGameLaunch,
			devicev1.CapabilityGameLaunchEmulator,
			devicev1.CapabilityEmulatorSetup,
			devicev1.CapabilitySaveDomainClaim,
			devicev1.CapabilitySaveDomainRelease,
			devicev1.CapabilitySaveDomainSnapshot,
			devicev1.CapabilitySaveDomainRestore,
			devicev1.CapabilitySaveDomainReconcile,
			devicev1.CapabilityInventoryRefresh,
			devicev1.CapabilityInstallationPreflight,
		},
	}
	if err := metadata.Validate(); err != nil {
		return devicev1.EndpointMetadata{}, err
	}
	return metadata, nil
}

func normalizedExecutionMode(mode devicev1.ClientExecutionMode) devicev1.ClientExecutionMode {
	if mode == "" {
		return devicev1.ClientExecutionModeStandard
	}
	return mode
}

func startURI(options StartOptions, mode devicev1.ClientExecutionMode) string {
	uri := &url.URL{Scheme: "mga", Host: "start"}
	query := uri.Query()
	query.Set("server", strings.TrimSpace(options.ServerURL))
	query.Set("launch_id", strings.TrimSpace(options.LaunchID))
	query.Set("token", strings.TrimSpace(options.Token))
	query.Set("mode", string(mode))
	uri.RawQuery = query.Encode()
	return uri.String()
}
