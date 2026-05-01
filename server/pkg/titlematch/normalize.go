package titlematch

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	// Strip trailing ROM/dump/set suffixes while preserving meaningful middle-title qualifiers.
	trailingBracketNoiseRE = regexp.MustCompile(`[\s._-]*[\(\[][^\)\]]*[\)\]]\s*$`)
	setupPrefixRE          = regexp.MustCompile(`(?i)^setup[\s._-]+`)
	versionSuffixRE        = regexp.MustCompile(`(?i)[\s._-]+(?:(?:v|version[\s._-]*)\d+(?:\.\d+)+(?:[\s._-]+[a-z]{2,8}\d*)*|\d+(?:\.\d+)+[\s._-]+[a-z]{2,8}\d*(?:[\s._-]+[a-z]{2,8}\d*)*)\s*$`)
	nonAlphaNumRE          = regexp.MustCompile(`[^a-z0-9\s]+`)
	multiSpaceRE           = regexp.MustCompile(`\s+`)
)

func NormalizeLookupTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = foldLatinDiacritics(value)
	value = stripTrailingLookupNoise(value)
	value = setupPrefixRE.ReplaceAllString(value, "")
	value = nonAlphaNumRE.ReplaceAllString(value, " ")
	value = multiSpaceRE.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func foldLatinDiacritics(value string) string {
	folded, _, err := transform.String(transform.Chain(norm.NFD, transform.RemoveFunc(func(r rune) bool {
		return unicode.Is(unicode.Mn, r)
	}), norm.NFC), value)
	if err != nil {
		return value
	}
	return folded
}

// LookupTitleVariants returns an ordered, deduped set of lookup titles from
// most literal to most canonical.
func LookupTitleVariants(value string) []string {
	var variants []string
	seen := map[string]bool{}
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			return
		}
		seen[candidate] = true
		variants = append(variants, candidate)
	}
	add(value)
	add(NormalizeLookupTitle(value))
	return variants
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
