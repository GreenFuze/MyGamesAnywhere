//go:build windows

package clientapp

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wtdUIChoiceNone                          = 2
	wtdRevokeWholeChain                      = 1
	wtdChoiceFile                            = 1
	wtdStateActionVerify                     = 1
	wtdStateActionClose                      = 2
	wtdRevocationCheckChainExcludeRoot       = 0x80
	seeMaskNoCloseProcess                    = 0x00000040
	swShowNormal                             = 1
	idOK                                     = 1
	idCancel                                 = 2
	idTimeout                                = 32000
	messageBoxOKCancel                       = 0x00000001
	messageBoxIconWarning                    = 0x00000030
	messageBoxSetForeground                  = 0x00010000
	maxInnoDetectionBytes              int64 = 32 * 1024 * 1024
)

var (
	shell32                = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW    = shell32.NewProc("ShellExecuteExW")
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxTimeoutW = user32.NewProc("MessageBoxTimeoutW")
	wintrust               = windows.NewLazySystemDLL("wintrust.dll")
	procWTHelperProvData   = wintrust.NewProc("WTHelperProvDataFromStateData")
	procWTHelperProvSigner = wintrust.NewProc("WTHelperGetProvSignerFromChain")
)

type windowsAuthenticodeVerifier struct{}
type boundedInnoFamilyDetector struct{}
type windowsLocalConfirmer struct{}
type windowsInstallerProcessRunner struct{}

type cryptProviderCert struct {
	Size                uint32
	Certificate         *windows.CertContext
	Commercial          int32
	TrustedRoot         int32
	SelfSigned          int32
	TestCertificate     int32
	RevokedReason       uint32
	Confidence          uint32
	Error               uint32
	TrustListContext    unsafe.Pointer
	TrustListSignerCert int32
	ControlContext      unsafe.Pointer
	ControlError        uint32
	IsCyclic            int32
	ChainElement        unsafe.Pointer
}

type cryptProviderSigner struct {
	Size           uint32
	VerifyAsOf     windows.Filetime
	CertChainCount uint32
	CertChain      *cryptProviderCert
	SignerType     uint32
	SignerInfo     unsafe.Pointer
	Error          uint32
	CounterSigners uint32
	CounterSigner  unsafe.Pointer
	ChainContext   unsafe.Pointer
}

func newAuthenticodeVerifier() AuthenticodeVerifier { return windowsAuthenticodeVerifier{} }
func newInnoFamilyDetector() InnoFamilyDetector     { return boundedInnoFamilyDetector{} }
func newLocalConfirmer() LocalConfirmer             { return windowsLocalConfirmer{} }
func newInstallerProcessRunner() InstallerProcessRunner {
	return windowsInstallerProcessRunner{}
}

