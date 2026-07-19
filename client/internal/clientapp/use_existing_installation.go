package clientapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/desktop"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

var ErrUseExistingDeclined = errors.New("local confirmation declined")

type ExistingInstallationUser interface {
	Use(context.Context, devicev1.UseExistingInstallationRequest) (devicev1.UseExistingInstallationResult, error)
}

type UseExistingConfirmer interface {
	Confirm(context.Context, string, string, string) (bool, error)
}

type desktopUseExistingConfirmer struct{}

func (desktopUseExistingConfirmer) Confirm(ctx context.Context, title, path, server string) (bool, error) {
	return desktop.ConfirmUseExistingInstallation(ctx, title, path, server)
}

type LocalExistingInstallationUser struct {
	ownership *InstallationOwnership
	serverURL string
	confirmer UseExistingConfirmer
	programs  RegisteredProgramObserver
	now       func() time.Time
}

func NewLocalExistingInstallationUser(ownership *InstallationOwnership, serverURL string) (*LocalExistingInstallationUser, error) {
	if ownership == nil || ownership.catalog == nil {
		return nil, errors.New("installation ownership is required")
	}
	if strings.TrimSpace(serverURL) == "" {
		return nil, errors.New("server URL is required")
	}
	programs, _ := newRegisteredProgramInspector().(RegisteredProgramObserver)
	return &LocalExistingInstallationUser{ownership: ownership, serverURL: serverURL, confirmer: desktopUseExistingConfirmer{}, programs: programs, now: time.Now}, nil
}

func (u *LocalExistingInstallationUser) Use(ctx context.Context, request devicev1.UseExistingInstallationRequest) (devicev1.UseExistingInstallationResult, error) {
	var result devicev1.UseExistingInstallationResult
	if u == nil || u.ownership == nil || u.ownership.catalog == nil || u.confirmer == nil || u.now == nil {
		return result, errors.New("existing installation service is unavailable")
	}
	if err := request.Validate(); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	record, found := u.ownership.catalog.FindByID(request.LocalInstallationID)
	if !found {
		return result, errors.New("installation ownership record not found")
	}
	if record.State != OwnershipOwned && record.State != OwnershipReleased {
		return result, fmt.Errorf("installation is %s and cannot be shared", record.State)
	}
	if strings.EqualFold(record.OwnerBindingID, u.ownership.bindingID) {
		return result, errors.New("this MGA Server already manages the installation")
	}
	manifest, err := readInstallManifest(record.InstallPath)
	if err != nil {
		return result, err
	}
	if !isSupportedLaunchManifestVersion(manifest.SchemaVersion) || !strings.EqualFold(manifest.LocalInstallationID, record.LocalInstallationID) ||
		!strings.EqualFold(filepath.Clean(manifest.InstallRoot), filepath.Clean(record.InstallRoot)) {
		return result, errors.New("installation manifest does not match the local ownership catalog")
	}
	if strings.TrimSpace(manifest.LaunchTarget) == "" || len(manifest.LaunchCandidates) == 0 {
		return result, errors.New("installation has no verified launch target")
	}
	for _, candidate := range manifest.LaunchCandidates {
		if reason, fileErr := validateRecordedRegularFile(record.InstallPath, candidate, devicev1.ValidationReasonLaunchTargetMissing); fileErr != nil {
			return result, fileErr
		} else if reason != "" {
			return result, fmt.Errorf("launch candidate %s is unavailable", candidate)
		}
	}
	if info, statErr := os.Stat(record.InstallPath); statErr != nil || !info.IsDir() {
		return result, errors.New("installation folder is unavailable")
	}
	products := append([]devicev1.NativeProductObservation(nil), record.NativeProducts...)
	if record.InstallKind == devicev1.InstallKindGogInno {
		if u.programs == nil {
			return result, errors.New("registered product observer is unavailable")
		}
		observed, observeErr := u.programs.Associations(record.InstallPath)
		if observeErr != nil {
			return result, fmt.Errorf("observe registered product: %w", observeErr)
		}
		if len(observed) == 0 {
			return result, errors.New("Windows no longer reports this installed product")
		}
		products = registeredProductsToProtocol(observed)
		if err := u.ownership.catalog.ReplaceNativeProducts(record.LocalInstallationID, products); err != nil {
			return result, err
		}
	}
	if !u.ownership.catalog.HasUseGrant(record.LocalInstallationID, u.ownership.bindingID) {
		approved, confirmErr := u.confirmer.Confirm(ctx, request.Title, record.InstallPath, u.serverURL)
		if confirmErr != nil {
			return result, confirmErr
		}
		if !approved {
			return result, ErrUseExistingDeclined
		}
		if err := u.ownership.catalog.GrantUse(record.LocalInstallationID, u.ownership.bindingID, u.now().UTC()); err != nil {
			return result, err
		}
	}
	result = devicev1.UseExistingInstallationResult{
		LocalInstallationID: record.LocalInstallationID, GameID: request.GameID, SourceGameID: request.SourceGameID,
		InstallRoot: record.InstallRoot, InstallPath: record.InstallPath, LaunchTarget: manifest.LaunchTarget,
		LaunchCandidates: append([]string(nil), manifest.LaunchCandidates...), NativeProducts: products, GrantedAt: u.now().UTC(),
	}
	return result, result.Validate()
}

func registeredProductsToProtocol(products []RegisteredProgramObservation) []devicev1.NativeProductObservation {
	result := make([]devicev1.NativeProductObservation, 0, len(products))
	for _, product := range products {
		capabilities := []string{}
		if product.CanUninstall {
			capabilities = append(capabilities, "uninstall")
		}
		result = append(result, devicev1.NativeProductObservation{
			Provider: "windows_uninstall", ProductID: product.ProductID, DisplayName: product.DisplayName,
			Version: product.Version, Publisher: product.Publisher, Capabilities: capabilities,
		})
	}
	return result
}
