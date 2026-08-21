package clientapp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/commandipc"
	"github.com/GreenFuze/MyGamesAnywhere/client/internal/singleinstance"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/google/uuid"
)

const (
	commandAcknowledgementTimeout = 2 * time.Minute
	commandEndpointReadyTimeout   = 10 * time.Second
	commandDialAttemptTimeout     = 500 * time.Millisecond
	upgradeShutdownTimeout        = 15 * time.Second
)

type protocolAction struct {
	host    string
	rawURI  string
	start   StartOptions
	pair    PairOptions
	release ReleaseInstallationOptions
	adopt   AdoptInstallationOptions
}

// HandleProtocol forwards a browser protocol request to the existing tray
// owner for this OS user, or handles it locally when no tray owner exists.
func (s *Service) HandleProtocol(ctx context.Context, rawURI string) error {
	if s == nil {
		return errors.New("client service is required")
	}
	if len(rawURI) > commandipc.MaxFrameBytes-512 {
		return errors.New("MGA protocol URI is too large")
	}
	running, err := singleinstance.IsRunning(s.instanceLockName())
	if err != nil {
		return fmt.Errorf("check running MGA Client: %w", err)
	}
	if !running {
		return s.executeStandaloneProtocol(ctx, rawURI)
	}

	request := commandipc.Request{
		Version:   commandipc.Version,
		RequestID: uuid.NewString(),
		URI:       rawURI,
	}
	commandContext, cancel := context.WithTimeout(ctx, commandAcknowledgementTimeout)
	defer cancel()
	readyDeadline := time.Now().Add(commandEndpointReadyTimeout)
	endpoint := s.commandEndpoint()
	for {
		attemptContext, attemptCancel := context.WithTimeout(commandContext, commandDialAttemptTimeout)
		err = commandipc.ForwardWithDialTimeout(commandContext, attemptContext, endpoint, request)
		attemptCancel()
		if err == nil {
			s.Logf("forwarded browser protocol request %s to the running client", request.RequestID)
			return nil
		}
		if !errors.Is(err, commandipc.ErrUnavailable) {
			return err
		}
		running, runningErr := singleinstance.IsRunning(s.instanceLockName())
		if runningErr != nil {
			return fmt.Errorf("check running MGA Client while forwarding: %w", runningErr)
		}
		if !running {
			return s.executeStandaloneProtocol(ctx, rawURI)
		}
		if time.Now().After(readyDeadline) {
			return errors.New("the running MGA Client did not open its command channel; use its tray icon to show logs or exit it, then try again")
		}
		select {
		case <-commandContext.Done():
			return errors.New("the running MGA Client did not acknowledge the request in time; check its tray icon and logs, then try again")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (s *Service) executeStandaloneProtocol(ctx context.Context, rawURI string) error {
	action, err := parseProtocolAction(rawURI)
	if err != nil {
		return err
	}
	switch action.host {
	case "start":
		err = s.Start(ctx, action.start)
		if errors.Is(err, ErrElevationRelaunched) {
			return nil
		}
		return err
	case "pair":
		if _, err = s.Pair(ctx, action.pair); err != nil {
			return err
		}
	case "release":
		if err = s.ConfirmAndReleaseInstallation(ctx, action.release); err != nil {
			return err
		}
	case "adopt":
		if err = s.ConfirmAndAdoptInstallation(ctx, action.adopt); err != nil {
			return err
		}
	default:
		return errors.New("unsupported MGA protocol URI")
	}
	return s.RunAgent(ctx)
}

func (s *Service) executeForwardedProtocol(ctx context.Context, rawURI string, executionMode devicev1.ClientExecutionMode) (commandipc.Outcome, error) {
	action, err := parseProtocolAction(rawURI)
	if err != nil {
		return commandipc.Outcome{}, err
	}
	switch action.host {
	case "start":
		requestedMode := normalizedExecutionMode(action.start.ExecutionMode)
		if requestedMode == devicev1.ClientExecutionModeElevated && executionMode != requestedMode {
			if err := launchTakeover(true, requestedMode, rawURI); err != nil {
				return commandipc.Outcome{}, err
			}
			s.Logf("elevated agent takeover launched for forwarded browser request")
			return commandipc.Outcome{StopPrimary: true}, nil
		}
		if requestedMode == devicev1.ClientExecutionModeStandard && executionMode == devicev1.ClientExecutionModeElevated {
			return commandipc.Outcome{}, errors.New("MGA Client is already running as administrator; exit it from the tray before starting it in standard mode")
		}
		action.start.ExecutionMode = executionMode
		return commandipc.Outcome{}, s.acknowledgeLaunch(ctx, action.start)
	case "pair":
		if _, err = s.Pair(ctx, action.pair); err != nil {
			return commandipc.Outcome{}, err
		}
	case "release":
		if err = s.ConfirmAndReleaseInstallation(ctx, action.release); err != nil {
			return commandipc.Outcome{}, err
		}
	case "adopt":
		if err = s.ConfirmAndAdoptInstallation(ctx, action.adopt); err != nil {
			return commandipc.Outcome{}, err
		}
	default:
		return commandipc.Outcome{}, errors.New("unsupported MGA protocol URI")
	}
	if err := launchTakeover(false, executionMode, ""); err != nil {
		return commandipc.Outcome{}, err
	}
	s.Logf("same-mode agent takeover launched after forwarded %s request", action.host)
	return commandipc.Outcome{StopPrimary: true}, nil
}

// StopForUpgrade asks the same OS user's running tray owner to shut down and
// waits until its mutex is released. The installer calls this before replacing
// files. No running agent is already a successful, idempotent result.
func (s *Service) StopForUpgrade(ctx context.Context) error {
	if s == nil {
		return errors.New("client service is required")
	}
	running, err := singleinstance.IsRunning(s.instanceLockName())
	if err != nil {
		return fmt.Errorf("check running MGA Client before upgrade: %w", err)
	}
	if !running {
		return nil
	}

	commandContext, cancel := context.WithTimeout(ctx, upgradeShutdownTimeout)
	defer cancel()
	request := commandipc.Request{
		Version:   commandipc.Version,
		RequestID: uuid.NewString(),
		Action:    commandipc.ActionStopForUpgrade,
	}
	readyDeadline := time.Now().Add(commandEndpointReadyTimeout)
	endpoint := s.commandEndpoint()
	for {
		attemptContext, attemptCancel := context.WithTimeout(commandContext, commandDialAttemptTimeout)
		err = commandipc.ForwardWithDialTimeout(commandContext, attemptContext, endpoint, request)
		attemptCancel()
		if err == nil {
			break
		}
		if !errors.Is(err, commandipc.ErrUnavailable) {
			return fmt.Errorf("request MGA Client shutdown for upgrade: %w", err)
		}
		running, err = singleinstance.IsRunning(s.instanceLockName())
		if err != nil {
			return fmt.Errorf("check MGA Client shutdown for upgrade: %w", err)
		}
		if !running {
			return nil
		}
		if time.Now().After(readyDeadline) {
			return errors.New("the running MGA Client did not open its upgrade command channel")
		}
		select {
		case <-commandContext.Done():
			return errors.New("timed out requesting MGA Client shutdown for upgrade")
		case <-time.After(100 * time.Millisecond):
		}
	}

	for {
		running, err = singleinstance.IsRunning(s.instanceLockName())
		if err != nil {
			return fmt.Errorf("verify MGA Client shutdown for upgrade: %w", err)
		}
		if !running {
			s.Logf("MGA Client stopped for installer upgrade")
			return nil
		}
		select {
		case <-commandContext.Done():
			return errors.New("running MGA Client acknowledged the upgrade but did not stop")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func parseProtocolAction(rawURI string) (protocolAction, error) {
	rawURI = strings.TrimSpace(rawURI)
	parsed, err := url.Parse(rawURI)
	if err != nil || !strings.EqualFold(parsed.Scheme, "mga") {
		return protocolAction{}, errors.New("unsupported MGA protocol URI")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return protocolAction{}, errors.New("MGA protocol URI contains unsupported authority or fragment data")
	}
	action := protocolAction{host: strings.ToLower(parsed.Host), rawURI: rawURI}
	query := parsed.Query()
	switch action.host {
	case "start":
		action.start = StartOptions{
			ServerURL:     query.Get("server"),
			LaunchID:      query.Get("launch_id"),
			Token:         query.Get("token"),
			ExecutionMode: devicev1.ClientExecutionMode(query.Get("mode")),
		}
	case "pair":
		action.pair = PairOptions{ServerURL: query.Get("server"), Code: query.Get("code")}
	case "release":
		action.release = ReleaseInstallationOptions{LocalInstallationID: query.Get("installation_id"), ServerURL: query.Get("server")}
	case "adopt":
		action.adopt = AdoptInstallationOptions{LocalInstallationID: query.Get("installation_id"), ServerURL: query.Get("server")}
	default:
		return protocolAction{}, errors.New("unsupported MGA protocol URI")
	}
	return action, nil
}
