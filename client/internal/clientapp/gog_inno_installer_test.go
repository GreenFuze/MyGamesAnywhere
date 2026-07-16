package clientapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type fakeAuthenticodeVerifier struct {
	subject    string
	thumbprint string
	err        error
}

func (f fakeAuthenticodeVerifier) VerifyGOG(string) (string, string, error) {
	return f.subject, f.thumbprint, f.err
}

type fakeInnoDetector struct {
	inno bool
	err  error
}

func (f fakeInnoDetector) IsInnoSetup(string) (bool, error) { return f.inno, f.err }

type fakeLocalConfirmer struct {
	uninstallErr   error
	cleanupErr     error
	uninstallCalls *int
	cleanupCalls   *int
}

func (f fakeLocalConfirmer) ConfirmUninstall(context.Context, UninstallConfirmationDetails, time.Duration) error {
	if f.uninstallCalls != nil {
		(*f.uninstallCalls)++
	}
	return f.uninstallErr
}

func (f fakeLocalConfirmer) ConfirmCleanup(context.Context, CleanupConfirmationDetails, time.Duration) error {
	if f.cleanupCalls != nil {
		(*f.cleanupCalls)++
	}
	return f.cleanupErr
}

type fakeInstallerProcessRunner struct {
	specs    []InstallerProcessSpec
	startErr error
	exitCode int
	waitErr  error
	onStart  func(InstallerProcessSpec) error
}

func (f *fakeInstallerProcessRunner) Start(_ context.Context, spec InstallerProcessSpec) (InstallerProcess, error) {
	f.specs = append(f.specs, spec)
	if f.startErr != nil {
		return nil, f.startErr
	}
	if f.onStart != nil {
		if err := f.onStart(spec); err != nil {
			return nil, err
		}
	}
	return fakeInstallerProcess{exitCode: f.exitCode, waitErr: f.waitErr}, nil
}

type fakeInstallerProcess struct {
	exitCode int
	waitErr  error
}

type fakeRegisteredProgramInspector struct {
	associated bool
	err        error
	calls      int
}

func (f *fakeRegisteredProgramInspector) HasAssociation(string) (bool, error) {
	f.calls++
	return f.associated, f.err
}

func (fakeInstallerProcess) PID() int { return 4242 }
func (f fakeInstallerProcess) Wait(context.Context, time.Duration) (int, error) {
	return f.exitCode, f.waitErr
}

