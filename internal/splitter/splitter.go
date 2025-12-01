package splitter

import (
	"strings"
	"unicode/utf8"
)

// IRC control characters
const (
	ircBold          = '\x02'
	ircItalic        = '\x1D'
	ircUnderline     = '\x1F'
	ircStrikethrough = '\x1E'
	ircMonospace     = '\x11'
	ircColor         = '\x03'
	ircHexColor      = '\x04'
	ircReverse       = '\x16'
	ircReset         = '\x0F'
)

// formatState tracks active IRC formatting
type formatState struct {
	bold          bool
	italic        bool
	underline     bool
	strikethrough bool
	monospace     bool
	reverse       bool
	fgColor       string // Empty if no color set
	bgColor       string // Empty if no background
}

// Splitter handles splitting long messages into IRC-compliant chunks
type Splitter struct {
	maxLength int // Maximum message length in bytes
}

// New creates a new message splitter with the specified max length
func New(maxLength int) *Splitter {
	return &Splitter{
		maxLength: maxLength,
	}
}

// Split breaks a message into multiple parts if it exceeds the max length.
// It splits at word boundaries and never breaks words in the middle.
// It is IRC-format aware: formatting is carried over to subsequent parts.
// Returns a slice of message parts that are all within the length limit.
func (s *Splitter) Split(message string) []string {
	// If message fits within limit, return as-is
	if len(message) <= s.maxLength {
		return []string{message}
	}

	var parts []string
	remaining := message
	var activeFormat formatState

	for len(remaining) > 0 {
		// Calculate prefix needed to restore formatting
		prefix := activeFormat.toIRCCodes()
		availableLen := s.maxLength - len(prefix)

		// If remaining (with prefix) fits, add it and we're done
		if len(remaining) <= availableLen {
			if prefix != "" {
				parts = append(parts, prefix+remaining)
			} else {
				parts = append(parts, remaining)
			}
			break
		}

		// Find the split point at a word boundary, avoiding IRC code sequences
		splitPoint := s.findSplitPointIRC(remaining, availableLen)

		// Extract the part and trim trailing whitespace
		part := strings.TrimRight(remaining[:splitPoint], " \t\n\r")
		if part != "" {
			if prefix != "" {
				parts = append(parts, prefix+part)
			} else {
				parts = append(parts, part)
			}
		}

		// Update format state based on what we just output
		activeFormat = s.parseFormatState(remaining[:splitPoint], activeFormat)

		// Move to the next part, skipping leading whitespace
		remaining = strings.TrimLeft(remaining[splitPoint:], " \t\n\r")
	}

	return parts
}

// findSplitPointIRC finds the best position to split the message at or before maxLen.
// It is IRC-format aware and won't split in the middle of color code sequences.
// It tries to split at word boundaries (spaces, punctuation) and never breaks
// in the middle of a UTF-8 character or word.
func (s *Splitter) findSplitPointIRC(message string, maxLen int) int {
	// If the message is shorter than maxLen, return its length
	if len(message) <= maxLen {
		return len(message)
	}

	// Ensure we don't split in the middle of a UTF-8 character
	// Walk backwards from maxLen to find a valid UTF-8 boundary
	splitPoint := maxLen
	for splitPoint > 0 && !utf8.RuneStart(message[splitPoint]) {
		splitPoint--
	}

	// Check if we're in the middle of an IRC color code sequence
	// Color codes: \x03 followed by optional digits and comma
	splitPoint = s.adjustForColorCode(message, splitPoint)

	// Now find the last word boundary at or before splitPoint
	// Look for spaces, tabs, newlines, or common punctuation
	// But skip positions that are inside IRC control sequences
	lastSpace := -1
	for i := 0; i < splitPoint; i++ {
		c := message[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			// Make sure this space isn't right after a color code start
			if !s.isInsideColorCode(message, i) {
				lastSpace = i
			}
		}
	}

	// If we found a space, split there
	if lastSpace > 0 {
		return lastSpace
	}

	// No space found - look for punctuation as a fallback
	lastPunct := -1
	for i := 0; i < splitPoint; i++ {
		c := message[i]
		if c == ',' || c == '.' || c == ';' || c == ':' || c == '!' || c == '?' {
			// Don't split on comma if it's part of a color code
			if c == ',' && s.isColorCodeComma(message, i) {
				continue
			}
			lastPunct = i + 1 // Split after punctuation
		}
	}

	if lastPunct > 0 {
		return lastPunct
	}

	// No good split point found - split at the UTF-8 boundary
	// This should be rare and only happens with very long words
	return splitPoint
}

// adjustForColorCode moves the split point back if it's in the middle of a color code
func (s *Splitter) adjustForColorCode(message string, pos int) int {
	if pos <= 0 || pos >= len(message) {
		return pos
	}

	// Look backwards for a color code start (\x03)
	// Color codes can be: \x03, \x03N, \x03NN, \x03N,N, \x03N,NN, \x03NN,N, \x03NN,NN
	// Maximum length after \x03 is 5 characters (NN,NN)
	searchStart := pos - 6
	if searchStart < 0 {
		searchStart = 0
	}

	for i := pos - 1; i >= searchStart; i-- {
		if message[i] == ircColor {
			// Found a color code start, check if pos is inside it
			codeEnd := s.findColorCodeEnd(message, i)
			if pos <= codeEnd {
				// We're inside the color code, move split point before it
				return i
			}
			break
		}
		// If we hit a non-digit, non-comma character, we're not in a color code
		c := message[i]
		if c != ',' && (c < '0' || c > '9') && c != ircColor {
			break
		}
	}

	// Also check for hex color codes (\x04 followed by 6 hex digits)
	if pos >= 7 {
		for i := pos - 1; i >= pos-7 && i >= 0; i-- {
			if message[i] == ircHexColor {
				// Check if we're inside the 6 hex digits
				if pos <= i+7 {
					return i
				}
				break
			}
		}
	}

	return pos
}

