package clientapp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/user"
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
	ServerURL string
	LaunchID  string
	Token     string
}

type Status struct {
	Paired           bool
	ServerURL        string
	EndpointID       string
	ClientInstanceID string
	DisplayName      string
	PrivateKeyReady  bool
}

type DoctorResult struct {
	Status       Status
	ServerHealth string
}

type Service struct {
	layout     clientruntime.Layout
	configs    *clientconfig.Store
	identities identity.Store
	buildInfo  buildinfo.Info
	httpClient *http.Client
	logger     *log.Logger
	logFile    *os.File
}

func New(dataDir string, info buildinfo.Info) (*Service, error) {
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
	return &Service{
		layout:     layout,
		configs:    configs,
		identities: identity.NewStore(layout.PrivateKeyPath),
		buildInfo:  info,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		logger:     log.New(logFile, "", log.Ldate|log.Ltime|log.LUTC),
		logFile:    logFile,
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

func (s *Service) Pair(ctx context.Context, options PairOptions) (clientconfig.Config, error) {
	s.Logf("pairing requested for server %s", strings.TrimSpace(options.ServerURL))
	if _, err := s.configs.Load(); err == nil {
		return clientconfig.Config{}, errors.New("MGA Client is already paired; unpair or use a separate data directory")
	} else if !errors.Is(err, clientconfig.ErrNotPaired) {
		return clientconfig.Config{}, err
	}
	serverURL, err := validateServerURL(options.ServerURL)
	if err != nil {
		return clientconfig.Config{}, err
	}
	metadata, err := localMetadata(options.DisplayName)
	if err != nil {
		return clientconfig.Config{}, err
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return clientconfig.Config{}, fmt.Errorf("generate endpoint identity: %w", err)
	}
	if err := s.identities.Save(privateKey); err != nil {
		return clientconfig.Config{}, fmt.Errorf("protect endpoint private key: %w", err)
	}
	paired := false
	defer func() {
		if !paired {
			_ = s.identities.Clear()
		}
	}()
	instanceID := uuid.NewString()
	pairingRequest := devicev1.PairingRequest{
		Code:             strings.TrimSpace(options.Code),
		ClientInstanceID: instanceID,
		PublicKey:        base64.RawURLEncoding.EncodeToString(publicKey),
		ClientVersion:    s.buildInfo.Version,
		Versions:         devicev1.SupportedVersionRange(),
		Metadata:         metadata,
	}
	if err := pairingRequest.Validate(); err != nil {
		return clientconfig.Config{}, err
	}
	body, err := json.Marshal(pairingRequest)
	if err != nil {
		return clientconfig.Config{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/devices/pair", bytes.NewReader(body))
	if err != nil {
		return clientconfig.Config{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := s.httpClient.Do(request)
	if err != nil {
		return clientconfig.Config{}, fmt.Errorf("pair with MGA Server: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024))
		return clientconfig.Config{}, fmt.Errorf("pair with MGA Server: %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	var pairingResponse devicev1.PairingResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pairingResponse); err != nil {
		return clientconfig.Config{}, fmt.Errorf("decode pairing response: %w", err)
	}
	if err := pairingResponse.Validate(); err != nil {
		return clientconfig.Config{}, err
	}
	config := clientconfig.Config{
		SchemaVersion:    clientconfig.SchemaVersion,
		ServerURL:        serverURL,
		WebSocketURL:     pairingResponse.WebSocketURL,
		EndpointID:       pairingResponse.EndpointID,
		ClientInstanceID: instanceID,
		DisplayName:      metadata.DisplayName,
	}
	if err := s.configs.Save(config); err != nil {
		return clientconfig.Config{}, fmt.Errorf("save paired client config: %w", err)
	}
	paired = true
	s.Logf("paired endpoint %s as %s", config.EndpointID, config.DisplayName)
	return config, nil
}

func (s *Service) Status() (Status, error) {
	config, err := s.configs.Load()
	if errors.Is(err, clientconfig.ErrNotPaired) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	_, keyErr := s.identities.Load()
	return Status{
		Paired:           true,
		ServerURL:        config.ServerURL,
		EndpointID:       config.EndpointID,
		ClientInstanceID: config.ClientInstanceID,
		DisplayName:      config.DisplayName,
		PrivateKeyReady:  keyErr == nil,
	}, nil
}

func (s *Service) Doctor(ctx context.Context) (DoctorResult, error) {
	status, err := s.Status()
	if err != nil {
		return DoctorResult{}, err
	}
	result := DoctorResult{Status: status, ServerHealth: "not checked"}
	if !status.Paired {
		return result, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, status.ServerURL+"/health", nil)
	if err != nil {
		return DoctorResult{}, err
	}
	response, err := s.httpClient.Do(request)
	if err != nil {
		result.ServerHealth = err.Error()
		return result, nil
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		result.ServerHealth = "OK"
	} else {
		result.ServerHealth = response.Status
	}
	return result, nil
}

func (s *Service) RunAgent(ctx context.Context) error {
	config, err := s.configs.Load()
	if err != nil {
		return err
	}
	privateKey, err := s.identities.Load()
	if err != nil {
		return fmt.Errorf("load endpoint identity: %w", err)
	}
	lock, err := singleinstance.Acquire("MGAClient-" + config.ClientInstanceID)
	if err != nil {
		return err
	}
	defer lock.Close()
	agent, err := NewAgent(config, privateKey, s.buildInfo, s.logger)
	if err != nil {
		return err
	}
	host, err := desktop.NewHost(desktop.Options{
		DisplayName: config.DisplayName,
		LogPath:     s.layout.LogPath,
		Version:     s.buildInfo.Version,
	})
	if err != nil {
		return err
	}
	s.Logf("agent starting for endpoint %s", config.EndpointID)
	err = host.Run(ctx, agent.Run)
	if err != nil && !errors.Is(err, context.Canceled) {
		s.Logf("agent stopped with error: %v", err)
		return err
	}
	s.Logf("agent stopped")
	return nil
}

// Start acknowledges a browser launch challenge before ensuring the per-user
// agent is running. An existing agent is success: the acknowledgement still
// lets the browser associate itself with the correct endpoint.
func (s *Service) Start(ctx context.Context, options StartOptions) error {
	if err := s.acknowledgeLaunch(ctx, options); err != nil {
		return err
	}
	if err := s.RunAgent(ctx); err != nil && !errors.Is(err, singleinstance.ErrAlreadyRunning) {
		return err
	}
	return nil
}

func (s *Service) acknowledgeLaunch(ctx context.Context, options StartOptions) error {
	config, err := s.configs.Load()
	if err != nil {
		return err
	}
	serverURL, err := validateServerURL(options.ServerURL)
	if err != nil {
		return err
	}
	if serverURL != config.ServerURL {
		return errors.New("launch server does not match the paired MGA Server")
	}
	privateKey, err := s.identities.Load()
	if err != nil {
		return fmt.Errorf("load endpoint identity: %w", err)
	}
	launchRequest := devicev1.ClientLaunchRequest{
		LaunchID:   strings.TrimSpace(options.LaunchID),
		Token:      strings.TrimSpace(options.Token),
		EndpointID: config.EndpointID,
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

func (s *Service) Unpair() error {
	config, err := s.configs.Load()
	if errors.Is(err, clientconfig.ErrNotPaired) {
		return nil
	}
	if err != nil {
		return err
	}
	lock, err := singleinstance.Acquire("MGAClient-" + config.ClientInstanceID)
	if err != nil {
		return fmt.Errorf("stop the running MGA Client agent before unpairing: %w", err)
	}
	defer lock.Close()
	if err := s.configs.Clear(); err != nil {
		return fmt.Errorf("clear client configuration: %w", err)
	}
	if err := s.identities.Clear(); err != nil {
		return fmt.Errorf("clear endpoint private key: %w", err)
	}
	return nil
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

func localMetadata(displayName string) (devicev1.EndpointMetadata, error) {
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
		DisplayName: displayName,
		HostName:    hostName,
		OSUser:      userName,
		Platform:    runtime.GOOS,
		Arch:        runtime.GOARCH,
		Capabilities: []string{
			devicev1.CapabilityEndpointPing,
			devicev1.CapabilityEndpointRefresh,
			devicev1.CapabilityEndpointStop,
			devicev1.CapabilityGameInstallArchive,
			devicev1.CapabilityGameUninstall,
			devicev1.CapabilityGameLaunch,
			devicev1.CapabilityInventoryRefresh,
		},
	}
	if err := metadata.Validate(); err != nil {
		return devicev1.EndpointMetadata{}, err
	}
	return metadata, nil
}
