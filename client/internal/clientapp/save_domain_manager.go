package clientapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/desktop"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

var ErrSaveDomainConfirmationDeclined = errors.New("local save-domain confirmation declined")

type SaveDomainManager interface {
	Claim(context.Context, devicev1.SaveDomainClaimRequest) (devicev1.SaveDomainClaimResult, error)
	Release(context.Context, devicev1.SaveDomainReleaseRequest) (devicev1.SaveDomainReleaseResult, error)
	Snapshot(context.Context, devicev1.SaveDomainSnapshotRequest) (devicev1.SaveDomainSnapshotResult, error)
	Restore(context.Context, devicev1.SaveDomainRestoreRequest) (devicev1.SaveDomainRestoreResult, error)
	Reconcile(context.Context, devicev1.SaveDomainReconcileRequest) (devicev1.SaveDomainReconcileResult, error)
}

type SaveDomainConfirmer interface {
	ConfirmClaim(context.Context, string, string) (bool, error)
	ConfirmRelease(context.Context, string, string) (bool, error)
	ConfirmRestore(context.Context, string, string) (bool, error)
}

type desktopSaveDomainConfirmer struct{}

func (desktopSaveDomainConfirmer) ConfirmClaim(ctx context.Context, title, server string) (bool, error) {
	return desktop.ConfirmSaveDomainClaim(ctx, title, server)
}

func (desktopSaveDomainConfirmer) ConfirmRelease(ctx context.Context, title, server string) (bool, error) {
	return desktop.ConfirmSaveDomainRelease(ctx, title, server)
}

func (desktopSaveDomainConfirmer) ConfirmRestore(ctx context.Context, title, server string) (bool, error) {
	return desktop.ConfirmSaveDomainRestore(ctx, title, server)
}

// LocalSaveDomainManager owns local confirmation and the client-only save path.
// Server-supplied IDs are correlation data and never override catalog authority.
type LocalSaveDomainManager struct {
	catalog     *SaveDomainCatalog
	bindingID   string
	serverURL   string
	saveRoot    string
	confirmer   SaveDomainConfirmer
	client      *http.Client
	coordinator *InstallationCoordinator
	detector    ScummVMRouteDetector
	now         func() time.Time
}

func NewLocalSaveDomainManager(ownership *InstallationOwnership, serverURL string) (*LocalSaveDomainManager, error) {
	if ownership == nil || ownership.saveDomains == nil || strings.TrimSpace(ownership.bindingID) == "" || strings.TrimSpace(ownership.saveRoot) == "" {
		return nil, errors.New("save domain authority is unavailable")
	}
	if strings.TrimSpace(serverURL) == "" {
		return nil, errors.New("server URL is required")
	}
	detector, err := newLocalScummVMRouteDetector()
	if err != nil {
		return nil, err
	}
	return &LocalSaveDomainManager{catalog: ownership.saveDomains, bindingID: ownership.bindingID, serverURL: serverURL, saveRoot: ownership.saveRoot, confirmer: desktopSaveDomainConfirmer{}, client: &http.Client{Timeout: 0}, coordinator: ownership.coordinator, detector: detector, now: time.Now}, nil
}

