package ircformat

import (
	"testing"
)

func TestFormat_Bold(t *testing.T) {
	f := NewFormatter()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple bold",
			input:    "<BOLD>hello</BOLD>",
			expected: "\x02hello\x02",
		},
		{
			name:     "short bold tag",
			input:    "<B>hello</B>",
			expected: "\x02hello\x02",
		},
		{
			name:     "case insensitive",
			input:    "<bold>hello</bold>",
			expected: "\x02hello\x02",
		},
		{
			name:     "mixed case",
			input:    "<Bold>hello</BOLD>",
			expected: "\x02hello\x02",
		},
		{
			name:     "bold with surrounding text",
			input:    "this is <BOLD>important</BOLD> text",
			expected: "this is \x02important\x02 text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := f.Format(tt.input)
			if result != tt.expected {
				t.Errorf("Format(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormat_Italic(t *testing.T) {
	f := NewFormatter()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple italic",
			input:    "<ITALIC>hello</ITALIC>",
			expected: "\x1Dhello\x1D",
		},
		{
			name:     "short italic tag",
			input:    "<I>hello</I>",
			expected: "\x1Dhello\x1D",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := f.Format(tt.input)
			if result != tt.expected {
				t.Errorf("Format(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormat_Underline(t *testing.T) {
	f := NewFormatter()

	result := f.Format("<UNDERLINE>hello</UNDERLINE>")
	expected := "\x1Fhello\x1F"
	if result != expected {
		t.Errorf("Format() = %q, want %q", result, expected)
	}

	result = f.Format("<U>hello</U>")
	if result != expected {
		t.Errorf("Format() = %q, want %q", result, expected)
	}
}

func TestFormat_Strikethrough(t *testing.T) {
	f := NewFormatter()

	// Strikethrough tags should be stripped for compatibility
	result := f.Format("<STRIKE>hello</STRIKE>")
	expected := "hello"
	if result != expected {
		t.Errorf("Format() = %q, want %q", result, expected)
	}

	result = f.Format("<S>hello</S>")
	if result != expected {
		t.Errorf("Format() = %q, want %q", result, expected)
	}
}

func TestFormat_Monospace(t *testing.T) {
	f := NewFormatter()

	// Monospace tags should be converted to Grey color (14) for compatibility
	// \x0314 = Color Grey, \x03 = Reset Color
	result := f.Format("<MONO>code</MONO>")
	expected := "\x0314code\x03"
	if result != expected {
		t.Errorf("Format() = %q, want %q", result, expected)
	}

	result = f.Format("<M>code</M>")
	if result != expected {
		t.Errorf("Format() = %q, want %q", result, expected)
	}
}

func TestFormat_Reverse(t *testing.T) {
	f := NewFormatter()

	result := f.Format("<REVERSE>inverted</REVERSE>")
	expected := "\x16inverted\x16"
	if result != expected {
		t.Errorf("Format() = %q, want %q", result, expected)
	}
}

func TestFormat_Reset(t *testing.T) {
	f := NewFormatter()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "reset tag",
			input:    "<BOLD>bold<RESET/>normal",
			expected: "\x02bold\x0Fnormal",
		},
		{
			name:     "reset with space",
			input:    "<BOLD>bold<RESET />normal",
			expected: "\x02bold\x0Fnormal",
		},
		{
			name:     "case insensitive reset",
			input:    "<BOLD>bold<reset/>normal",
			expected: "\x02bold\x0Fnormal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := f.Format(tt.input)
			if result != tt.expected {
				t.Errorf("Format(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormat_Color(t *testing.T) {
	f := NewFormatter()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "foreground only",
			input:    `<COLOR fg="03">green</COLOR>`,
			expected: "\x0303green\x03",
		},
		{
			name:     "foreground and background",
			input:    `<COLOR fg="04" bg="01">red on black</COLOR>`,
			expected: "\x0304,01red on black\x03",
		},
		{
			name:     "single digit foreground",
			input:    `<COLOR fg="3">green</COLOR>`,
			expected: "\x0303green\x03",
		},
		{
			name:     "single quotes",
			input:    `<COLOR fg='03'>green</COLOR>`,
			expected: "\x0303green\x03",
		},
		{
			name:     "no quotes",
			input:    `<COLOR fg=03>green</COLOR>`,
			expected: "\x0303green\x03",
		},
		{
			name:     "case insensitive",
			input:    `<color fg="03">green</color>`,
			expected: "\x0303green\x03",
		},
		{
			name:     "color with surrounding text",
			input:    `This is <COLOR fg="04">red</COLOR> text`,
			expected: "This is \x0304red\x03 text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := f.Format(tt.input)
			if result != tt.expected {
				t.Errorf("Format(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormat_Nested(t *testing.T) {
	f := NewFormatter()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bold and italic",
			input:    "<BOLD><ITALIC>bold italic</ITALIC></BOLD>",
			expected: "\x02\x1Dbold italic\x1D\x02",
		},
		{
			name:     "bold with color",
			input:    `<BOLD><COLOR fg="03">bold green</COLOR></BOLD>`,
			expected: "\x02\x0303bold green\x03\x02",
		},
		{
			name:     "multiple nested",
			input:    `<BOLD><ITALIC><COLOR fg="04">bold italic red</COLOR></ITALIC></BOLD>`,
			expected: "\x02\x1D\x0304bold italic red\x03\x1D\x02",
		},
		{
			name:     "nested with reset",
			input:    "<BOLD><ITALIC>styled<RESET/>plain",
			expected: "\x02\x1Dstyled\x0Fplain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := f.Format(tt.input)
			if result != tt.expected {
				t.Errorf("Format(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormat_NoTags(t *testing.T) {
	f := NewFormatter()

	// Plain text should pass through unchanged
	input := "Hello, this is plain text!"
	result := f.Format(input)
	if result != input {
		t.Errorf("Format(%q) = %q, want %q", input, result, input)
	}
}

func TestFormat_EmptyString(t *testing.T) {
	f := NewFormatter()

	result := f.Format("")
	if result != "" {
		t.Errorf("Format(\"\") = %q, want \"\"", result)
	}
}

func TestStripTags(t *testing.T) {
	f := NewFormatter()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strip bold",
			input:    "<BOLD>hello</BOLD>",
			expected: "hello",
		},
		{
			name:     "strip color",
			input:    `<COLOR fg="03">green</COLOR>`,
			expected: "green",
		},
		{
			name:     "strip nested",
			input:    `<BOLD><COLOR fg="03">bold green</COLOR></BOLD>`,
			expected: "bold green",
		},
		{
			name:     "strip reset",
			input:    "<BOLD>bold<RESET/>plain",
			expected: "boldplain",
		},
		{
			name:     "plain text unchanged",
			input:    "plain text",
			expected: "plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := f.StripTags(tt.input)
			if result != tt.expected {
				t.Errorf("StripTags(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStripIRCCodes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strip bold",
			input:    "\x02hello\x02",
			expected: "hello",
		},
		{
			name:     "strip color",
			input:    "\x0303green\x03",
			expected: "green",
		},
		{
			name:     "strip color with bg",
			input:    "\x0304,01red on black\x03",
			expected: "red on black",
		},
		{
			name:     "strip multiple",
			input:    "\x02\x0303bold green\x03\x02",
			expected: "bold green",
		},
		{
			name:     "strip reset",
			input:    "\x02bold\x0Fplain",
			expected: "boldplain",
		},
		{
			name:     "plain text unchanged",
			input:    "plain text",
			expected: "plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripIRCCodes(tt.input)
			if result != tt.expected {
				t.Errorf("StripIRCCodes(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestHasFormatting(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "has bold",
			input:    "<BOLD>hello</BOLD>",
			expected: true,
		},
		{
			name:     "has color",
			input:    `<COLOR fg="03">green</COLOR>`,
			expected: true,
		},
		{
			name:     "has reset",
			input:    "text<RESET/>more",
			expected: true,
		},
		{
			name:     "no formatting",
			input:    "plain text",
			expected: false,
		},
		{
			name:     "html-like but not our tags",
			input:    "<div>not our tag</div>",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasFormatting(tt.input)
			if result != tt.expected {
				t.Errorf("HasFormatting(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDefaultFormatter(t *testing.T) {
	// Test the package-level convenience functions
	result := Format("<BOLD>hello</BOLD>")
	expected := "\x02hello\x02"
	if result != expected {
		t.Errorf("Format() = %q, want %q", result, expected)
	}

	result = StripTags("<BOLD>hello</BOLD>")
	expected = "hello"
	if result != expected {
		t.Errorf("StripTags() = %q, want %q", result, expected)
	}
}
