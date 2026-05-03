package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/getlantern/systray"
)

type trayApp struct {
	baseURL     string
	serverExe   string
	configPath  string
	appDir      string
	dataDir     string
	runtimeMode string
	mode        string
	serviceName string
	childMu     sync.Mutex
	child       *exec.Cmd
	client      *http.Client
}

func main() {
	app := &trayApp{client: &http.Client{Timeout: 2 * time.Second}}
	flag.StringVar(&app.baseURL, "base-url", "http://127.0.0.1:8900", "MGA server URL.")
	flag.StringVar(&app.serverExe, "server-exe", defaultServerExe(), "Path to mga_server.exe.")
	flag.StringVar(&app.configPath, "config", "", "Path to config.json.")
	flag.StringVar(&app.appDir, "app-dir", "", "Immutable app directory.")
	flag.StringVar(&app.dataDir, "data-dir", "", "Mutable data directory.")
	flag.StringVar(&app.runtimeMode, "runtime-mode", "", "Runtime mode passed to the server.")
	flag.StringVar(&app.mode, "mode", "process", "Runtime controller mode: process or service.")
	flag.StringVar(&app.serviceName, "service-name", "MyGamesAnywhere", "Windows service name.")
	flag.Parse()
	systray.Run(app.onReady, func() {})
}

func (a *trayApp) onReady() {
	if icon, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "mga.ico")); err == nil {
		systray.SetIcon(icon)
	}
	systray.SetTitle("MGA")
	systray.SetTooltip("MyGamesAnywhere")

	open := systray.AddMenuItem("Open MGA", "Open MyGamesAnywhere in the default browser")
	start := systray.AddMenuItem("Start MGA", "Start the MyGamesAnywhere server")
	restart := systray.AddMenuItem("Restart MGA", "Restart the MyGamesAnywhere server")
	stop := systray.AddMenuItem("Shutdown MGA", "Stop the MyGamesAnywhere server")
	systray.AddSeparator()
	exit := systray.AddMenuItem("Exit Tray", "Exit the tray companion")

	go a.statusLoop()
	go func() {
		for {
			select {
			case <-open.ClickedCh:
				_ = openBrowser(a.baseURL)
			case <-start.ClickedCh:
				_ = a.start()
			case <-restart.ClickedCh:
				_ = a.stop()
				_ = a.start()
			case <-stop.ClickedCh:
				_ = a.stop()
			case <-exit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func (a *trayApp) statusLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		a.updateStatus()
		<-ticker.C
	}
}

func (a *trayApp) updateStatus() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/health", nil)
	res, err := a.client.Do(req)
	if err == nil && res.Body != nil {
		_ = res.Body.Close()
	}
	if err == nil && res.StatusCode >= 200 && res.StatusCode < 300 {
		systray.SetTooltip("MyGamesAnywhere is running")
		return
	}
	systray.SetTooltip("MyGamesAnywhere is stopped")
}

func (a *trayApp) start() error {
	if a.mode == "service" {
		return runCommand("sc.exe", "start", a.serviceName)
	}
	a.childMu.Lock()
	defer a.childMu.Unlock()
	if a.child != nil && a.child.Process != nil {
		return nil
	}
	args := []string{"--no-tray"}
	if a.configPath != "" {
		args = append(args, "--config", a.configPath)
	}
	if a.appDir != "" {
		args = append(args, "--app-dir", a.appDir)
	}
	if a.dataDir != "" {
		args = append(args, "--data-dir", a.dataDir)
	}
	if a.runtimeMode != "" {
		args = append(args, "--runtime-mode", a.runtimeMode)
	}
	cmd := exec.Command(a.serverExe, args...)
	cmd.Dir = filepath.Dir(a.serverExe)
	if err := cmd.Start(); err != nil {
		return err
	}
	a.child = cmd
	go func() {
		_ = cmd.Wait()
		a.childMu.Lock()
		if a.child == cmd {
			a.child = nil
		}
		a.childMu.Unlock()
	}()
	return nil
}

func (a *trayApp) stop() error {
	if a.mode == "service" {
		return runCommand("sc.exe", "stop", a.serviceName)
	}
	a.childMu.Lock()
	defer a.childMu.Unlock()
	if a.child == nil || a.child.Process == nil {
		return nil
	}
	err := a.child.Process.Kill()
	a.child = nil
	return err
}

func openBrowser(url string) error {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", "start", "", url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %v failed: %w: %s", name, args, err, string(out))
	}
	return nil
}

func defaultServerExe() string {
	exe, err := os.Executable()
	if err != nil {
		return "mga_server.exe"
	}
	ext := ".exe"
	if runtime.GOOS != "windows" {
		ext = ""
	}
	return filepath.Join(filepath.Dir(exe), "mga_server"+ext)
}
