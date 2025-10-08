package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea/v2"
)

func pressKey(code rune, text string, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text, Mod: mod}
}

func TestDeleteConfirmDefaultNo(t *testing.T) {
	model := NewDeleteConfirmModel()
	model.Configure("pods.v1./demo", "default")
	_, cmd := model.Update(pressKey(tea.KeyEnter, "", 0))
	if cmd == nil {
		t.Fatalf("expected command on enter")
	}
	msg := cmd()
	res, ok := msg.(DeleteConfirmMsg)
	if !ok {
		t.Fatalf("expected DeleteConfirmMsg, got %T", msg)
	}
	if res.Confirm {
		t.Fatalf("expected confirmation false by default")
	}
}

func TestDeleteConfirmYesSelection(t *testing.T) {
	model := NewDeleteConfirmModel()
	model.Configure("pods.v1./demo", "default")
	m, _ := model.Update(pressKey(tea.KeyLeft, "", 0))
	model = m.(*DeleteConfirmModel)
	_, cmd := model.Update(pressKey(tea.KeyEnter, "", 0))
	if cmd == nil {
		t.Fatalf("expected command after moving focus")
	}
	msg := cmd()
	res, ok := msg.(DeleteConfirmMsg)
	if !ok {
		t.Fatalf("expected DeleteConfirmMsg, got %T", msg)
	}
	if !res.Confirm {
		t.Fatalf("expected confirmation true when selecting yes")
	}
}

func TestDeleteConfirmRuneShortcut(t *testing.T) {
	model := NewDeleteConfirmModel()
	model.Configure("pods.v1./demo", "")
	_, cmd := model.Update(pressKey('y', "y", 0))
	if cmd == nil {
		t.Fatalf("expected command for rune shortcut")
	}
	msg := cmd()
	res, ok := msg.(DeleteConfirmMsg)
	if !ok {
		t.Fatalf("expected DeleteConfirmMsg, got %T", msg)
	}
	if !res.Confirm {
		t.Fatalf("expected confirm true via shortcut")
	}
}
