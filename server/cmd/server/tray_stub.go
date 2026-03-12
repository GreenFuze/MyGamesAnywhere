//go:build !windows

package main

import "context"

// runTray is a no-op on non-Windows; the tray is only shown on Windows.
func runTray(cancel context.CancelFunc) {}
