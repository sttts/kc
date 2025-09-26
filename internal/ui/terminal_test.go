package ui

import (
	"testing"
)

func TestInsertInputAtCursor(t *testing.T) {
	terminal := &Terminal{}

	tests := []struct {
		name       string
		line       string
		cursorX    int
		input      string
		wantLine   string
		wantCursor int
	}{
		{
			name:       "empty input",
			line:       "hello world",
			cursorX:    6,
			input:      "",
			wantLine:   "hello world",
			wantCursor: 6,
		},
		{
			name:       "input fits in available space",
			line:       "hello world",
			cursorX:    6,
			input:      "there",
			wantLine:   "hello there ",
			wantCursor: 11,
		},
		{
			name:       "input exactly fits available space",
			line:       "hello world",
			cursorX:    6,
			input:      "world",
			wantLine:   "hello world ",
			wantCursor: 11,
		},
		{
			name:       "input too long, needs truncation",
			line:       "hello world",
			cursorX:    6,
			input:      "very long input",
			wantLine:   "hello very ",
			wantCursor: 11,
		},
		{
			name:       "input longer than entire line",
			line:       "hello world",
			cursorX:    6,
			input:      "extremely long input that exceeds the entire line length",
			wantLine:   "hello extr ",
			wantCursor: 11,
		},
		{
			name:       "cursor at beginning of line",
			line:       "hello world",
			cursorX:    0,
			input:      "hi",
			wantLine:   "hiello worl ",
			wantCursor: 2,
		},
		{
			name:       "cursor at end of line",
			line:       "hello world",
			cursorX:    11,
			input:      "test",
			wantLine:   "hello worldtest ",
			wantCursor: 15,
		},
		{
			name:       "input needs cutting from beginning",
			line:       "hello world",
			cursorX:    8,
			input:      "test",
			wantLine:   "llo wotest ",
			wantCursor: 12,
		},
		{
			name:       "single character line",
			line:       "a",
			cursorX:    0,
			input:      "b",
			wantLine:   "b ",
			wantCursor: 1,
		},
		{
			name:       "empty line",
			line:       "",
			cursorX:    0,
			input:      "hello",
			wantLine:   "hello ",
			wantCursor: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLine, gotCursor := terminal.insertInputAtCursor(tt.line, tt.cursorX, tt.input)

			if gotLine != tt.wantLine {
				t.Errorf("insertInputAtCursor() line = %q, want %q", gotLine, tt.wantLine)
			}
			if gotCursor != tt.wantCursor {
				t.Errorf("insertInputAtCursor() cursor = %d, want %d", gotCursor, tt.wantCursor)
			}
		})
	}
}

func TestInsertInputAtCursorLineLength(t *testing.T) {
	terminal := &Terminal{}

	// Test that line length is always preserved (except for the added space)
	line := "hello world"
	originalLen := len(line)

	tests := []struct {
		cursorX int
		input   string
	}{
		{0, "a"},
		{5, "test"},
		{6, "very long input that should be truncated"},
		{11, "anything"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result, _ := terminal.insertInputAtCursor(line, tt.cursorX, tt.input)
			expectedLen := originalLen + 1 // +1 for the space at the end
			if len(result) != expectedLen {
				t.Errorf("Line length changed: got %d, want %d", len(result), expectedLen)
			}
		})
	}
}
