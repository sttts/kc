package ui

import (
	"strings"
	"testing"
)

func TestTextInputModalFieldHasNoExtraLines(t *testing.T) {
	m := NewTextInputModal(TextInputModalConfig{
		Title:       "Test",
		Label:       "Label",
		Description: "desc",
	})
	m.SetDimensions(40, 10)
	m.FocusInput()
	m.SetValue("example")
	field, _ := m.renderField(40)
	lineCount := strings.Count(field, "\n")
	t.Logf("field=%q", field)
	if lineCount != 0 {
		t.Fatalf("expected single-line field, got %q", field)
	}
	desc := m.renderDescription(40)
	t.Logf("desc=%q", desc)
	view, _ := m.View()
	t.Logf("view=%q", view)
}
