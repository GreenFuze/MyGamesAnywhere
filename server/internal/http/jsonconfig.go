package http

import (
	"encoding/json"
	"reflect"
	"strings"
)

// configJSONObjectDeepEqual reports whether two JSON strings decode to the same JSON object
// (deep-equal maps). Used for duplicate integration detection.
func configJSONObjectDeepEqual(jsonA, jsonB string) bool {
	a := strings.TrimSpace(jsonA)
	b := strings.TrimSpace(jsonB)
	if a == "" {
		a = "{}"
	}
	if b == "" {
		b = "{}"
	}
	var ma, mb map[string]any
	if err := json.Unmarshal([]byte(a), &ma); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &mb); err != nil {
		return false
	}
	if ma == nil {
		ma = map[string]any{}
	}
	if mb == nil {
		mb = map[string]any{}
	}
	return reflect.DeepEqual(ma, mb)
}
