//go:build !windows

package clientapp

import (
	"context"
	"errors"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
)

type unsupportedStorefrontPlatform struct{}

func newStorefrontProductObserver() StorefrontProductObserver { return unsupportedStorefrontPlatform{} }
func newStorefrontRouteLauncher() StorefrontRouteLauncher     { return unsupportedStorefrontPlatform{} }

func (unsupportedStorefrontPlatform) Observe(context.Context, []devicev1.StorefrontProductCandidate) ([]devicev1.StorefrontProductObservation, error) {
	return nil, errors.New("storefront observation is unavailable on this platform")
}

func (unsupportedStorefrontPlatform) Launch(context.Context, string, string) error {
	return errors.New("storefront launch is unavailable on this platform")
}