func TestManagedGogInnoInstallerMultiFileProgressFixedArgsAndManifest(t *testing.T) {
	t.Parallel()
	installerBytes := []byte("MZ signed Inno Setup package")
	companionBytes := []byte("companion payload")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/setup.exe":
			_, _ = w.Write(installerBytes)
		case "/setup-1.bin":
			_, _ = w.Write(companionBytes)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	runner := &fakeInstallerProcessRunner{}
	runner.onStart = func(spec InstallerProcessSpec) error {
		destination := fixedArgumentValue(spec.Arguments, "/DIR=")
		if destination == "" {
			return errors.New("missing fixed destination argument")
		}
		if err := os.MkdirAll(destination, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destination, "Game.exe"), []byte("game"), 0o600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(destination, "unins000.exe"), []byte("uninstall"), 0o600)
	}
	installer := newFakeGogInstaller(t, server.URL, fakeLocalConfirmer{}, runner)
	var updates []CommandProgressUpdate
	result, err := installer.Install(context.Background(), "command-multi", devicev1.GogInnoInstallRequest{
		GameID: "game-1", SourceGameID: "source-1", Title: "Game", DestinationRoot: t.TempDir(), DestinationName: "Game",
		Installer: devicev1.PackageTransferDescriptor{
			FileName: "setup_game.exe", Role: devicev1.PackageTransferRoleInstaller, SizeBytes: uint64(len(installerBytes)),
			DownloadURL: "/setup.exe", DownloadToken: "token",
		},
		Companions: []devicev1.PackageTransferDescriptor{{
			FileName: "setup_game-1.bin", Role: devicev1.PackageTransferRoleCompanion, SizeBytes: uint64(len(companionBytes)),
			DownloadURL: "/setup-1.bin", DownloadToken: "token",
		}},
	}, func(update CommandProgressUpdate) error {
		updates = append(updates, update)
		return nil
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(result.PackageFiles) != 2 || result.TotalPackageBytes != uint64(len(installerBytes)+len(companionBytes)) {
		t.Fatalf("package result = %#v", result)
	}
	if result.LaunchTarget != "Game.exe" || result.UninstallTarget != "unins000.exe" || result.ProcessID != 4242 {
		t.Fatalf("discovery/process result = %#v", result)
	}
	if len(runner.specs) != 1 {
		t.Fatalf("process starts = %d", len(runner.specs))
	}
	wantArgs := []string{"/SP-", "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART"}
	for _, argument := range wantArgs {
		if !containsExact(runner.specs[0].Arguments, argument) {
			t.Errorf("arguments %v do not contain %q", runner.specs[0].Arguments, argument)
		}
	}
	if fixedArgumentValue(runner.specs[0].Arguments, "/DIR=") != result.InstallPath ||
		fixedArgumentValue(runner.specs[0].Arguments, "/LOG=") == "" ||
		runner.specs[0].WorkingDirectory == "" {
		t.Fatalf("process spec = %#v", runner.specs[0])
	}
	foundAggregateDownload := false
	for _, update := range updates {
		if update.Stage == "download" && update.StagePercent == 100 {
			foundAggregateDownload = true
		}
	}
	if !foundAggregateDownload {
		t.Fatalf("aggregate progress = %+v", updates)
	}
	data, err := os.ReadFile(filepath.Join(result.InstallPath, installManifestName))
	if err != nil {
		t.Fatal(err)
	}
	var manifest gogInnoManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 3 || manifest.UninstallTarget != "unins000.exe" || len(manifest.PackageFiles) != 2 {
		t.Fatalf("manifest = %#v", manifest)
	}
	if result.CompletionBasis != devicev1.GogInnoCompletionExitZero || manifest.CompletionBasis != devicev1.GogInnoCompletionExitZero {
		t.Fatalf("completion basis result=%q manifest=%q", result.CompletionBasis, manifest.CompletionBasis)
	}
	markers, markerErr := filepath.Glob(filepath.Join(result.InstallRoot, ".mga", gogInnoFailureMarkerDirectory, "*.json"))
	if markerErr != nil || len(markers) != 0 {
		t.Fatalf("committed install retained failure marker: markers=%v error=%v", markers, markerErr)
	}
}

func TestManagedGogInnoInstallerRejectsValidationAndExecutionFailures(t *testing.T) {
	t.Parallel()
	payload := []byte("MZ Inno Setup")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	baseRequest := func(root string) devicev1.GogInnoInstallRequest {
		return devicev1.GogInnoInstallRequest{
			GameID: "game", SourceGameID: "source", Title: "Game", DestinationRoot: root, DestinationName: "Game",
			Installer: devicev1.PackageTransferDescriptor{
				FileName: "setup_game.exe", Role: devicev1.PackageTransferRoleInstaller, SizeBytes: uint64(len(payload)),
				DownloadURL: server.URL, DownloadToken: "token",
			},
		}
	}
	tests := []struct {
		name      string
		verifier  fakeAuthenticodeVerifier
		detector  fakeInnoDetector
		confirmer fakeLocalConfirmer
		runner    *fakeInstallerProcessRunner
		code      string
	}{
		{name: "invalid signature", verifier: fakeAuthenticodeVerifier{err: errors.New("bad signature")}, detector: fakeInnoDetector{inno: true}, runner: &fakeInstallerProcessRunner{}, code: "invalid_installer_signature"},
		{name: "wrong publisher", verifier: fakeAuthenticodeVerifier{subject: "Other Publisher", thumbprint: "AA"}, detector: fakeInnoDetector{inno: true}, runner: &fakeInstallerProcessRunner{}, code: "invalid_installer_signature"},
		{name: "wrong family", verifier: validFakeVerifier(), detector: fakeInnoDetector{inno: false}, runner: &fakeInstallerProcessRunner{}, code: "unsupported_installer"},
		{name: "UAC declined", verifier: validFakeVerifier(), detector: fakeInnoDetector{inno: true}, runner: &fakeInstallerProcessRunner{startErr: ErrUACDeclined}, code: "uac_declined"},
		{name: "nonzero", verifier: validFakeVerifier(), detector: fakeInnoDetector{inno: true}, runner: &fakeInstallerProcessRunner{exitCode: 7}, code: "installer_exit_nonzero"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			installer, err := NewManagedGogInnoInstaller(server.URL, test.verifier, test.detector, test.confirmer, test.runner)
			if err != nil {
				t.Fatal(err)
			}
			_, err = installer.Install(context.Background(), "command-"+strings.ReplaceAll(test.name, " ", "-"), baseRequest(t.TempDir()), nil)
			assertGogErrorCode(t, err, test.code)
			if (test.code == "invalid_installer_signature" || test.code == "unsupported_installer") && len(test.runner.specs) != 0 {
				t.Fatalf("process executed after pre-start failure: %#v", test.runner.specs)
			}
		})
	}
}

