//go:build !windows

package clientapp

import (
	"context"
	"time"
)

type unsupportedAuthenticodeVerifier struct{}
type unsupportedInnoFamilyDetector struct{}
type unsupportedLocalConfirmer struct{}
type unsupportedInstallerProcessRunner struct{}

func newAuthenticodeVerifier() AuthenticodeVerifier { return unsupportedAuthenticodeVerifier{} }
func newInnoFamilyDetector() InnoFamilyDetector     { return unsupportedInnoFamilyDetector{} }
func newLocalConfirmer() LocalConfirmer             { return unsupportedLocalConfirmer{} }
func newInstallerProcessRunner() InstallerProcessRunner {
	return unsupportedInstallerProcessRunner{}
}

func (unsupportedAuthenticodeVerifier) VerifyGOG(string) (string, string, error) {
	return "", "", ErrUnsupportedInstallerPlatform
}

func (unsupportedInnoFamilyDetector) IsInnoSetup(string) (bool, error) {
	return false, ErrUnsupportedInstallerPlatform
}

func (unsupportedLocalConfirmer) ConfirmInstall(context.Context, InstallConfirmationDetails, time.Duration) error {
	return ErrUnsupportedInstallerPlatform
}

func (unsupportedLocalConfirmer) ConfirmUninstall(context.Context, UninstallConfirmationDetails, time.Duration) error {
	return ErrUnsupportedInstallerPlatform
}

func (unsupportedInstallerProcessRunner) Start(context.Context, InstallerProcessSpec) (InstallerProcess, error) {
	return nil, ErrUnsupportedInstallerPlatform
}
