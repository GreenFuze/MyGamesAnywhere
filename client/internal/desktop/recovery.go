package desktop

import (
	"context"
	"fmt"
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
