package ui

import (
    "testing"
    tea "github.com/charmbracelet/bubbletea/v2"
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
    // Defaults
    if terminal.showPanels != true {
        t.Errorf("Expected showPanels to be true, got %v", terminal.showPanels)
    }
    if terminal.hasTyped != false {
        t.Errorf("Expected hasTyped to be false, got %v", terminal.hasTyped)
    }
    if terminal.shellExited != false {
        t.Errorf("Expected shellExited to be false, got %v", terminal.shellExited)
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

func TestTerminalResizeViaWindowSize(t *testing.T) {
    terminal := NewTerminal()
    // Simulate a window size message to update dimensions
    model, _ := terminal.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
    term := model.(*Terminal)
    if term.width != 100 {
        t.Errorf("Expected width to be 100, got %d", term.width)
    }
    if term.height != 50 {
        t.Errorf("Expected height to be 50, got %d", term.height)
    }
}
