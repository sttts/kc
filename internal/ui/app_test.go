package ui

import (
	"context"
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

	if lw := panel.listWidget(context.Background()); lw == nil {
		t.Fatalf("expected list widget to initialize")
	} else if len(lw.Items()) != 0 {
		t.Errorf("Expected empty items slice, got length %d", len(lw.Items()))
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
	panel.SetDimensions(t.Context(), 100, 50)

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

func TestPanelHelpAndMenuActionsDisabled(t *testing.T) {
	app := NewApp()
	panel := NewPanel("test")
	panel.SetActionHandlers(app.panelActionHandlers())

	ctx := t.Context()
	caps := panel.Capabilities(ctx)

	if caps.HasHelp {
		t.Fatal("expected HasHelp to be false until help is implemented")
	}
	if caps.HasContextMenu {
		t.Fatal("expected HasContextMenu to be false until context menu is implemented")
	}
}
