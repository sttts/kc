package ui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func TestPanelHelpAndMenuActions(t *testing.T) {
	app := NewApp()
	panel := NewPanel("test")
	panel.SetActionHandlers(app.panelActionHandlers())

	ctx := t.Context()
	caps := panel.Capabilities(ctx)

	if !caps.HasHelp {
		t.Fatal("expected HasHelp to be true when help is implemented")
	}
	if caps.HasContextMenu {
		t.Fatal("expected HasContextMenu to be false until context menu is implemented")
	}
}

func TestDeleteConfirmEnterTriggersMessage(t *testing.T) {
	app := NewApp()
	app.width = 80
	app.height = 24

	modal := app.modalManager.modals["delete_confirm"]
	if modal == nil {
		t.Fatalf("delete_confirm modal not registered")
	}

	target := deleteTarget{
		panelIdx:  0,
		gvr:       schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		namespace: "default",
		name:      "demo",
	}
	app.pendingDelete = &target

	app.deleteConfirm.Configure("pods.v1./demo", "default")
	app.deleteConfirm.SetDimensions(40, 8)
	modal.SetContent(app.deleteConfirm)
	modal.SetDimensions(app.width, app.height)
	modal.SetWindowed(50, 8, "")
	modal.SetOnClose(func() tea.Cmd {
		app.pendingDelete = nil
		return nil
	})
	app.modalManager.Show("delete_confirm")

	// Move focus to "Yes".
	model, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	app = model.(*App)

	model, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected Enter to trigger a command when delete modal is open")
	}
	app = model.(*App)

	msg := cmd()
	res, ok := msg.(DeleteConfirmMsg)
	if !ok {
		t.Fatalf("expected DeleteConfirmMsg, got %T", msg)
	}
	if !res.Confirm || !res.Close {
		t.Fatalf("expected delete confirmation message, got %+v", res)
	}

	if app.pendingDelete == nil {
		t.Fatalf("pendingDelete cleared before handling confirmation")
	}

	model, deleteCmd := app.Update(res)
	app = model.(*App)
	if deleteCmd == nil {
		t.Fatalf("expected delete confirmation to trigger delete command")
	}
}
