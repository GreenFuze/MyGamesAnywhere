//go:build windows

package desktop

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os/exec"
	"sync"

	"github.com/getlantern/systray"
)

// Run hosts the agent behind a per-user Windows notification-area icon. It
// blocks until the agent stops or Exit is selected.
func (h *Host) Run(ctx context.Context, runner AgentRunner) error {
	if h == nil {
		return errors.New("desktop host is required")
	}
	if runner == nil {
		return errors.New("desktop agent runner is required")
	}

	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	var startOnce sync.Once

	onReady := func() {
		systray.SetIcon(createTrayIcon())
		systray.SetTooltip(fmt.Sprintf("MGA Client — %s", h.options.DisplayName))
		versionItem := systray.AddMenuItem("MGA Client "+h.options.Version, h.options.DisplayName)
		versionItem.Disable()
		systray.AddSeparator()
		showLogs := systray.AddMenuItem("Show logs", "Open the MGA Client log")
		exit := systray.AddMenuItem("Exit", "Stop this user's MGA Client")

		go func() {
			for {
				select {
				case <-runContext.Done():
					return
				case <-showLogs.ClickedCh:
					_ = exec.Command("notepad.exe", h.options.LogPath).Start()
				case <-exit.ClickedCh:
					cancel()
					return
				}
			}
		}()

		startOnce.Do(func() {
			go func() {
				result <- runner(runContext)
				systray.Quit()
			}()
		})
	}

	systray.Run(onReady, cancel)
	select {
	case err := <-result:
		return err
	default:
		return runContext.Err()
	}
}

func createTrayIcon() []byte {
	const size = 32
	canvas := image.NewRGBA(image.Rect(0, 0, size, size))
	background := color.RGBA{R: 20, G: 29, B: 46, A: 255}
	accent := color.RGBA{R: 55, G: 189, B: 248, A: 255}
	white := color.RGBA{R: 245, G: 248, B: 255, A: 255}

	for y := 1; y < size-1; y++ {
		for x := 1; x < size-1; x++ {
			dx, dy := x-size/2, y-size/2
			if dx*dx+dy*dy <= 14*14 {
				canvas.Set(x, y, background)
			}
		}
	}
	for y := 8; y <= 23; y++ {
		for x := 8; x <= 11; x++ {
			canvas.Set(x, y, accent)
		}
		for x := 20; x <= 23; x++ {
			canvas.Set(x, y, accent)
		}
	}
	for offset := 0; offset < 7; offset++ {
		canvas.Set(12+offset, 9+offset, white)
		canvas.Set(19-offset, 9+offset, white)
	}

	var pngData bytes.Buffer
	if err := png.Encode(&pngData, canvas); err != nil {
		panic(fmt.Sprintf("encode MGA Client tray icon: %v", err))
	}

	var icon bytes.Buffer
	_ = binary.Write(&icon, binary.LittleEndian, uint16(0))
	_ = binary.Write(&icon, binary.LittleEndian, uint16(1))
	_ = binary.Write(&icon, binary.LittleEndian, uint16(1))
	icon.WriteByte(size)
	icon.WriteByte(size)
	icon.WriteByte(0)
	icon.WriteByte(0)
	_ = binary.Write(&icon, binary.LittleEndian, uint16(1))
	_ = binary.Write(&icon, binary.LittleEndian, uint16(32))
	_ = binary.Write(&icon, binary.LittleEndian, uint32(pngData.Len()))
	_ = binary.Write(&icon, binary.LittleEndian, uint32(22))
	icon.Write(pngData.Bytes())
	return icon.Bytes()
}
