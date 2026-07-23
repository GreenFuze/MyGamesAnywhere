package desktop

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ncruces/zenity"
)

// ConfirmServerUnbind explains a pairing mismatch and requires a local choice.
func ConfirmServerUnbind(ctx context.Context, pairedServer, requestedServer string) (bool, error) {
	if strings.TrimSpace(pairedServer) == "" || strings.TrimSpace(requestedServer) == "" {
		return false, fmt.Errorf("paired and requested server URLs are required")
	}
	err := zenity.Question(
		fmt.Sprintf("This MGA Client is paired with:\n%s\n\nThis browser is using:\n%s\n\nUnbind this Windows user from the old server? Afterward, return to MGA and choose ‘Pair this Windows user’.", pairedServer, requestedServer),
		zenity.Title("MGA Client — Different server"),
		zenity.OKLabel("Unbind"),
		zenity.CancelLabel("Keep current pairing"),
		zenity.WarningIcon,
		zenity.DefaultCancel(),
		zenity.Context(ctx),
	)
	if err == nil {
		return true, nil
	}
	if err == zenity.ErrCanceled {
		return false, nil
	}
	return false, fmt.Errorf("show server pairing dialog: %w", err)
}

// ConfirmUnbind requires a local choice before removing the current pairing.
func ConfirmUnbind(ctx context.Context, pairedServer string) (bool, error) {
	if strings.TrimSpace(pairedServer) == "" {
		return false, fmt.Errorf("paired server URL is required")
	}
	err := zenity.Question(
		fmt.Sprintf("Unbind this Windows user from:\n%s\n\nThe local client identity will be removed. MGA's historical device entry will remain until an administrator removes it.", pairedServer),
		zenity.Title("MGA Client — Unbind from server"),
		zenity.OKLabel("Unbind"),
		zenity.CancelLabel("Cancel"),
		zenity.WarningIcon,
		zenity.DefaultCancel(),
		zenity.Context(ctx),
	)
	if err == nil {
		return true, nil
	}
	if err == zenity.ErrCanceled {
		return false, nil
	}
	return false, fmt.Errorf("show unbind dialog: %w", err)
}

// ShowError makes protocol-handler failures visible for the windowless client.
func ShowError(title, message string) error {
	return zenity.Error(message, zenity.Title(title), zenity.OKLabel("OK"))
}

func ConfirmInstallationRelease(ctx context.Context, title, path, server string) (bool, error) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(path) == "" || strings.TrimSpace(server) == "" {
		return false, fmt.Errorf("installation title, path, and server are required")
	}
	err := zenity.Question(
		fmt.Sprintf("Release %s from:\n%s\n\nInstalled files will stay at:\n%s\n\nAnother paired MGA Server can pick it up later. The current server will no longer be allowed to update or uninstall it.", title, server, path),
		zenity.Title("MGA Client — Release installation"), zenity.OKLabel("Release"), zenity.CancelLabel("Cancel"), zenity.WarningIcon, zenity.DefaultCancel(), zenity.Context(ctx),
	)
	if err == nil {
		return true, nil
	}
	if err == zenity.ErrCanceled {
		return false, nil
	}
	return false, fmt.Errorf("show release dialog: %w", err)
}

func ConfirmInstallationAdoption(ctx context.Context, title, path, server string) (bool, error) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(path) == "" || strings.TrimSpace(server) == "" {
		return false, fmt.Errorf("installation title, path, and server are required")
	}
	err := zenity.Question(
		fmt.Sprintf("Let this MGA Server manage %s?\n\nServer:\n%s\n\nInstalled at:\n%s\n\nThe server will be allowed to launch, update, repair, and uninstall this installation when those actions are supported.", title, server, path),
		zenity.Title("MGA Client — Pick up installation"), zenity.OKLabel("Pick up"), zenity.CancelLabel("Cancel"), zenity.WarningIcon, zenity.DefaultCancel(), zenity.Context(ctx),
	)
	if err == nil {
		return true, nil
	}
	if err == zenity.ErrCanceled {
		return false, nil
	}
	return false, fmt.Errorf("show adoption dialog: %w", err)
}

