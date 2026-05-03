//go:build windows

package main

import (
	"context"
	_ "embed"
	"os/exec"

	"github.com/getlantern/systray"
)

//go:embed mga.ico
var mgaIcon []byte

// Run starts the system tray (Windows). When the user chooses Exit, cancel is called and the tray quits.
// Run blocks until the tray is closed; call it in a goroutine.
// baseURL is the HTTP root (e.g. http://127.0.0.1:8900) for "Open Web Frontend".
func runTray(cancel context.CancelFunc, baseURL string) {
	onReady := func() {
		systray.SetTitle("MGA Server")
		systray.SetTooltip("MyGamesAnywhere Server")
		systray.SetIcon(mgaIcon)
		openItem := systray.AddMenuItem("Open MGA", "Open MyGamesAnywhere in the default browser")
		systray.AddSeparator()
		exitItem := systray.AddMenuItem("Shutdown MGA", "Stop the server and close the tray")
		go func() {
			for range openItem.ClickedCh {
				_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", baseURL).Start()
			}
		}()
		go func() {
			<-exitItem.ClickedCh
			cancel()
			systray.Quit()
		}()
	}
	systray.Run(onReady, nil)
}