func (windowsAuthenticodeVerifier) VerifyGOG(path string) (string, string, error) {
	filePath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", "", err
	}
	fileInfo := windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: filePath}
	trustData := windows.WinTrustData{
		Size: uint32(unsafe.Sizeof(windows.WinTrustData{})), UIChoice: wtdUIChoiceNone,
		RevocationChecks: wtdRevokeWholeChain, UnionChoice: wtdChoiceFile,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&fileInfo), StateAction: wtdStateActionVerify,
		ProvFlags: wtdRevocationCheckChainExcludeRoot,
	}
	verifyErr := windows.WinVerifyTrustEx(0, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &trustData)
	if verifyErr != nil {
		trustData.StateAction = wtdStateActionClose
		_ = windows.WinVerifyTrustEx(0, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &trustData)
		return "", "", fmt.Errorf("WinVerifyTrust rejected installer: %w", verifyErr)
	}
	defer func() {
		trustData.StateAction = wtdStateActionClose
		_ = windows.WinVerifyTrustEx(0, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &trustData)
	}()
	providerData, _, _ := procWTHelperProvData.Call(uintptr(trustData.StateData))
	if providerData == 0 {
		return "", "", errors.New("WinVerifyTrust did not return signer data")
	}
	signerAddress, _, _ := procWTHelperProvSigner.Call(providerData, 0, 0, 0)
	if signerAddress == 0 {
		return "", "", errors.New("WinVerifyTrust did not return a primary signer")
	}
	signer := cryptProviderSignerFromAddress(signerAddress)
	if signer.CertChainCount == 0 || signer.CertChain == nil || signer.CertChain.Certificate == nil {
		return "", "", errors.New("WinVerifyTrust signer has no leaf certificate")
	}
	certificate := signer.CertChain.Certificate
	encoded := unsafe.Slice(certificate.EncodedCert, certificate.Length)
	parsed, err := x509.ParseCertificate(encoded)
	if err != nil {
		return "", "", fmt.Errorf("parse Authenticode signer certificate: %w", err)
	}
	subject := parsed.Subject.String()
	if !signerIsGOG(subject) {
		return "", "", fmt.Errorf("verified signer is not %s", gogPublisherName)
	}
	sum := sha1.Sum(parsed.Raw)
	return subject, strings.ToUpper(hex.EncodeToString(sum[:])), nil
}

// WTHelperGetProvSignerFromChain returns a WinTrust-owned pointer as uintptr.
//
//go:nocheckptr
func cryptProviderSignerFromAddress(address uintptr) *cryptProviderSigner {
	return (*cryptProviderSigner)(unsafe.Add(nil, address))
}

func (boundedInnoFamilyDetector) IsInnoSetup(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	header := make([]byte, 2)
	if _, err := io.ReadFull(file, header); err != nil {
		return false, err
	}
	if !bytes.Equal(header, []byte{'M', 'Z'}) {
		return false, errors.New("installer is not a Windows PE file")
	}
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	window := maxInnoDetectionBytes / 2
	firstSize := min(info.Size(), window)
	first := make([]byte, firstSize)
	if _, err := file.ReadAt(first, 0); err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if bytes.Contains(first, []byte("Inno Setup")) {
		return true, nil
	}
	if info.Size() <= firstSize {
		return false, nil
	}
	lastSize := min(info.Size()-firstSize, window)
	last := make([]byte, lastSize)
	_, err = file.ReadAt(last, info.Size()-lastSize)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	return bytes.Contains(last, []byte("Inno Setup")), nil
}

func (windowsLocalConfirmer) ConfirmInstall(ctx context.Context, details InstallConfirmationDetails, timeout time.Duration) error {
	message := fmt.Sprintf("Install %s?\n\nVerified publisher: %s\nDestination: %s\nServer: %s\n\n%s",
		details.GameTitle, details.Publisher, details.Destination, details.Server, details.PossibleUACNote)
	return showConfirmation(ctx, "MGA Client — Approve installation", message, timeout)
}

func (windowsLocalConfirmer) ConfirmUninstall(ctx context.Context, details UninstallConfirmationDetails, timeout time.Duration) error {
	message := fmt.Sprintf("Uninstall %s?\n\nVerified publisher: %s\nInstalled at: %s\nServer: %s\n\n%s",
		details.GameTitle, details.Publisher, details.InstallPath, details.Server, details.Warning)
	return showConfirmation(ctx, "MGA Client — Approve uninstall", message, timeout)
}

func showConfirmation(ctx context.Context, title, message string, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	text, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return err
	}
	caption, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return err
	}
	timeoutMilliseconds := uint32(timeout / time.Millisecond)
	result, _, callErr := procMessageBoxTimeoutW.Call(
		0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(caption)),
		messageBoxOKCancel|messageBoxIconWarning|messageBoxSetForeground, 0, uintptr(timeoutMilliseconds),
	)
	switch result {
	case idOK:
		return nil
	case idTimeout:
		return ErrLocalConfirmationTimeout
	case idCancel:
		return ErrLocalConfirmationDeclined
	case 0:
		return fmt.Errorf("show local confirmation: %w", callErr)
	default:
		return ErrLocalConfirmationDeclined
	}
}

