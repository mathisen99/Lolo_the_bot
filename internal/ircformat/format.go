// Package ircformat provides conversion from custom XML-like tags to IRC formatting codes.
//
// IRC formatting uses control characters (0x02 for bold, 0x03 for color, etc.)
// which are difficult for AI models to output directly. This package allows
// AI to use readable tags like <BOLD>text</BOLD> which are then converted
// to proper IRC control codes before sending to the channel.
//
// Supported tags:
//   - <BOLD>text</BOLD> or <B>text</B>
//   - <ITALIC>text</ITALIC> or <I>text</I>
//   - <UNDERLINE>text</UNDERLINE> or <U>text</U>
//   - <STRIKE>text</STRIKE> or <S>text</S>
//   - <MONO>text</MONO> or <M>text</M>
//   - <COLOR fg="03">text</COLOR> or <COLOR fg="04" bg="01">text</COLOR>
//   - <REVERSE>text</REVERSE>
//   - <RESET/> (self-closing, clears all formatting)
//
// Tags can be nested: <BOLD><COLOR fg="03">bold green</COLOR></BOLD>
package ircformat

import (
	"regexp"
	"strings"
)

// IRC control characters
const (
	Bold          = "\x02" // 0x02 - Toggle bold
	Italic        = "\x1D" // 0x1D - Toggle italic
	Underline     = "\x1F" // 0x1F - Toggle underline
	Strikethrough = "\x1E" // 0x1E - Toggle strikethrough
	Monospace     = "\x11" // 0x11 - Toggle monospace
	Color         = "\x03" // 0x03 - Color code prefix
	HexColor      = "\x04" // 0x04 - Hex color prefix
	Reverse       = "\x16" // 0x16 - Toggle reverse colors
	Reset         = "\x0F" // 0x0F - Reset all formatting
)

// Standard IRC color codes (00-15)
const (
	ColorWhite      = "00"
	ColorBlack      = "01"
	ColorBlue       = "02"
	ColorGreen      = "03"
	ColorRed        = "04"
	ColorBrown      = "05"
	ColorMagenta    = "06"
	ColorOrange     = "07"
	ColorYellow     = "08"
	ColorLightGreen = "09"
	ColorCyan       = "10"
	ColorLightCyan  = "11"
	ColorLightBlue  = "12"
	ColorPink       = "13"
	ColorGrey       = "14"
	ColorLightGrey  = "15"
	ColorDefault    = "99"
)

// Formatter converts custom tags to IRC formatting codes
type Formatter struct {
	// tagPatterns maps tag names to their IRC control codes
	tagPatterns map[string]string
}

// NewFormatter creates a new IRC formatter
func NewFormatter() *Formatter {
	return &Formatter{
		tagPatterns: map[string]string{
			"BOLD":      Bold,
			"B":         Bold,
			"ITALIC":    Italic,
			"I":         Italic,
			"UNDERLINE": Underline,
			"U":         Underline,
			"REVERSE":   Reverse,
		},
	}
}

// Format converts all custom tags in the input to IRC formatting codes
func (f *Formatter) Format(input string) string {
	if input == "" {
		return input
	}

	result := input

	// Handle self-closing reset tag first
	result = f.handleResetTag(result)

	// Handle special tags (MONO for compatibility, STRIKE for cleanup)
	result = f.handleSpecialTags(result)

	// Handle color tags (most complex, do first)
	result = f.handleColorTags(result)

	// Handle simple toggle tags
	result = f.handleSimpleTags(result)

	return result
}

// handleSpecialTags handles tags that need specific compatibility logic
// <MONO> -> Color Grey (standard IRC code) instead of Monospace (extended)
// <STRIKE> -> Strip tags (avoid weird symbols on old clients)
func (f *Formatter) handleSpecialTags(input string) string {
	result := input

	// Handle MONO/M -> Color Grey (14)
	// We use color code 14 (Grey) to simulate code block appearance
	// This is compatible with all clients, unlike \x11 (Monospace)
	monoStart := Color + "14"
	monoEnd := Color // Reset color

	// Replace <MONO> tags
	result = strings.ReplaceAll(result, "<MONO>", monoStart)
	result = strings.ReplaceAll(result, "<mono>", monoStart) // basic case insensitivity
	result = strings.ReplaceAll(result, "</MONO>", monoEnd)
	result = strings.ReplaceAll(result, "</mono>", monoEnd)

	// Replace <M> tags
	result = strings.ReplaceAll(result, "<M>", monoStart)
	result = strings.ReplaceAll(result, "<m>", monoStart)
	result = strings.ReplaceAll(result, "</M>", monoEnd)
	result = strings.ReplaceAll(result, "</m>", monoEnd)

	// Handle STRIKE/S -> Strip tags
	// Strikethrough (\x1E) causes squares/symbols in older clients
	result = strings.ReplaceAll(result, "<STRIKE>", "")
	result = strings.ReplaceAll(result, "<strike>", "")
	result = strings.ReplaceAll(result, "</STRIKE>", "")
	result = strings.ReplaceAll(result, "</strike>", "")

	result = strings.ReplaceAll(result, "<S>", "")
	result = strings.ReplaceAll(result, "<s>", "")
	result = strings.ReplaceAll(result, "</S>", "")
	result = strings.ReplaceAll(result, "</s>", "")

	return result
}