func TestManagedGogInnoInstallerAcceptsOnlyExactValidatedPostSuccessCrash(t *testing.T) {
	t.Parallel()
	payload := []byte("MZ Inno Setup")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	crashExit := int(uint32(0xC000041D))

	tests := []struct {
		name             string
		exitCode         int
		log              string
		writeUninstaller bool
		wantSuccess      bool
		wantCode         string
	}{
		{name: "exact crash and success sentinel", exitCode: crashExit, log: "Installation process succeeded", writeUninstaller: true, wantSuccess: true},
		{name: "missing success sentinel", exitCode: crashExit, log: "Deinitializing Setup", writeUninstaller: true, wantCode: "installer_exit_nonzero"},
		{name: "failure sentinel wins", exitCode: crashExit, log: "Installation process succeeded\nRolling back changes", writeUninstaller: true, wantCode: "installer_exit_nonzero"},
		{name: "different exit remains failure", exitCode: crashExit + 1, log: "Installation process succeeded", writeUninstaller: true, wantCode: "installer_exit_nonzero"},
		{name: "post validation still required", exitCode: crashExit, log: "Installation process succeeded", wantCode: "uninstaller_missing"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := &fakeInstallerProcessRunner{exitCode: test.exitCode}
			runner.onStart = func(spec InstallerProcessSpec) error {
				destination := fixedArgumentValue(spec.Arguments, "/DIR=")
				logPath := fixedArgumentValue(spec.Arguments, "/LOG=")
				if err := os.WriteFile(filepath.Join(destination, "Game.exe"), []byte("game"), 0o600); err != nil {
					return err
				}
				if test.writeUninstaller {
					if err := os.WriteFile(filepath.Join(destination, "unins000.exe"), []byte("uninstall"), 0o600); err != nil {
						return err
					}
				}
				return os.WriteFile(logPath, []byte(test.log), 0o600)
			}
			installer := newFakeGogInstaller(t, server.URL, fakeLocalConfirmer{}, runner)
			result, err := installer.Install(context.Background(), "command-crash", devicev1.GogInnoInstallRequest{
				GameID: "game", SourceGameID: "source", Title: "Game", DestinationRoot: root, DestinationName: "Game",
				Installer: devicev1.PackageTransferDescriptor{FileName: "setup_game.exe", Role: devicev1.PackageTransferRoleInstaller,
					SizeBytes: uint64(len(payload)), DownloadURL: server.URL, DownloadToken: "token"},
			}, nil)
			if test.wantSuccess {
				if err != nil {
					t.Fatalf("Install() error = %v", err)
				}
				if result.CompletionBasis != devicev1.GogInnoCompletionValidatedPostSuccessCrash || result.ExitCode == nil || *result.ExitCode != crashExit {
					t.Fatalf("result = %#v", result)
				}
				var manifest gogInnoManifest
				data, readErr := os.ReadFile(filepath.Join(result.InstallPath, installManifestName))
				if readErr != nil || json.Unmarshal(data, &manifest) != nil {
					t.Fatalf("read manifest: %v", readErr)
				}
				if manifest.CompletionBasis != devicev1.GogInnoCompletionValidatedPostSuccessCrash || manifest.ExitCode == nil || *manifest.ExitCode != crashExit {
					t.Fatalf("manifest = %#v", manifest)
				}
				return
			}
			assertGogErrorCode(t, err, test.wantCode)
			if result.CleanupMarkerID == "" {
				t.Fatalf("post-start failure has no cleanup marker: %#v", result)
			}
			markerPath, markerPathErr := failureMarkerSidecarPath(result.InstallRoot, result.CleanupMarkerID)
			if markerPathErr != nil {
				t.Fatal(markerPathErr)
			}
			if _, markerErr := os.Stat(markerPath); markerErr != nil {
				t.Fatalf("post-start failure marker missing: %v", markerErr)
			}
		})
	}
}