func (windowsInstallerProcessRunner) Start(_ context.Context, spec InstallerProcessSpec) (InstallerProcess, error) {
	command := exec.Command(spec.Path, spec.Arguments...)
	command.Dir = spec.WorkingDirectory
	if err := command.Start(); err == nil {
		return &normalWindowsProcess{command: command}, nil
	} else if !errors.Is(err, windows.ERROR_ELEVATION_REQUIRED) {
		return nil, err
	}
	return startElevatedProcess(spec)
}

type normalWindowsProcess struct {
	command *exec.Cmd
}

func (p *normalWindowsProcess) PID() int { return p.command.Process.Pid }

func (p *normalWindowsProcess) Wait(ctx context.Context, timeout time.Duration) (int, error) {
	type waitResult struct {
		code int
		err  error
	}
	done := make(chan waitResult, 1)
	go func() {
		err := p.command.Wait()
		code := -1
		if p.command.ProcessState != nil {
			code = p.command.ProcessState.ExitCode()
		}
		done <- waitResult{code: code, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-done:
		if result.err != nil {
			var exitErr *exec.ExitError
			if !errors.As(result.err, &exitErr) {
				return result.code, result.err
			}
		}
		return result.code, nil
	case <-ctx.Done():
		return -1, ctx.Err()
	case <-timer.C:
		return -1, context.DeadlineExceeded
	}
}

type shellExecuteInfo struct {
	Size       uint32
	Mask       uint32
	HWND       windows.Handle
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	Instance   windows.Handle
	IDList     unsafe.Pointer
	Class      *uint16
	ClassKey   windows.Handle
	HotKey     uint32
	Icon       windows.Handle
	Process    windows.Handle
}

func startElevatedProcess(spec InstallerProcessSpec) (InstallerProcess, error) {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(spec.Path)
	if err != nil {
		return nil, err
	}
	escaped := make([]string, len(spec.Arguments))
	for index, argument := range spec.Arguments {
		escaped[index] = syscall.EscapeArg(argument)
	}
	parameters, err := windows.UTF16PtrFromString(strings.Join(escaped, " "))
	if err != nil {
		return nil, err
	}
	directory, err := windows.UTF16PtrFromString(spec.WorkingDirectory)
	if err != nil {
		return nil, err
	}
	info := shellExecuteInfo{
		Mask: seeMaskNoCloseProcess, Verb: verb, File: file, Parameters: parameters,
		Directory: directory, Show: swShowNormal,
	}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return nil, ErrUACDeclined
		}
		return nil, fmt.Errorf("start elevated installer: %w", callErr)
	}
	if info.Process == 0 {
		return nil, errors.New("elevated installer did not return a process handle")
	}
	pid, err := windows.GetProcessId(info.Process)
	if err != nil {
		windows.CloseHandle(info.Process)
		return nil, err
	}
	return &elevatedWindowsProcess{handle: info.Process, pid: int(pid)}, nil
}

type elevatedWindowsProcess struct {
	handle windows.Handle
	pid    int
}

func (p *elevatedWindowsProcess) PID() int { return p.pid }

func (p *elevatedWindowsProcess) Wait(ctx context.Context, timeout time.Duration) (int, error) {
	done := make(chan error, 1)
	go func() {
		_, err := windows.WaitForSingleObject(p.handle, windows.INFINITE)
		done <- err
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			return -1, err
		}
		var code uint32
		if err := windows.GetExitCodeProcess(p.handle, &code); err != nil {
			return -1, err
		}
		windows.CloseHandle(p.handle)
		p.handle = 0
		return int(code), nil
	case <-ctx.Done():
		return -1, ctx.Err()
	case <-timer.C:
		return -1, context.DeadlineExceeded
	}
}
