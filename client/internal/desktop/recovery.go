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
