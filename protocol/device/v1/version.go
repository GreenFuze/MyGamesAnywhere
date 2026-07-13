// Package v1 defines the first version of the MGA server-to-device protocol.
package v1

import (
	"errors"
	"fmt"
)

// ProtocolVersion identifies one wire-compatible protocol version.
type ProtocolVersion uint16

const (
	// Version is the protocol version implemented by this package.
	Version ProtocolVersion = 1
)

var ErrNoCompatibleProtocol = errors.New("no compatible protocol version")

// VersionRange is an inclusive range of supported protocol versions.
type VersionRange struct {
	Min ProtocolVersion `json:"min"`
	Max ProtocolVersion `json:"max"`
}

// SupportedVersionRange returns the versions implemented by this package.
func SupportedVersionRange() VersionRange {
	return VersionRange{Min: Version, Max: Version}
}

// Validate rejects empty or inverted ranges.
func (r VersionRange) Validate() error {
	if r.Min == 0 {
		return errors.New("minimum protocol version must be greater than zero")
	}
	if r.Max < r.Min {
		return fmt.Errorf("maximum protocol version %d is lower than minimum %d", r.Max, r.Min)
	}
	return nil
}

// NegotiateVersion selects the highest version supported by both peers.
func NegotiateVersion(left, right VersionRange) (ProtocolVersion, error) {
	if err := left.Validate(); err != nil {
		return 0, fmt.Errorf("validate left protocol range: %w", err)
	}
	if err := right.Validate(); err != nil {
		return 0, fmt.Errorf("validate right protocol range: %w", err)
	}

	minVersion := left.Min
	if right.Min > minVersion {
		minVersion = right.Min
	}
	maxVersion := left.Max
	if right.Max < maxVersion {
		maxVersion = right.Max
	}
	if maxVersion < minVersion {
		return 0, fmt.Errorf("%w: left=%d-%d right=%d-%d", ErrNoCompatibleProtocol, left.Min, left.Max, right.Min, right.Max)
	}
	return maxVersion, nil
}
