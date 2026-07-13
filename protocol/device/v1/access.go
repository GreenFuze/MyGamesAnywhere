package v1

import (
	"errors"
	"fmt"
)

// AccessLevel is a profile's authority over one device endpoint.
type AccessLevel string

const (
	AccessView   AccessLevel = "view"
	AccessPlay   AccessLevel = "play"
	AccessManage AccessLevel = "manage"
	AccessOwner  AccessLevel = "owner"
)

var accessRanks = map[AccessLevel]int{
	AccessView:   1,
	AccessPlay:   2,
	AccessManage: 3,
	AccessOwner:  4,
}

// Validate rejects unknown access levels.
func (l AccessLevel) Validate() error {
	if _, ok := accessRanks[l]; !ok {
		return fmt.Errorf("unknown access level %q", l)
	}
	return nil
}

// Allows reports whether the granted access level satisfies the required one.
// Invalid values are errors rather than silently denying or allowing access.
func (l AccessLevel) Allows(required AccessLevel) (bool, error) {
	if err := l.Validate(); err != nil {
		return false, fmt.Errorf("validate granted access: %w", err)
	}
	if err := required.Validate(); err != nil {
		return false, fmt.Errorf("validate required access: %w", err)
	}
	grantedRank, ok := accessRanks[l]
	if !ok {
		return false, errors.New("granted access rank is unavailable")
	}
	requiredRank, ok := accessRanks[required]
	if !ok {
		return false, errors.New("required access rank is unavailable")
	}
	return grantedRank >= requiredRank, nil
}