func TestManagedGogInnoInstallerPreStartFailureRemovesMarkerOnlyDestination(t *testing.T) {
	t.Parallel()
	payload := []byte("MZ Inno Setup")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	root := t.TempDir()
	installer := newFakeGogInstaller(t, server.URL, fakeLocalConfirmer{}, &fakeInstallerProcessRunner{startErr: ErrUACDeclined})
	_, err := installer.Install(context.Background(), "command-prestart", devicev1.GogInnoInstallRequest{
		GameID: "game", SourceGameID: "source", Title: "Game", DestinationRoot: root, DestinationName: "Game",
		Installer: devicev1.PackageTransferDescriptor{FileName: "setup_game.exe", Role: devicev1.PackageTransferRoleInstaller,
			SizeBytes: uint64(len(payload)), DownloadURL: server.URL, DownloadToken: "token"},
	}, nil)
	assertGogErrorCode(t, err, "uac_declined")
	if _, statErr := os.Stat(filepath.Join(root, "Game")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("pre-start marker-only destination remains: %v", statErr)
	}
}

func TestReadBoundedInnoLogTailUTF16LE(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "installer.log")
	text := "Installation process succeeded"
	data := []byte{0xff, 0xfe}
	for _, character := range text {
		data = append(data, byte(character), 0)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	decoded, err := readBoundedInnoLogTail(path)
	if err != nil || !strings.Contains(decoded, text) {
		t.Fatalf("decoded=%q error=%v", decoded, err)
	}
}

func TestReadBoundedInnoLogTailDoesNotAcceptSentinelOutsideFinalMiB(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "installer.log")
	data := append([]byte("Installation process succeeded\n"), []byte(strings.Repeat("x", int(maxGogInnoLogTailBytes)+1))...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	accepted, err := isValidatedPostSuccessCrash(int(uint32(0xC000041D)), path)
	if err != nil {
		t.Fatal(err)
	}
	if accepted {
		t.Fatal("classifier accepted a success sentinel outside the final 1 MiB")
	}
}

func TestManagedGogInnoInstallerRejectsOffOriginAndCompanionMismatch(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	installer := newFakeGogInstaller(t, server.URL, fakeLocalConfirmer{}, &fakeInstallerProcessRunner{})
	request := devicev1.GogInnoInstallRequest{
		GameID: "game", SourceGameID: "source", Title: "Game", DestinationRoot: t.TempDir(), DestinationName: "Game",
		Installer: devicev1.PackageTransferDescriptor{
			FileName: "setup_game.exe", Role: devicev1.PackageTransferRoleInstaller, SizeBytes: 1,
			DownloadURL: "https://example.invalid/setup.exe", DownloadToken: "token",
		},
	}
	_, err := installer.Install(context.Background(), "off-origin", request, nil)
	assertGogErrorCode(t, err, "invalid_companion_set")

	request.Installer.DownloadURL = "/setup.exe"
	request.Companions = []devicev1.PackageTransferDescriptor{{
		FileName: "setup_other-1.bin", Role: devicev1.PackageTransferRoleCompanion, SizeBytes: 1,
		DownloadURL: "/setup.bin", DownloadToken: "token",
	}}
	_, err = installer.Install(context.Background(), "bad-companion", request, nil)
	assertGogErrorCode(t, err, "invalid_companion_set")
}

