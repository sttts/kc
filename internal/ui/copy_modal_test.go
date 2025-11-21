package ui

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func keyPress(code rune, text string, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text, Mod: mod}
}

func TestCopyToLocalModelConfirm(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	model := NewCopyToLocalModel()
	model.SetDimensions(80, 12)
	model.Configure("logs/foo", target)
	model.FocusPath()
	_, cmd := model.Update(keyPress(tea.KeyEnter, "", 0))
	if cmd == nil {
		t.Fatalf("expected command on enter")
	}
	msg := cmd()
	res, ok := msg.(CopyToLocalResultMsg)
	if !ok {
		t.Fatalf("expected CopyToLocalResultMsg, got %T", msg)
	}
	if !res.Confirm || !res.Close || res.Path != target {
		t.Fatalf("expected confirm+close, got %+v", res)
	}
}

func TestCopyToLocalModelValidationFailure(t *testing.T) {
	model := NewCopyToLocalModel()
	model.SetDimensions(70, 10)
	model.Configure("logs/foo", "/unlikely-dir/out.txt")
	model.FocusPath()
	_, cmd := model.Update(keyPress(tea.KeyEnter, "", 0))
	if cmd != nil {
		t.Fatalf("expected nil command on invalid path")
	}
	if model.modal.Error() == "" {
		t.Fatalf("expected validation error")
	}
}