// findColorCodeEnd returns the position after the last character of a color code
func (s *Splitter) findColorCodeEnd(message string, colorStart int) int {
	pos := colorStart + 1 // Skip the \x03

	// Read first color (0-2 digits)
	digits := 0
	for pos < len(message) && digits < 2 && message[pos] >= '0' && message[pos] <= '9' {
		pos++
		digits++
	}

	// Check for comma and second color
	if pos < len(message) && message[pos] == ',' {
		pos++ // Skip comma
		digits = 0
		for pos < len(message) && digits < 2 && message[pos] >= '0' && message[pos] <= '9' {
			pos++
			digits++
		}
	}

	return pos
}

// isInsideColorCode checks if position is inside a color code sequence
func (s *Splitter) isInsideColorCode(message string, pos int) bool {
	if pos <= 0 {
		return false
	}

	// Look backwards for \x03
	searchStart := pos - 6
	if searchStart < 0 {
		searchStart = 0
	}

	for i := pos - 1; i >= searchStart; i-- {
		if message[i] == ircColor {
			codeEnd := s.findColorCodeEnd(message, i)
			return pos < codeEnd
		}
		c := message[i]
		if c != ',' && (c < '0' || c > '9') {
			return false
		}
	}

	return false
}

// isColorCodeComma checks if the comma at pos is part of a color code
func (s *Splitter) isColorCodeComma(message string, pos int) bool {
	if pos <= 0 {
		return false
	}

	// A color code comma must be preceded by \x03 and 1-2 digits
	// Look backwards
	i := pos - 1
	digits := 0
	for i >= 0 && digits < 2 && message[i] >= '0' && message[i] <= '9' {
		i--
		digits++
	}

	if digits > 0 && i >= 0 && message[i] == ircColor {
		return true
	}

	return false
}

// parseFormatState parses a string and returns the active format state at the end
func (s *Splitter) parseFormatState(text string, initial formatState) formatState {
	state := initial

	for i := 0; i < len(text); i++ {
		c := text[i]
		switch c {
		case ircBold:
			state.bold = !state.bold
		case ircItalic:
			state.italic = !state.italic
		case ircUnderline:
			state.underline = !state.underline
		case ircStrikethrough:
			state.strikethrough = !state.strikethrough
		case ircMonospace:
			state.monospace = !state.monospace
		case ircReverse:
			state.reverse = !state.reverse
		case ircReset:
			state = formatState{} // Clear all
		case ircColor:
			// Parse color code
			i++ // Move past \x03
			fg, bg, newPos := s.parseColorCode(text, i)
			i = newPos - 1 // -1 because loop will increment

			if fg == "" && bg == "" {
				// Just \x03 alone resets colors
				state.fgColor = ""
				state.bgColor = ""
			} else {
				if fg != "" {
					state.fgColor = fg
				}
				if bg != "" {
					state.bgColor = bg
				}
			}
		case ircHexColor:
			// Skip hex color (6 hex digits)
			if i+6 < len(text) {
				i += 6
			}
		}
	}

	return state
}

// parseColorCode parses a color code starting at pos, returns fg, bg colors and new position
func (s *Splitter) parseColorCode(text string, pos int) (fg, bg string, newPos int) {
	// Read foreground color (1-2 digits)
	start := pos
	for pos < len(text) && pos-start < 2 && text[pos] >= '0' && text[pos] <= '9' {
		pos++
	}
	if pos > start {
		fg = text[start:pos]
	}

	// Check for background color
	if pos < len(text) && text[pos] == ',' {
		pos++ // Skip comma
		start = pos
		for pos < len(text) && pos-start < 2 && text[pos] >= '0' && text[pos] <= '9' {
			pos++
		}
		if pos > start {
			bg = text[start:pos]
		}
	}

	return fg, bg, pos
}

// toIRCCodes converts the format state back to IRC control codes
func (f *formatState) toIRCCodes() string {
	if !f.hasAnyFormatting() {
		return ""
	}

	var sb strings.Builder

	// Apply colors first
	if f.fgColor != "" {
		sb.WriteByte(byte(ircColor))
		sb.WriteString(f.fgColor)
		if f.bgColor != "" {
			sb.WriteByte(',')
			sb.WriteString(f.bgColor)
		}
	}

	// Apply toggle formatting
	if f.bold {
		sb.WriteByte(byte(ircBold))
	}
	if f.italic {
		sb.WriteByte(byte(ircItalic))
	}
	if f.underline {
		sb.WriteByte(byte(ircUnderline))
	}
	if f.strikethrough {
		sb.WriteByte(byte(ircStrikethrough))
	}
	if f.monospace {
		sb.WriteByte(byte(ircMonospace))
	}
	if f.reverse {
		sb.WriteByte(byte(ircReverse))
	}

	return sb.String()
}

// hasAnyFormatting returns true if any formatting is active
func (f *formatState) hasAnyFormatting() bool {
	return f.bold || f.italic || f.underline || f.strikethrough ||
		f.monospace || f.reverse || f.fgColor != "" || f.bgColor != ""
}

// findSplitPoint is kept for backwards compatibility but now calls findSplitPointIRC
func (s *Splitter) findSplitPoint(message string, maxLen int) int {
	return s.findSplitPointIRC(message, maxLen)
}

// NeedsSplit returns true if the message needs to be split
func (s *Splitter) NeedsSplit(message string) bool {
	return len(message) > s.maxLength
}

// MaxLength returns the configured maximum message length
func (s *Splitter) MaxLength() int {
	return s.maxLength
}