// handleResetTag converts <RESET/> to the reset control character
func (f *Formatter) handleResetTag(input string) string {
	// Match <RESET/> or <RESET /> (case insensitive)
	resetPattern := regexp.MustCompile(`(?i)<RESET\s*/>`)
	return resetPattern.ReplaceAllString(input, Reset)
}

// handleColorTags converts <COLOR fg="XX">text</COLOR> and <COLOR fg="XX" bg="YY">text</COLOR>
func (f *Formatter) handleColorTags(input string) string {
	// Pattern for color with foreground only: <COLOR fg="03">text</COLOR>
	// Pattern for color with fg and bg: <COLOR fg="04" bg="01">text</COLOR>
	// Also support single quotes and no quotes for flexibility

	// Full color tag pattern (opening tag with attributes)
	colorOpenPattern := regexp.MustCompile(`(?i)<COLOR\s+fg\s*=\s*["']?(\d{1,2})["']?(?:\s+bg\s*=\s*["']?(\d{1,2})["']?)?\s*>`)
	colorClosePattern := regexp.MustCompile(`(?i)</COLOR>`)

	result := input

	// Find all opening color tags and replace them
	result = colorOpenPattern.ReplaceAllStringFunc(result, func(match string) string {
		submatches := colorOpenPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match // No match, return original
		}

		fg := submatches[1]
		// Ensure 2-digit format for foreground
		if len(fg) == 1 {
			fg = "0" + fg
		}

		// Check if background color is specified
		if len(submatches) >= 3 && submatches[2] != "" {
			bg := submatches[2]
			if len(bg) == 1 {
				bg = "0" + bg
			}
			return Color + fg + "," + bg
		}

		return Color + fg
	})

	// Replace closing color tags with color reset (just the color code without numbers)
	result = colorClosePattern.ReplaceAllString(result, Color)

	return result
}

// handleSimpleTags converts simple toggle tags like <BOLD>text</BOLD>
func (f *Formatter) handleSimpleTags(input string) string {
	result := input

	for tagName, controlChar := range f.tagPatterns {
		// Opening tag pattern (case insensitive)
		openPattern := regexp.MustCompile(`(?i)<` + tagName + `>`)
		// Closing tag pattern (case insensitive)
		closePattern := regexp.MustCompile(`(?i)</` + tagName + `>`)

		// Replace opening tags with control character
		result = openPattern.ReplaceAllString(result, controlChar)
		// Replace closing tags with control character (toggle off)
		result = closePattern.ReplaceAllString(result, controlChar)
	}

	return result
}

// StripTags removes all custom formatting tags without converting them
// Useful for logging or when formatting is not supported
func (f *Formatter) StripTags(input string) string {
	if input == "" {
		return input
	}

	result := input

	// Remove reset tags
	resetPattern := regexp.MustCompile(`(?i)<RESET\s*/>`)
	result = resetPattern.ReplaceAllString(result, "")

	// Remove color tags
	colorOpenPattern := regexp.MustCompile(`(?i)<COLOR\s+[^>]*>`)
	colorClosePattern := regexp.MustCompile(`(?i)</COLOR>`)
	result = colorOpenPattern.ReplaceAllString(result, "")
	result = colorClosePattern.ReplaceAllString(result, "")

	// Remove simple tags
	for tagName := range f.tagPatterns {
		openPattern := regexp.MustCompile(`(?i)<` + tagName + `>`)
		closePattern := regexp.MustCompile(`(?i)</` + tagName + `>`)
		result = openPattern.ReplaceAllString(result, "")
		result = closePattern.ReplaceAllString(result, "")
	}

	return result
}

// StripIRCCodes removes all IRC formatting control characters from text
// Useful for getting plain text from formatted messages
func StripIRCCodes(input string) string {
	if input == "" {
		return input
	}

	result := input

	// Remove color codes (0x03 followed by optional digits and comma)
	colorPattern := regexp.MustCompile("\x03(?:\\d{1,2}(?:,\\d{1,2})?)?")
	result = colorPattern.ReplaceAllString(result, "")

	// Remove hex color codes (0x04 followed by 6 hex digits)
	hexColorPattern := regexp.MustCompile("\x04[0-9A-Fa-f]{6}")
	result = hexColorPattern.ReplaceAllString(result, "")

	// Remove simple control characters
	controlChars := []string{Bold, Italic, Underline, Strikethrough, Monospace, Reverse, Reset}
	for _, char := range controlChars {
		result = strings.ReplaceAll(result, char, "")
	}

	return result
}

// HasFormatting checks if the input contains any custom formatting tags
func HasFormatting(input string) bool {
	// Check for any tag-like patterns
	tagPattern := regexp.MustCompile(`(?i)<(BOLD|B|ITALIC|I|UNDERLINE|U|STRIKE|S|MONO|M|COLOR|REVERSE|RESET)[^>]*>`)
	return tagPattern.MatchString(input)
}

// Default formatter instance for convenience
var defaultFormatter = NewFormatter()

// Format converts custom tags to IRC codes using the default formatter
func Format(input string) string {
	return defaultFormatter.Format(input)
}

// StripTags removes custom tags using the default formatter
func StripTags(input string) string {
	return defaultFormatter.StripTags(input)
}
