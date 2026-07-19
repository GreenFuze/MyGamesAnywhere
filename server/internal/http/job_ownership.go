package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrProfileJobBusy deliberately reveals no foreign job identifier or
	// progress when MGA's globally serialized worker is occupied by another
	// profile.
	ErrProfileJobBusy  = errors.New("the operation is busy for another profile")
	ErrProfileRequired = errors.New("profile is required")
)

func marshalProfileOwnedJobEvent(payload any, profileID string) ([]byte, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return nil, ErrProfileRequired
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("profile-owned event must be a JSON object: %w", err)
	}
	object["profile_id"] = profileID
	return json.Marshal(object)
}
