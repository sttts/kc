package ui

import (
	"testing"
)

func TestNewApp(t *testing.T) {
	app := NewApp()

	if app == nil {
		t.Fatal("NewApp() returned nil")
	}

	if app.leftPanel == nil {
		t.Error("Left panel is nil")
	}

	if app.rightPanel == nil {
		t.Error("Right panel is nil")
	}

	if app.terminal == nil {
		t.Error("Terminal is nil")
	}

	if app.activePanel != 0 {
		t.Errorf("Expected active panel to be 0, got %d", app.activePanel)
	}

	if app.showTerminal {
		t.Error("Expected showTerminal to be false initially")
	}
}

func TestNewPanel(t *testing.T) {
	panel := NewPanel("Test Panel")

	if panel == nil {
		t.Fatal("NewPanel() returned nil")
	}

	if panel.title != "Test Panel" {
		t.Errorf("Expected title to be 'Test Panel', got '%s'", panel.title)
	}

	if len(panel.items) != 0 {
		t.Errorf("Expected empty items slice, got length %d", len(panel.items))
	}

	if panel.selected != 0 {
		t.Errorf("Expected selected to be 0, got %d", panel.selected)
	}
}

func TestNewTerminal(t *testing.T) {
	terminal := NewTerminal()

	if terminal == nil {
		t.Fatal("NewTerminal() returned nil")
	}

	if len(terminal.history) != 0 {
		t.Errorf("Expected empty history, got length %d", len(terminal.history))
	}

	if terminal.currentCmd != "" {
		t.Errorf("Expected empty current command, got '%s'", terminal.currentCmd)
	}

	if terminal.cursor != 0 {
		t.Errorf("Expected cursor to be 0, got %d", terminal.cursor)
	}
}

func TestPanelSetDimensions(t *testing.T) {
	panel := NewPanel("Test")
	panel.SetDimensions(100, 50)

	if panel.width != 100 {
		t.Errorf("Expected width to be 100, got %d", panel.width)
	}

	if panel.height != 50 {
		t.Errorf("Expected height to be 50, got %d", panel.height)
	}
}

func TestTerminalSetDimensions(t *testing.T) {
	terminal := NewTerminal()
	terminal.SetDimensions(100, 50)

	if terminal.width != 100 {
		t.Errorf("Expected width to be 100, got %d", terminal.width)
	}

	if terminal.height != 50 {
		t.Errorf("Expected height to be 50, got %d", terminal.height)
	}
}
