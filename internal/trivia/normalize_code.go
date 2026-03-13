package trivia

import "strings"

var codeTightPunctuation = map[rune]struct{}{
	'(': {},
	')': {},
	'[': {},
	']': {},
	'{': {},
	'}': {},
	',': {},
}

// NormalizeCodeAnswer trims and normalizes one-line code conservatively.
// Returns empty string if input is empty or contains newlines.
func NormalizeCodeAnswer(input string) string {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return ""
	}
	if strings.ContainsAny(raw, "\n\r") {
		return ""
	}

	var b strings.Builder
	b.Grow(len(raw))

	inSingle := false
	inDouble := false
	escaping := false
	pendingSpace := false

	for _, r := range raw {
		if inSingle {
			b.WriteRune(r)
			if escaping {
				escaping = false
				continue
			}
			if r == '\\' {
				escaping = true
				continue
			}
			if r == '\'' {
				inSingle = false
			}
			continue
		}

		if inDouble {
			b.WriteRune(r)
			if escaping {
				escaping = false
				continue
			}
			if r == '\\' {
				escaping = true
				continue
			}
			if r == '"' {
				inDouble = false
			}
			continue
		}

		if r == '\'' {
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
			inSingle = true
			b.WriteRune(r)
			continue
		}
		if r == '"' {
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
			inDouble = true
			b.WriteRune(r)
			continue
		}

		if isCodeWhitespace(r) {
			pendingSpace = true
			continue
		}

		if _, tight := codeTightPunctuation[r]; tight {
			trimTrailingSpace(&b)
			b.WriteRune(r)
			pendingSpace = false
			continue
		}

		if pendingSpace && b.Len() > 0 {
			b.WriteByte(' ')
		}
		pendingSpace = false
		b.WriteRune(r)
	}

	return strings.TrimSpace(b.String())
}

// CodeAnswerVariants returns deterministic exact-match candidates for one-line code.
func CodeAnswerVariants(input string) []string {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return nil
	}
	if strings.ContainsAny(raw, "\n\r") {
		return nil
	}

	normalized := NormalizeCodeAnswer(raw)
	if normalized == "" {
		return nil
	}

	if raw == normalized {
		return []string{raw}
	}
	return []string{raw, normalized}
}

func isCodeWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\v' || r == '\f'
}

func trimTrailingSpace(b *strings.Builder) {
	s := b.String()
	if s == "" || s[len(s)-1] != ' ' {
		return
	}
	b.Reset()
	b.WriteString(s[:len(s)-1])
}
