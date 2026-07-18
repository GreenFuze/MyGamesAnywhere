//go:build !windows

package clientapp

func validateInstallRootStorage(_ string) error { return nil }