func TestManagedGogInnoUninstallUsesManifestMembershipAndNeverDeletesLeftovers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	installPath := filepath.Join(root, "Game")
	if err := os.MkdirAll(installPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installPath, "unins000.exe"), []byte("uninstall"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := gogInnoManifest{
		SchemaVersion: 3, GameID: "game", SourceGameID: "source", InstallRoot: root, InstallPath: installPath,
		InstallerFamily: devicev1.GogInnoInstallerFamily, UninstallTarget: "unins000.exe",
	}
	if err := writeGogInnoManifest(installPath, manifest); err != nil {
		t.Fatal(err)
	}
	runner := &fakeInstallerProcessRunner{}
	installer := newFakeGogInstaller(t, "https://mga.example", fakeLocalConfirmer{}, runner)
	result, err := installer.Uninstall(context.Background(), devicev1.GogInnoUninstallRequest{
		GameID: "game", SourceGameID: "source", InstallPath: installPath,
		InstallerFamily: devicev1.GogInnoInstallerFamily, UninstallTarget: "unins000.exe",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Removed || !result.LeftoverDirectory {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(installPath, "unins000.exe")); err != nil {
		t.Fatalf("uninstaller/leftovers were recursively deleted: %v", err)
	}
	if len(runner.specs) != 1 {
		t.Fatalf("process starts = %d", len(runner.specs))
	}
	want := []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART"}
	if strings.Join(runner.specs[0].Arguments, "|") != strings.Join(want, "|") {
		t.Fatalf("uninstall arguments = %v", runner.specs[0].Arguments)
	}

	badRequest := devicev1.GogInnoUninstallRequest{
		GameID: "game", SourceGameID: "source", InstallPath: installPath,
		InstallerFamily: devicev1.GogInnoInstallerFamily, UninstallTarget: "subdir/unins001.exe",
	}
	_, err = installer.Uninstall(context.Background(), badRequest, nil)
	assertGogErrorCode(t, err, "uninstaller_mismatch")
}

func TestManagedGogInnoCleanupFailedUsesPublisherUninstallerThenBoundedDelete(t *testing.T) {
	t.Parallel()
	request := createFailedCleanupFixture(t, true)
	cleanupCalls := 0
	runner := &fakeInstallerProcessRunner{}
	runner.onStart = func(spec InstallerProcessSpec) error {
		if spec.Path != filepath.Join(request.InstallPath, "unins000.exe") {
			return errors.New("cleanup did not use the recorded publisher uninstaller")
		}
		markerPath, markerErr := failureMarkerSidecarPath(request.InstallRoot, request.CleanupMarkerID)
		if markerErr != nil {
			return markerErr
		}
		if _, err := os.Stat(markerPath); err != nil {
			return errors.New("cleanup deleted files before publisher uninstaller start")
		}
		return nil
	}
	installer := newFakeGogInstaller(t, "https://mga.example", fakeLocalConfirmer{cleanupCalls: &cleanupCalls}, runner)
	result, err := installer.CleanupFailed(context.Background(), request, nil)
	if err != nil {
		t.Fatalf("CleanupFailed() error = %v", err)
	}
	if cleanupCalls != 1 || !result.Removed || !result.PublisherUninstallerUsed || !result.BoundedDeleteUsed || !result.SystemChangesMayRemain {
		t.Fatalf("result=%#v cleanupCalls=%d", result, cleanupCalls)
	}
	if _, err := os.Stat(request.InstallPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("marked destination remains: %v", err)
	}
}

func TestManagedGogInnoCleanupFailedPreservesFilesWhenPublisherUninstallerFails(t *testing.T) {
	t.Parallel()
	request := createFailedCleanupFixture(t, true)
	installer := newFakeGogInstaller(t, "https://mga.example", fakeLocalConfirmer{}, &fakeInstallerProcessRunner{exitCode: 7})
	result, err := installer.CleanupFailed(context.Background(), request, nil)
	assertGogErrorCode(t, err, "cleanup_uninstaller_failed")
	if !result.PublisherUninstallerUsed || result.BoundedDeleteUsed {
		t.Fatalf("result = %#v", result)
	}
	for _, name := range []string{"payload.bin", "unins000.exe"} {
		if _, statErr := os.Stat(filepath.Join(request.InstallPath, name)); statErr != nil {
			t.Fatalf("%s was not preserved: %v", name, statErr)
		}
	}
	markerPath, markerErr := failureMarkerSidecarPath(request.InstallRoot, request.CleanupMarkerID)
	if markerErr != nil {
		t.Fatal(markerErr)
	}
	if _, statErr := os.Stat(markerPath); statErr != nil {
		t.Fatalf("sidecar marker was not preserved: %v", statErr)
	}
}

func TestManagedGogInnoCleanupFailedWithoutUninstallerDeletesOnlyMarkedDestination(t *testing.T) {
	t.Parallel()
	request := createFailedCleanupFixture(t, false)
	outside := filepath.Join(request.InstallRoot, "keep.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeInstallerProcessRunner{startErr: errors.New("runner must not be called")}
	installer := newFakeGogInstaller(t, "https://mga.example", fakeLocalConfirmer{}, runner)
	result, err := installer.CleanupFailed(context.Background(), request, nil)
	if err != nil {
		t.Fatalf("CleanupFailed() error = %v", err)
	}
	if result.PublisherUninstallerUsed || !result.BoundedDeleteUsed || len(runner.specs) != 0 {
		t.Fatalf("result=%#v runner=%#v", result, runner.specs)
	}
	if data, readErr := os.ReadFile(outside); readErr != nil || string(data) != "keep" {
		t.Fatalf("outside file changed: data=%q error=%v", data, readErr)
	}
}

func TestManagedGogInnoCleanupFailedPreservesRegisteredProgramFolder(t *testing.T) {
	t.Parallel()
	request := createFailedCleanupFixture(t, false)
	inspector := &fakeRegisteredProgramInspector{associated: true}
	installer := newFakeGogInstaller(t, "https://mga.example", fakeLocalConfirmer{}, &fakeInstallerProcessRunner{})
	installer.programs = inspector
	result, err := installer.CleanupFailed(context.Background(), request, nil)
	assertGogErrorCode(t, err, "cleanup_registered_program_present")
	if inspector.calls != 1 || result.BoundedDeleteUsed {
		t.Fatalf("inspector calls=%d result=%#v", inspector.calls, result)
	}
	if _, statErr := os.Stat(filepath.Join(request.InstallPath, "payload.bin")); statErr != nil {
		t.Fatalf("registered program folder was modified: %v", statErr)
	}
}

func TestManagedGogInnoCleanupFailedRejectsMissingMismatchedAndRootMarkers(t *testing.T) {
	t.Parallel()
	valid := createFailedCleanupFixture(t, false)
	installer := newFakeGogInstaller(t, "https://mga.example", fakeLocalConfirmer{}, &fakeInstallerProcessRunner{})

	missing := valid
	missing.InstallPath = filepath.Join(valid.InstallRoot, "Legacy")
	if err := os.Mkdir(missing.InstallPath, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := installer.CleanupFailed(context.Background(), missing, nil)
	assertGogErrorCode(t, err, "cleanup_marker_mismatch")

	mismatch := valid
	mismatch.CleanupMarkerID = "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	_, err = installer.CleanupFailed(context.Background(), mismatch, nil)
	assertGogErrorCode(t, err, "cleanup_marker_missing")

	root := valid
	root.InstallPath = valid.InstallRoot
	_, err = installer.CleanupFailed(context.Background(), root, nil)
	assertGogErrorCode(t, err, "cleanup_boundary_failed")

	if _, statErr := os.Stat(filepath.Join(valid.InstallPath, "payload.bin")); statErr != nil {
		t.Fatalf("rejected cleanup modified the marked folder: %v", statErr)
	}
}

func TestManagedGogInnoCleanupFailedRejectsReplacedSchema2Destination(t *testing.T) {
	t.Parallel()
	request := createFailedCleanupFixture(t, false)
	if err := os.RemoveAll(request.InstallPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(request.InstallPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(request.InstallPath, "replacement.bin"), []byte("do not delete"), 0o600); err != nil {
		t.Fatal(err)
	}
	installer := newFakeGogInstaller(t, "https://mga.example", fakeLocalConfirmer{}, &fakeInstallerProcessRunner{})
	_, err := installer.CleanupFailed(context.Background(), request, nil)
	assertGogErrorCode(t, err, "cleanup_marker_mismatch")
	if _, statErr := os.Stat(filepath.Join(request.InstallPath, "replacement.bin")); statErr != nil {
		t.Fatalf("replaced destination was modified: %v", statErr)
	}
}

func TestManagedGogInnoCleanupDeclinePreservesMarkedFolder(t *testing.T) {
	t.Parallel()
	request := createFailedCleanupFixture(t, false)
	installer := newFakeGogInstaller(t, "https://mga.example", fakeLocalConfirmer{cleanupErr: ErrLocalConfirmationDeclined}, &fakeInstallerProcessRunner{})
	_, err := installer.CleanupFailed(context.Background(), request, nil)
	assertGogErrorCode(t, err, "local_confirmation_declined")
	if _, statErr := os.Stat(filepath.Join(request.InstallPath, "payload.bin")); statErr != nil {
		t.Fatalf("declined cleanup modified files: %v", statErr)
	}
}

func TestManagedGogInnoCleanupDoesNotFollowChildReparsePoint(t *testing.T) {
	t.Parallel()
	request := createFailedCleanupFixture(t, false)
	outsideRoot := t.TempDir()
	outside := filepath.Join(outsideRoot, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(request.InstallPath, "outside-link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	installer := newFakeGogInstaller(t, "https://mga.example", fakeLocalConfirmer{}, &fakeInstallerProcessRunner{})
	if _, err := installer.CleanupFailed(context.Background(), request, nil); err != nil {
		t.Fatalf("CleanupFailed() error = %v", err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "outside" {
		t.Fatalf("cleanup followed child link: data=%q error=%v", data, err)
	}
}

func createFailedCleanupFixture(t *testing.T, withUninstaller bool) devicev1.GogInnoFailedCleanupRequest {
	t.Helper()
	root := t.TempDir()
	installPath := filepath.Join(root, "Failed Game")
	primaryHash := strings.Repeat("a", 64)
	marker, err := newGogInnoFailureMarker("command-cleanup", devicev1.GogInnoInstallRequest{
		GameID: "game", SourceGameID: "source",
	}, root, installPath, primaryHash, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	markerRecord, err := createGogInnoFailureMarker(root, installPath, marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installPath, "payload.bin"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	uninstaller := ""
	if withUninstaller {
		uninstaller = "unins000.exe"
		if err := os.WriteFile(filepath.Join(installPath, uninstaller), []byte("uninstaller"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return devicev1.GogInnoFailedCleanupRequest{
		GameID: "game", SourceGameID: "source", InstallRoot: root, InstallPath: installPath,
		InstallerFamily: devicev1.GogInnoInstallerFamily, CleanupMarkerID: markerRecord.Marker.MarkerID,
		PrimarySHA256: primaryHash, UninstallTarget: uninstaller,
	}
}

func newFakeGogInstaller(t *testing.T, serverURL string, confirmer LocalConfirmer, runner InstallerProcessRunner) *ManagedGogInnoInstaller {
	t.Helper()
	installer, err := NewManagedGogInnoInstaller(serverURL, validFakeVerifier(), fakeInnoDetector{inno: true}, confirmer, runner)
	if err != nil {
		t.Fatal(err)
	}
	return installer
}

func validFakeVerifier() fakeAuthenticodeVerifier {
	return fakeAuthenticodeVerifier{subject: "CN=GOG Sp. z o.o.", thumbprint: "AABBCC"}
}

func fixedArgumentValue(arguments []string, prefix string) string {
	for _, argument := range arguments {
		if strings.HasPrefix(argument, prefix) {
			return strings.Trim(strings.TrimPrefix(argument, prefix), `"`)
		}
	}
	return ""
}

func TestInnoPathArgumentLeavesWindowsQuotingToProcessRunner(t *testing.T) {
	t.Parallel()
	got := innoPathArgument("/DIR=", `C:\Games\MGA E2E Game`)
	want := `/DIR=C:\Games\MGA E2E Game`
	if got != want {
		t.Fatalf("innoPathArgument() = %q, want %q", got, want)
	}
	if fixedArgumentValue([]string{got}, "/DIR=") != `C:\Games\MGA E2E Game` {
		t.Fatalf("fixedArgumentValue() did not retain the path from %q", got)
	}
}

func containsExact(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func assertGogErrorCode(t *testing.T, err error, expected string) {
	t.Helper()
	var commandError *GogInnoCommandError
	if !errors.As(err, &commandError) {
		t.Fatalf("error = %v, want GogInnoCommandError", err)
	}
	if commandError.Code != expected {
		t.Fatalf("error code = %q, want %q", commandError.Code, expected)
	}
}
