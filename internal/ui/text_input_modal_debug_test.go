package ui

import (
	"strings"
	"testing"
)

func TestTextInputViewNewlines(t *testing.T) {
	m := NewTextInputModal(TextInputModalConfig{Title: "Test", Label: "Label", Description: "desc"})
	m.SetDimensions(20, 6)
	m.FocusInput()
	m.SetValue("1234567890")
	m.input.SetWidth(10)
	view := m.input.View()
	t.Logf("raw=%q", view)
	if strings.Count(view, "\n") != 0 {
		t.Fatalf("view contained newline: %q", view)
	}
}
