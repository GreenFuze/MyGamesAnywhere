package installprefs

import (
	"context"
	"errors"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type memoryRepository struct {
	profileRoot  string
	endpointRoot string
}

func (r *memoryRepository) GetProfileRoot(context.Context, string) (string, error) {
	return r.profileRoot, nil
}

func (r *memoryRepository) SetProfileRoot(_ context.Context, _ string, root string, _ time.Time) error {
	r.profileRoot = root
	return nil
}

func (r *memoryRepository) GetEndpointRoot(context.Context, string) (string, error) {
	return r.endpointRoot, nil
}

func (r *memoryRepository) SetEndpointRoot(_ context.Context, _, root, _ string, _ time.Time) error {
	r.endpointRoot = root
	return nil
}

type recordingAuthorizer struct {
	required []devicev1.AccessLevel
	err      error
}

func (a *recordingAuthorizer) AuthorizeEndpoint(_ context.Context, _, _ string, required devicev1.AccessLevel) error {
	a.required = append(a.required, required)
	return a.err
}

func TestResolveForInstallUsesLockedPrecedence(t *testing.T) {
	repository := &memoryRepository{profileRoot: `%USERPROFILE%\Profile Games`, endpointRoot: `D:\Device Games`}
	authorizer := &recordingAuthorizer{}
	service, err := NewService(repository, authorizer)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		override string
		endpoint string
		profile  string
		want     string
	}{
		{name: "per install", override: `E:\One Game`, endpoint: `D:\Device Games`, profile: `%USERPROFILE%\Profile Games`, want: `E:\One Game`},
		{name: "endpoint", endpoint: `D:\Device Games`, profile: `%USERPROFILE%\Profile Games`, want: `D:\Device Games`},
		{name: "profile", profile: `%USERPROFILE%\Profile Games`, want: `%USERPROFILE%\Profile Games`},
		{name: "fallback", want: devicev1.DefaultInstallRootTemplate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository.endpointRoot = test.endpoint
			repository.profileRoot = test.profile
			got, resolveErr := service.ResolveForInstall(context.Background(), "endpoint", "profile", test.override)
			if resolveErr != nil {
				t.Fatal(resolveErr)
			}
			if got != test.want {
				t.Fatalf("resolved root = %q, want %q", got, test.want)
			}
		})
	}
	for _, required := range authorizer.required {
		if required != devicev1.AccessManage {
			t.Fatalf("install authorization = %q, want manage", required)
		}
	}
	repository.endpointRoot = ""
	repository.profileRoot = ""
	preference, err := service.GetEndpoint(context.Background(), "endpoint", "profile")
	if err != nil {
		t.Fatal(err)
	}
	if preference.Source != "default" || preference.EffectiveRoot != devicev1.DefaultInstallRootTemplate {
		t.Fatalf("fallback preference = %#v", preference)
	}
}

func TestEndpointPreferenceRequiresOwnerAndClearsToProfile(t *testing.T) {
	repository := &memoryRepository{profileRoot: `C:\Profile Games`}
	authorizer := &recordingAuthorizer{}
	service, _ := NewService(repository, authorizer)

	preference, err := service.SetEndpoint(context.Background(), "endpoint", "profile", `D:\Games`)
	if err != nil {
		t.Fatal(err)
	}
	if preference.EffectiveRoot != `D:\Games` || preference.Source != "device" {
		t.Fatalf("device preference = %#v", preference)
	}
	preference, err = service.SetEndpoint(context.Background(), "endpoint", "profile", "")
	if err != nil {
		t.Fatal(err)
	}
	if preference.EffectiveRoot != `C:\Profile Games` || preference.Source != "profile" {
		t.Fatalf("cleared preference = %#v", preference)
	}
	if len(authorizer.required) != 2 || authorizer.required[0] != devicev1.AccessOwner || authorizer.required[1] != devicev1.AccessOwner {
		t.Fatalf("endpoint preference authorization = %v", authorizer.required)
	}
}

func TestInvalidRootFailsBeforePersistence(t *testing.T) {
	repository := &memoryRepository{}
	authorizer := &recordingAuthorizer{}
	service, _ := NewService(repository, authorizer)

	_, err := service.SetProfile(context.Background(), "profile", "C:\\Games\nOther")
	if !errors.Is(err, ErrInvalidRootTemplate) {
		t.Fatalf("SetProfile() error = %v, want invalid root", err)
	}
	if repository.profileRoot != "" {
		t.Fatalf("invalid root was persisted as %q", repository.profileRoot)
	}
}
