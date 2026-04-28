package titlematch

import (
	"regexp"
	"strings"
)

var (
	// Strip trailing ROM/dump/set suffixes while preserving meaningful middle-title qualifiers.
	trailingBracketNoiseRE = regexp.MustCompile(`[\s._-]*[\(\[][^\)\]]*[\)\]]\s*$`)
	setupPrefixRE          = regexp.MustCompile(`(?i)^setup[\s._-]+`)
	versionSuffixRE        = regexp.MustCompile(`(?i)[\s._-]+(?:v|version[\s._-]*)\d+(?:\.\d+)+(?:[\s._-]+[a-z]{2,8}\d*)*\s*$`)
	nonAlphaNumRE          = regexp.MustCompile(`[^a-z0-9\s]+`)
	multiSpaceRE           = regexp.MustCompile(`\s+`)
)

func NormalizeLookupTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = stripTrailingLookupNoise(value)
	value = setupPrefixRE.ReplaceAllString(value, "")
	value = nonAlphaNumRE.ReplaceAllString(value, " ")
	value = multiSpaceRE.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

// CleanDisplayTitle removes source/version dump suffixes while preserving the
// user-facing casing and punctuation of the meaningful title.
func CleanDisplayTitle(value string) string {
	value = strings.TrimSpace(value)
	value = stripTrailingLookupNoise(value)
	value = setupPrefixRE.ReplaceAllString(value, "")
	value = multiSpaceRE.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func stripTrailingLookupNoise(value string) string {
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
