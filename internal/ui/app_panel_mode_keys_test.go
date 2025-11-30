package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCtrlOneCyclesLeftPanelMode(t *testing.T) {
	app := NewApp()
	app.width = 80
	app.height = 24

	if app.leftPanel.Mode() != PanelModeList {
		t.Fatalf("expected left panel to start in list mode")
	}

	model, _ := app.Update(tea.KeyPressMsg{Code: '1', Text: "1", Mod: tea.ModCtrl})
	app = model.(*App)

	if got := app.leftPanel.Mode(); got != PanelModeDescribe {
		t.Fatalf("expected left panel mode to cycle to Describe, got %v", got)
	}
}

func TestCtrlTwoCyclesRightPanelMode(t *testing.T) {
	app := NewApp()
	app.width = 80
	app.height = 24

	if app.rightPanel.Mode() != PanelModeList {
		t.Fatalf("expected right panel to start in list mode")
	}

	model, _ := app.Update(tea.KeyPressMsg{Code: '2', Text: "2", Mod: tea.ModCtrl})
	app = model.(*App)

	if got := app.rightPanel.Mode(); got != PanelModeDescribe {
		t.Fatalf("expected right panel mode to cycle to Describe, got %v", got)
	}
}

func TestCtrlCycleSkipsHiddenPanel(t *testing.T) {
	app := NewApp()
	app.width = 80
	app.height = 24

	// Hide right panel
	_ = app.setPanelWidthPercent(1, 100)

	model, _ := app.Update(tea.KeyPressMsg{Code: '2', Text: "2", Mod: tea.ModCtrl})
	app = model.(*App)

	if got := app.rightPanel.Mode(); got != PanelModeList {
		t.Fatalf("expected right panel mode to remain List when hidden, got %v", got)
	}

	// Hide left panel
	_ = app.setPanelWidthPercent(0, 100)

	model, _ = app.Update(tea.KeyPressMsg{Code: '1', Text: "1", Mod: tea.ModCtrl})
	app = model.(*App)

	if got := app.leftPanel.Mode(); got != PanelModeList {
		t.Fatalf("expected left panel mode to remain List when hidden, got %v", got)
	}
}