func ConfirmUseExistingInstallation(ctx context.Context, title, path, server string) (bool, error) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(path) == "" || strings.TrimSpace(server) == "" {
		return false, fmt.Errorf("installation title, path, and server are required")
	}
	err := zenity.Question(
		fmt.Sprintf("Let this MGA Server play %s using the installation at:\n%s\n\nServer:\n%s\n\nThis gives launch access only. It does not let this server update, repair, or uninstall the game.", title, path, server),
		zenity.Title("MGA Client — Use existing installation"), zenity.OKLabel("Allow play"), zenity.CancelLabel("Cancel"), zenity.QuestionIcon, zenity.DefaultCancel(), zenity.Context(ctx),
	)
	if err == nil {
		return true, nil
	}
	if err == zenity.ErrCanceled {
		return false, nil
	}
	return false, fmt.Errorf("show use-existing dialog: %w", err)
}

type SaveDomainClaimDetails struct {
	Title       string
	Server      string
	Adapter     string
	ExactTarget string
	SaveKind    string
	LocalPath   string
}

func (d SaveDomainClaimDetails) Validate() error {
	for name, value := range map[string]string{
		"game title":   d.Title,
		"server":       d.Server,
		"adapter":      d.Adapter,
		"exact target": d.ExactTarget,
		"save kind":    d.SaveKind,
		"local path":   d.LocalPath,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s must be a single line", name)
		}
	}
	if !filepath.IsAbs(d.LocalPath) {
		return fmt.Errorf("local save path must be absolute")
	}
	return nil
}

func ConfirmSaveDomainClaim(ctx context.Context, details SaveDomainClaimDetails) (bool, error) {
	if err := details.Validate(); err != nil {
		return false, err
	}
	err := zenity.Question(
		fmt.Sprintf(
			"Let this MGA Server manage saves for %s?\n\nServer:\n%s\n\nGame route:\n%s — %s\n\nSave data:\n%s\n\nLocal folder:\n%s\n\nOnly this MGA Server will be allowed to restore this exact route's local saves. Another server needs a separate local confirmation.",
			details.Title,
			details.Server,
			details.Adapter,
			details.ExactTarget,
			details.SaveKind,
			details.LocalPath,
		),
		zenity.Title("MGA Client — Manage game saves"), zenity.OKLabel("Allow save backup"), zenity.CancelLabel("Cancel"), zenity.QuestionIcon, zenity.DefaultCancel(), zenity.Context(ctx),
	)
	if err == nil {
		return true, nil
	}
	if err == zenity.ErrCanceled {
		return false, nil
	}
	return false, fmt.Errorf("show save-domain confirmation: %w", err)
}

func ConfirmSaveDomainRelease(ctx context.Context, title, server string) (bool, error) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(server) == "" {
		return false, fmt.Errorf("game title and server are required")
	}
	err := zenity.Question(
		fmt.Sprintf("Release this MGA Server's save access for %s?\n\nServer:\n%s\n\nLocal save files will stay exactly where they are. Another paired MGA Server may claim them later, but it must reconcile its backup before restoring anything.", title, server),
		zenity.Title("MGA Client — Release game saves"), zenity.OKLabel("Release access"), zenity.CancelLabel("Cancel"), zenity.WarningIcon, zenity.DefaultCancel(), zenity.Context(ctx),
	)
	if err == nil {
		return true, nil
	}
	if err == zenity.ErrCanceled {
		return false, nil
	}
	return false, fmt.Errorf("show save-domain release dialog: %w", err)
}

// ConfirmSaveDomainRestore protects an explicit conflict resolution that will
// replace the live save folder. MGA keeps a timestamped local backup first.
func ConfirmSaveDomainRestore(ctx context.Context, title, server string) (bool, error) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(server) == "" {
		return false, fmt.Errorf("game title and server are required")
	}
	err := zenity.Question(
		fmt.Sprintf("%s wants MGA Client to restore the saved backup for %s.\n\nYour current files will be kept in a separate backup folder first.", server, title),
		zenity.Title("MGA Client — Restore game saves"), zenity.OKLabel("Restore and keep both"), zenity.CancelLabel("Cancel"), zenity.WarningIcon, zenity.DefaultCancel(), zenity.Context(ctx),
	)
	if err == nil {
		return true, nil
	}
	if err == zenity.ErrCanceled {
		return false, nil
	}
	return false, fmt.Errorf("show save restore dialog: %w", err)
}
