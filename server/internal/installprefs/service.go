package installprefs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

const maxRootTemplateBytes = 1024

var ErrInvalidRootTemplate = errors.New("invalid install folder")

type endpointAuthorizer interface {
	AuthorizeEndpoint(ctx context.Context, endpointID, profileID string, required devicev1.AccessLevel) error
}

type Service struct {
	repository Repository
	devices    endpointAuthorizer
	now        func() time.Time
}

func NewService(repository Repository, deviceService endpointAuthorizer) (*Service, error) {
	if repository == nil || deviceService == nil {
		return nil, errors.New("install preference repository and device service are required")
	}
	return &Service{repository: repository, devices: deviceService, now: time.Now}, nil
}

func (s *Service) GetProfile(ctx context.Context, profileID string) (Preference, error) {
	if strings.TrimSpace(profileID) == "" {
		return Preference{}, errors.New("profile is required")
	}
	root, err := s.repository.GetProfileRoot(ctx, profileID)
	if err != nil {
		return Preference{}, err
	}
	source := "profile"
	if strings.TrimSpace(root) == "" {
		root = devicev1.DefaultInstallRootTemplate
		source = "default"
	} else if _, err := normalizeRootTemplate(root, false); err != nil {
		return Preference{}, fmt.Errorf("stored profile install folder: %w", err)
	}
	return Preference{ProfileRoot: root, EffectiveRoot: root, Source: source}, nil
}

func (s *Service) SetProfile(ctx context.Context, profileID, rootTemplate string) (Preference, error) {
	if strings.TrimSpace(profileID) == "" {
		return Preference{}, errors.New("profile is required")
	}
	normalized, err := normalizeRootTemplate(rootTemplate, true)
	if err != nil {
		return Preference{}, err
	}
	if err := s.repository.SetProfileRoot(ctx, profileID, normalized, s.now().UTC()); err != nil {
		return Preference{}, err
	}
	return s.GetProfile(ctx, profileID)
}

func (s *Service) GetEndpoint(ctx context.Context, endpointID, profileID string) (Preference, error) {
	if err := s.devices.AuthorizeEndpoint(ctx, endpointID, profileID, devicev1.AccessView); err != nil {
		return Preference{}, err
	}
	return s.resolve(ctx, endpointID, profileID)
}

func (s *Service) SetEndpoint(ctx context.Context, endpointID, profileID, rootTemplate string) (Preference, error) {
	if err := s.devices.AuthorizeEndpoint(ctx, endpointID, profileID, devicev1.AccessOwner); err != nil {
		return Preference{}, err
	}
	normalized, err := normalizeRootTemplate(rootTemplate, true)
	if err != nil {
		return Preference{}, err
	}
	if err := s.repository.SetEndpointRoot(ctx, endpointID, normalized, profileID, s.now().UTC()); err != nil {
		return Preference{}, err
	}
	return s.resolve(ctx, endpointID, profileID)
}

func (s *Service) ResolveForInstall(ctx context.Context, endpointID, profileID, perInstallRoot string) (string, error) {
	if err := s.devices.AuthorizeEndpoint(ctx, endpointID, profileID, devicev1.AccessManage); err != nil {
		return "", err
	}
	if strings.TrimSpace(perInstallRoot) != "" {
		return normalizeRootTemplate(perInstallRoot, false)
	}
	preference, err := s.resolve(ctx, endpointID, profileID)
	if err != nil {
		return "", err
	}
	return preference.EffectiveRoot, nil
}

func (s *Service) resolve(ctx context.Context, endpointID, profileID string) (Preference, error) {
	profileRoot, err := s.repository.GetProfileRoot(ctx, profileID)
	if err != nil {
		return Preference{}, err
	}
	source := "profile"
	if strings.TrimSpace(profileRoot) == "" {
		profileRoot = devicev1.DefaultInstallRootTemplate
		source = "default"
	} else if _, err := normalizeRootTemplate(profileRoot, false); err != nil {
		return Preference{}, fmt.Errorf("stored profile install folder: %w", err)
	}
	endpointRoot, err := s.repository.GetEndpointRoot(ctx, endpointID)
	if err != nil {
		return Preference{}, err
	}
	preference := Preference{ProfileRoot: profileRoot, EndpointRoot: endpointRoot, EffectiveRoot: profileRoot, Source: source}
	if strings.TrimSpace(endpointRoot) != "" {
		if _, err := normalizeRootTemplate(endpointRoot, false); err != nil {
			return Preference{}, fmt.Errorf("stored device install folder: %w", err)
		}
		preference.EffectiveRoot = endpointRoot
		preference.Source = "device"
	}
	return preference, nil
}

func normalizeRootTemplate(value string, allowReset bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if allowReset {
			return "", nil
		}
		return "", fmt.Errorf("%w: install folder is required", ErrInvalidRootTemplate)
	}
	if !utf8.ValidString(value) || len(value) > maxRootTemplateBytes {
		return "", fmt.Errorf("%w: install folder must be at most %d bytes", ErrInvalidRootTemplate, maxRootTemplateBytes)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("%w: install folder must not contain control characters", ErrInvalidRootTemplate)
		}
	}
	return value, nil
}
