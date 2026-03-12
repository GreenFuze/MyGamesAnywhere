//go:build windows

package main

import (
	"context"
	_ "embed"

	"github.com/getlantern/systray"
)

//go:embed mga.ico
var mgaIcon []byte

// Run starts the system tray (Windows). When the user chooses Exit, cancel is called and the tray quits.
// Run blocks until the tray is closed; call it in a goroutine.
func runTray(cancel context.CancelFunc) {
	onReady := func() {
		systray.SetTitle("MGA Server")
		systray.SetTooltip("MyGamesAnywhere Server")
		systray.SetIcon(mgaIcon)
		exitItem := systray.AddMenuItem("Exit", "Close the application")
		go func() {
			<-exitItem.ClickedCh
			cancel()
			systray.Quit()
		}()
	}
	systray.Run(onReady, nil)
}
