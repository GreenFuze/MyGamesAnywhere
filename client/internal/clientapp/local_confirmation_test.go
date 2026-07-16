package clientapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ncruces/zenity"
)

type fakeConfirmationDialog struct {
	err          error
	title        string
	message      string
	approveLabel string
}

func (d *fakeConfirmationDialog) Question(_ context.Context, title, message, approveLabel string) error {
	d.title = title
	d.message = message
	d.approveLabel = approveLabel
	return d.err
}

func TestZenityLocalConfirmerMapsDialogResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dialog  error
		wantErr error
	}{
		{name: "approved"},
		{name: "declined", dialog: zenity.ErrCanceled, wantErr: ErrLocalConfirmationDeclined},
		{name: "timed out", dialog: context.DeadlineExceeded, wantErr: ErrLocalConfirmationTimeout},
		{name: "failed", dialog: errors.New("dialog unavailable"), wantErr: errors.New("show local confirmation dialog: dialog unavailable")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dialog := &fakeConfirmationDialog{err: test.dialog}
			confirmer := zenityLocalConfirmer{dialog: dialog}
			err := confirmer.confirm(context.Background(), "Confirm", "Proceed?", "Approve", time.Second)
			if test.wantErr == nil && err != nil {
				t.Fatalf("confirm() error = %v", err)
			}
			if test.wantErr != nil && (err == nil || err.Error() != test.wantErr.Error()) {
				t.Fatalf("confirm() error = %v, want %v", err, test.wantErr)
			}
			if dialog.title != "Confirm" || dialog.message != "Proceed?" || dialog.approveLabel != "Approve" {
				t.Fatalf("dialog arguments = %q, %q, %q", dialog.title, dialog.message, dialog.approveLabel)
			}
		})
	}
}

func TestZenityLocalConfirmerRejectsInvalidTimeout(t *testing.T) {
	t.Parallel()

	confirmer := zenityLocalConfirmer{dialog: &fakeConfirmationDialog{}}
	if err := confirmer.confirm(context.Background(), "Confirm", "Proceed?", "Approve", 0); err == nil {
		t.Fatal("zero timeout was accepted")
	}
}