func (m *LocalSaveDomainManager) Claim(ctx context.Context, request devicev1.SaveDomainClaimRequest) (devicev1.SaveDomainClaimResult, error) {
	var result devicev1.SaveDomainClaimResult
	if m == nil || m.catalog == nil || m.confirmer == nil || m.now == nil {
		return result, errors.New("save domain manager is unavailable")
	}
	if err := request.Validate(); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	routeFingerprint := strings.ToLower(request.RouteFingerprint)
	savePath := filepath.Join(m.saveRoot, "scummvm", routeFingerprint[:16])
	evidenceBytes := sha256.Sum256([]byte("scummvm-explicit-savepath-v1\x00" + routeFingerprint))
	evidenceFingerprint := hex.EncodeToString(evidenceBytes[:])
	domain, err := m.catalog.Resolve("scummvm", routeFingerprint, evidenceFingerprint, []string{savePath})
	if err != nil {
		return result, err
	}
	if request.LocalSaveDomainID != "" && request.LocalSaveDomainID != domain.LocalSaveDomainID {
		return result, errors.New("requested save domain does not match this exact ScummVM route")
	}
	if domain.State == SaveDomainOwned && strings.EqualFold(domain.WriterBindingID, m.bindingID) {
		return claimResult(request, domain, m.now().UTC())
	}
	if domain.State == SaveDomainOwned {
		return result, errors.New("this save domain is already managed by another MGA Server")
	}
	if domain.State == SaveDomainReconciliationRequired {
		return result, errors.New("this save domain must be reconciled before another MGA Server can manage it")
	}
	if m.detector == nil {
		return result, errors.New("ScummVM route detector is unavailable")
	}
	gameID, err := m.detector.Detect(ctx, routeFingerprint)
	if err != nil {
		return result, err
	}
	if err := m.catalog.SetScummVMGameID(domain.LocalSaveDomainID, gameID); err != nil {
		return result, err
	}
	domain, _ = m.catalog.FindByID(domain.LocalSaveDomainID)
	approved, err := m.confirmer.ConfirmClaim(ctx, request.Title, m.serverURL)
	if err != nil {
		return result, err
	}
	if !approved {
		return result, ErrSaveDomainConfirmationDeclined
	}
	if err := os.MkdirAll(savePath, 0o700); err != nil {
		return result, fmt.Errorf("prepare local ScummVM save folder: %w", err)
	}
	if err := m.catalog.Claim(domain.LocalSaveDomainID, m.bindingID); err != nil {
		return result, err
	}
	domain, _ = m.catalog.FindByID(domain.LocalSaveDomainID)
	return claimResult(request, domain, m.now().UTC())
}

func (m *LocalSaveDomainManager) Release(ctx context.Context, request devicev1.SaveDomainReleaseRequest) (devicev1.SaveDomainReleaseResult, error) {
	var result devicev1.SaveDomainReleaseResult
	if m == nil || m.catalog == nil || m.confirmer == nil || m.coordinator == nil || m.now == nil {
		return result, errors.New("save domain manager is unavailable")
	}
	if err := request.Validate(); err != nil {
		return result, err
	}
	domain, found := m.catalog.FindByID(request.LocalSaveDomainID)
	if !found {
		return result, errors.New("save domain record not found")
	}
	if domain.State != SaveDomainOwned || !strings.EqualFold(domain.WriterBindingID, m.bindingID) {
		return result, errors.New("this MGA Server is not the save writer")
	}
	releaseLease, err := m.coordinator.Reserve(m.bindingID, domain.ResolvedPaths[0], "save-domain:"+domain.LocalSaveDomainID)
	if err != nil {
		return result, err
	}
	defer releaseLease()
	approved, err := m.confirmer.ConfirmRelease(ctx, request.Title, m.serverURL)
	if err != nil {
		return result, err
	}
	if !approved {
		return result, ErrSaveDomainConfirmationDeclined
	}
	if err := m.catalog.Release(domain.LocalSaveDomainID, m.bindingID); err != nil {
		return result, err
	}
	result = devicev1.SaveDomainReleaseResult{GameID: request.GameID, SourceGameID: request.SourceGameID, LocalSaveDomainID: domain.LocalSaveDomainID, State: "released", ReleasedAt: m.now().UTC()}
	return result, result.Validate()
}

func claimResult(request devicev1.SaveDomainClaimRequest, domain LocalSaveDomainRecord, grantedAt time.Time) (devicev1.SaveDomainClaimResult, error) {
	state := "owned_here"
	if domain.State == SaveDomainReconciliationRequired {
		state = "reconciliation_required"
	}
	result := devicev1.SaveDomainClaimResult{GameID: request.GameID, SourceGameID: request.SourceGameID, LocalSaveDomainID: domain.LocalSaveDomainID, AdapterID: domain.AdapterID, RouteFingerprint: domain.RouteFingerprint, State: state, GrantedAt: grantedAt}
	return result, result.Validate()
}
