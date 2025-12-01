package splitter

import (
	"strings"
	"testing"
)

func TestSplit_PlainText(t *testing.T) {
	s := New(50)

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "short message",
			input:    "Hello world",
			expected: []string{"Hello world"},
		},
		{
			name:     "exactly at limit",
			input:    strings.Repeat("a", 50),
			expected: []string{strings.Repeat("a", 50)},
		},
		{
			name:     "needs split at word boundary",
			input:    "Hello world this is a test message that needs to be split",
			expected: []string{"Hello world this is a test message that needs to", "be split"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.Split(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Split() returned %d parts, want %d", len(result), len(tt.expected))
				t.Errorf("Got: %v", result)
				return
			}
			for i, part := range result {
				if part != tt.expected[i] {
					t.Errorf("Split()[%d] = %q, want %q", i, part, tt.expected[i])
				}
			}
		})
	}
}

func TestSplit_WithBoldFormatting(t *testing.T) {
	s := New(30)

	// Bold text that needs splitting should carry formatting to next part
	input := "\x02This is bold text that is very long and needs splitting\x02"

	result := s.Split(input)

	// First part should have bold
	if !strings.HasPrefix(result[0], "\x02") {
		t.Errorf("First part should start with bold, got: %q", result[0])
	}

	// Second part should also start with bold (carried over)
	if len(result) > 1 && !strings.HasPrefix(result[1], "\x02") {
		t.Errorf("Second part should start with bold (carried over), got: %q", result[1])
	}
}

func TestSplit_WithColorFormatting(t *testing.T) {
	s := New(30)

	// Colored text that needs splitting
	input := "\x0304This is red text that is very long and needs splitting\x03"

	result := s.Split(input)

	// First part should have color
	if !strings.HasPrefix(result[0], "\x0304") {
		t.Errorf("First part should start with color code, got: %q", result[0])
	}

	// Second part should also start with color (carried over)
	if len(result) > 1 && !strings.HasPrefix(result[1], "\x0304") {
		t.Errorf("Second part should start with color (carried over), got: %q", result[1])
	}
}

func TestSplit_WithColorAndBackground(t *testing.T) {
	s := New(40)

	// Colored text with background
	input := "\x0304,01Red on black text that needs to be split into parts\x03"

	result := s.Split(input)

	// First part should have full color code
	if !strings.HasPrefix(result[0], "\x0304,01") {
		t.Errorf("First part should have fg,bg color, got: %q", result[0])
	}

	// Second part should also have full color
	if len(result) > 1 && !strings.HasPrefix(result[1], "\x0304,01") {
		t.Errorf("Second part should have fg,bg color (carried over), got: %q", result[1])
	}
}

func TestSplit_NestedFormatting(t *testing.T) {
	s := New(40)

	// Bold + color
	input := "\x02\x0303Bold green text that is long enough to need splitting\x03\x02"

	result := s.Split(input)

	// Second part should have both bold and color
	if len(result) > 1 {
		part := result[1]
		hasColor := strings.Contains(part, "\x03")
		hasBold := strings.Contains(part, "\x02")
		if !hasColor || !hasBold {
			t.Errorf("Second part should have both bold and color, got: %q", part)
		}
	}
}

func TestSplit_NoSplitInsideColorCode(t *testing.T) {
	s := New(10)

	// Message where naive split would land inside color code
	input := "\x0304,01AB"

	result := s.Split(input)

	// Should not split the color code
	if len(result) != 1 {
		t.Errorf("Should not split short message with color code, got %d parts", len(result))
	}
}

func TestSplit_ResetClearsFormatting(t *testing.T) {
	s := New(30)

	// Bold text with reset in the middle
	input := "\x02Bold text\x0F normal text that continues on"

	result := s.Split(input)

	// If split happens after reset, second part should NOT have bold
	if len(result) > 1 {
		// The second part should not start with bold since reset cleared it
		if strings.HasPrefix(result[1], "\x02") {
			t.Errorf("Second part should not have bold after reset, got: %q", result[1])
		}
	}
}

func TestSplit_ColorCodeCommaNotSplitPoint(t *testing.T) {
	s := New(20)

	// The comma in color code should not be treated as punctuation split point
	input := "\x0304,01Red text here"

	result := s.Split(input)

	// Verify the color code is intact
	if !strings.HasPrefix(result[0], "\x0304,01") {
		t.Errorf("Color code should be intact, got: %q", result[0])
	}
}

func TestSplit_EmptyMessage(t *testing.T) {
	s := New(50)

	result := s.Split("")
	if len(result) != 1 || result[0] != "" {
		t.Errorf("Empty message should return single empty string, got: %v", result)
	}
}

func TestSplit_PlainTextUnchanged(t *testing.T) {
	s := New(100)

	// Plain text without any formatting should work exactly as before
	input := "This is plain text without any IRC formatting codes"

	result := s.Split(input)

	if len(result) != 1 || result[0] != input {
		t.Errorf("Plain text should pass through unchanged, got: %v", result)
	}
}

func TestNeedsSplit(t *testing.T) {
	s := New(50)

	if s.NeedsSplit("short") {
		t.Error("Short message should not need split")
	}

	if !s.NeedsSplit(strings.Repeat("a", 100)) {
		t.Error("Long message should need split")
	}
}

func TestFormatState_ToIRCCodes(t *testing.T) {
	tests := []struct {
		name     string
		state    formatState
		expected string
	}{
		{
			name:     "empty state",
			state:    formatState{},
			expected: "",
		},
		{
			name:     "bold only",
			state:    formatState{bold: true},
			expected: "\x02",
		},
		{
			name:     "color only",
			state:    formatState{fgColor: "04"},
			expected: "\x0304",
		},
		{
			name:     "color with background",
			state:    formatState{fgColor: "04", bgColor: "01"},
			expected: "\x0304,01",
		},
		{
			name:     "bold and color",
			state:    formatState{bold: true, fgColor: "03"},
			expected: "\x0303\x02",
		},
		{
			name:     "multiple toggles",
			state:    formatState{bold: true, italic: true, underline: true},
			expected: "\x02\x1D\x1F",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.state.toIRCCodes()
			if result != tt.expected {
				t.Errorf("toIRCCodes() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestParseFormatState(t *testing.T) {
	s := New(100)

	tests := []struct {
		name     string
		input    string
		expected formatState
	}{
		{
			name:     "plain text",
			input:    "hello world",
			expected: formatState{},
		},
		{
			name:     "bold on",
			input:    "\x02hello",
			expected: formatState{bold: true},
		},
		{
			name:     "bold toggle off",
			input:    "\x02hello\x02",
			expected: formatState{bold: false},
		},
		{
			name:     "color set",
			input:    "\x0304red",
			expected: formatState{fgColor: "04"},
		},
		{
			name:     "color with bg",
			input:    "\x0304,01red on black",
			expected: formatState{fgColor: "04", bgColor: "01"},
		},
		{
			name:     "reset clears all",
			input:    "\x02\x0304bold red\x0F",
			expected: formatState{},
		},
		{
			name:     "color reset",
			input:    "\x0304red\x03normal",
			expected: formatState{fgColor: "", bgColor: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.parseFormatState(tt.input, formatState{})
			if result != tt.expected {
				t.Errorf("parseFormatState() = %+v, want %+v", result, tt.expected)
			}
		})
	}
}
