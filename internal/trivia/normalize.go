package trivia

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

var fillerWords = map[string]struct{}{
	"a":   {},
	"an":  {},
	"and": {},
	"at":  {},
	"for": {},
	"in":  {},
	"of":  {},
	"on":  {},
	"the": {},
	"to":  {},
}

// NormalizeAnswer normalizes free text for strict answer matching.
func NormalizeAnswer(input string) string {
	return normalizeBase(input)
}

// NormalizeDedupKey normalizes uniqueness keys for dedup checks.
func NormalizeDedupKey(input string) string {
	base := normalizeBase(input)
	if base == "" {
		return ""
	}

	words := strings.Fields(base)
	filtered := make([]string, 0, len(words))
	for _, word := range words {
		if _, isFiller := fillerWords[word]; isFiller {
			continue
		}
		filtered = append(filtered, word)
	}

	if len(filtered) == 0 {
		return base
	}

	return strings.Join(filtered, " ")
}

// HashNormalized returns the SHA-256 hex digest of a normalized value.
func HashNormalized(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func normalizeBase(input string) string {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	if trimmed == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(trimmed))
	lastWasSpace := false

	for _, r := range trimmed {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r):
			b.WriteRune(r)
			lastWasSpace = false
		case unicode.IsSpace(r), unicode.IsPunct(r), unicode.IsSymbol(r):
			if !lastWasSpace {
				b.WriteRune(' ')
				lastWasSpace = true
			}
		}
	}

	return strings.Join(strings.Fields(b.String()), " ")
}
