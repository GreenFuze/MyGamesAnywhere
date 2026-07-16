//go:build !windows

package clientapp

import (
	"context"
	"os"
)

type unsupportedAuthenticodeVerifier struct{}
type unsupportedInnoFamilyDetector struct{}
type unsupportedInstallerProcessRunner struct{}
type unsupportedRegisteredProgramInspector struct{}

func newAuthenticodeVerifier() AuthenticodeVerifier { return unsupportedAuthenticodeVerifier{} }
func newInnoFamilyDetector() InnoFamilyDetector     { return unsupportedInnoFamilyDetector{} }
func newInstallerProcessRunner() InstallerProcessRunner {
	return unsupportedInstallerProcessRunner{}
}
func newRegisteredProgramInspector() RegisteredProgramInspector {
	return unsupportedRegisteredProgramInspector{}
}

func (unsupportedAuthenticodeVerifier) VerifyGOG(string) (string, string, error) {
	return "", "", ErrUnsupportedInstallerPlatform
}

func (unsupportedInnoFamilyDetector) IsInnoSetup(string) (bool, error) {
	return false, ErrUnsupportedInstallerPlatform
}

func isFilesystemReparsePoint(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSymlink != 0, nil
}

func filesystemObjectIdentity(string) (string, error) {
	return "", ErrUnsupportedInstallerPlatform
}

func (unsupportedInstallerProcessRunner) Start(context.Context, InstallerProcessSpec) (InstallerProcess, error) {
	return nil, ErrUnsupportedInstallerPlatform
}

func (unsupportedRegisteredProgramInspector) HasAssociation(string) (bool, error) {
	return false, ErrUnsupportedInstallerPlatform
}
