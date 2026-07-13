package v1

import (
	"errors"
	"testing"
)

func TestNegotiateVersionSelectsHighestOverlap(t *testing.T) {
	t.Parallel()

	got, err := NegotiateVersion(
		VersionRange{Min: 1, Max: 3},
		VersionRange{Min: 2, Max: 4},
	)
	if err != nil {
		t.Fatalf("NegotiateVersion() error = %v", err)
	}
	if got != 3 {
		t.Fatalf("NegotiateVersion() = %d, want 3", got)
	}
}

func TestNegotiateVersionRejectsNoOverlap(t *testing.T) {
	t.Parallel()

	_, err := NegotiateVersion(
		VersionRange{Min: 1, Max: 1},
		VersionRange{Min: 2, Max: 2},
	)
	if !errors.Is(err, ErrNoCompatibleProtocol) {
		t.Fatalf("NegotiateVersion() error = %v, want ErrNoCompatibleProtocol", err)
	}
}

func TestVersionRangeRejectsInvertedRange(t *testing.T) {
	t.Parallel()

	if err := (VersionRange{Min: 2, Max: 1}).Validate(); err == nil {
		t.Fatal("VersionRange.Validate() error = nil, want error")
	}
}

func TestVersionRangeRejectsZeroMinimum(t *testing.T) {
	t.Parallel()

	if err := (VersionRange{Min: 0, Max: 1}).Validate(); err == nil {
		t.Fatal("VersionRange.Validate() error = nil, want error")
	}
}

func TestNegotiateVersionRejectsInvalidPeerRange(t *testing.T) {
	t.Parallel()

	if _, err := NegotiateVersion(VersionRange{}, SupportedVersionRange()); err == nil {
		t.Fatal("NegotiateVersion() with invalid left range error = nil, want error")
	}
	if _, err := NegotiateVersion(SupportedVersionRange(), VersionRange{}); err == nil {
		t.Fatal("NegotiateVersion() with invalid right range error = nil, want error")
	}
}
