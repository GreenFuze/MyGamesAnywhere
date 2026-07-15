//go:build !windows

package clientapp

import devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"

func collectLocalStorage() ([]devicev1.StorageInventory, error) {
	return []devicev1.StorageInventory{}, nil
}
