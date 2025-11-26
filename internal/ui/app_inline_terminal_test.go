package ui

import "testing"

func TestInlineTerminalVisibleInCommandMode(t *testing.T) {
	app := NewApp()
	app.width = 80
	app.height = 24

	ctx := t.Context()
	app.leftPanel.SetMode(ctx, PanelModeCommand)

	if h := app.inlineTerminalHeight(); h != 2 {
		t.Fatalf("expected inline terminal height 2 in command mode, got %d", h)
	}

	_, _, panelHeight, _ := app.panelAreaMetrics()
	expectedPanelHeight := app.height - (1 + app.inlineTerminalHeight())
	if panelHeight != expectedPanelHeight {
		t.Fatalf("unexpected panel height with inline terminal visible: got %d, want %d", panelHeight, expectedPanelHeight)
	}
}
