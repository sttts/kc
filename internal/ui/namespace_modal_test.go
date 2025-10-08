package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea/v2"
)

func press(code rune, text string, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text, Mod: mod}
}

func TestNamespaceCreateModelEnterConfirm(t *testing.T) {
	model := NewNamespaceCreateModel()
	m, _ := model.Update(press('t', "t", 0))
	model = m.(*NamespaceCreateModel)
	m, _ = model.Update(press('e', "e", 0))
	model = m.(*NamespaceCreateModel)
	m, _ = model.Update(press('s', "s", 0))
	model = m.(*NamespaceCreateModel)
	m, _ = model.Update(press('t', "t", 0))
	model = m.(*NamespaceCreateModel)
	m, cmd := model.Update(press(tea.KeyEnter, "", 0))
	if cmd == nil {
		t.Fatalf("expected command on enter")
	}
	msg := cmd()
	result, ok := msg.(NamespaceCreateResultMsg)
	if !ok {
		t.Fatalf("expected NamespaceCreateResultMsg, got %T", msg)
	}
	if !result.Confirm || !result.Close || result.Name != "test" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestNamespaceCreateModelValidation(t *testing.T) {
	model := NewNamespaceCreateModel()
	m, cmd := model.Update(press(tea.KeyEnter, "", 0))
	if cmd != nil {
		t.Fatalf("expected nil command when name empty")
	}
	model = m.(*NamespaceCreateModel)
	if model.err == "" {
		t.Fatalf("expected validation error")
	}
}

func TestNamespaceCreateModelEscCancels(t *testing.T) {
	model := NewNamespaceCreateModel()
	_, cmd := model.Update(press(tea.KeyEsc, "", 0))
	if cmd == nil {
		t.Fatalf("expected command on esc")
	}
	msg := cmd()
	res, ok := msg.(NamespaceCreateResultMsg)
	if !ok {
		t.Fatalf("expected NamespaceCreateResultMsg, got %T", msg)
	}
	if res.Confirm || !res.Close {
		t.Fatalf("unexpected result: %+v", res)
	}
}
