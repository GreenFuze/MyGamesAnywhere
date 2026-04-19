package titlematch

import (
	"regexp"
	"strings"
)

var (
	trailingBracketNoiseRE = regexp.MustCompile(`[\s._-]*[\(\[][^\)\]]*[\)\]]\s*$`)
	setupPrefixRE          = regexp.MustCompile(`^setup[\s._-]+`)
	versionSuffixRE        = regexp.MustCompile(`[\s._-]+v?\d+(?:\.\d+)+(?:[\s._-]+[a-z]{2,8}\d*)*\s*$`)
	nonAlphaNumRE          = regexp.MustCompile(`[^a-z0-9\s]+`)
	multiSpaceRE           = regexp.MustCompile(`\s+`)
)

func NormalizeLookupTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for {
		changed := false
		if trailingBracketNoiseRE.MatchString(value) {
			value = trailingBracketNoiseRE.ReplaceAllString(value, "")
			changed = true
		}
		next := versionSuffixRE.ReplaceAllString(value, "")
		if next != value {
			value = next
			changed = true
		}
		if !changed {
			break
		}
	}
	value = setupPrefixRE.ReplaceAllString(value, "")
	value = nonAlphaNumRE.ReplaceAllString(value, " ")
	value = multiSpaceRE.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func TokenizeLookupTitle(value string) map[string]bool {
	words := strings.Fields(NormalizeLookupTitle(value))
	tokens := make(map[string]bool, len(words))
	for _, word := range words {
		tokens[word] = true
	}
	return tokens
}
