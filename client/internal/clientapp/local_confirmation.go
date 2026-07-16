package clientapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ncruces/zenity"
)

type localConfirmationDialog interface {
	Question(context.Context, string, string, string) error
}

type zenityConfirmationDialog struct{}

type zenityLocalConfirmer struct {
	dialog localConfirmationDialog
}

func newLocalConfirmer() LocalConfirmer {
	return zenityLocalConfirmer{dialog: zenityConfirmationDialog{}}
}

func (c zenityLocalConfirmer) ConfirmUninstall(ctx context.Context, details UninstallConfirmationDetails, timeout time.Duration) error {
	message := fmt.Sprintf("Uninstall %s?\n\nVerified publisher: %s\nInstalled at: %s\nServer: %s\n\n%s",
		details.GameTitle, details.Publisher, details.InstallPath, details.Server, details.Warning)
	return c.confirm(ctx, "MGA Client — Approve uninstall", message, "Uninstall", timeout)
}

func (c zenityLocalConfirmer) ConfirmCleanup(ctx context.Context, details CleanupConfirmationDetails, timeout time.Duration) error {
	message := fmt.Sprintf("Clean up failed install for %s?\n\nFolder: %s\nServer: %s\n\n%s",
		details.GameTitle, details.InstallPath, details.Server, details.Warning)
	return c.confirm(ctx, "MGA Client — Confirm cleanup", message, "Clean up", timeout)
}

func (c zenityLocalConfirmer) confirm(ctx context.Context, title, message, approveLabel string, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if timeout <= 0 {
		return errors.New("local confirmation timeout must be positive")
	}
	if c.dialog == nil {
		return errors.New("local confirmation dialog is not configured")
	}
	dialogContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := c.dialog.Question(dialogContext, title, message, approveLabel)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, zenity.ErrCanceled):
		return ErrLocalConfirmationDeclined
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(dialogContext.Err(), context.DeadlineExceeded):
		return ErrLocalConfirmationTimeout
	case ctx.Err() != nil:
		return ctx.Err()
	default:
		return fmt.Errorf("show local confirmation dialog: %w", err)
	}
}

func (zenityConfirmationDialog) Question(ctx context.Context, title, message, approveLabel string) error {
	return zenity.Question(message,
		zenity.Title(title),
		zenity.OKLabel(approveLabel),
		zenity.CancelLabel("Cancel"),
		zenity.WarningIcon,
		zenity.DefaultCancel(),
		zenity.Context(ctx),
	)
}
