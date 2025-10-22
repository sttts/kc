package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea/v2"
)

func TestNormalizePanelWidthPercents(t *testing.T) {
	cases := []struct {
		left, right                 int
		expectedLeft, expectedRight int
	}{
		{0, 0, 50, 50},
		{75, 0, 75, 25},
		{0, 80, 20, 80},
		{120, 10, 100, 0},
		{25, 90, 25, 75},
		{33, 0, 33, 67},
	}

	for _, tc := range cases {
		l, r := normalizePanelWidthPercents(tc.left, tc.right)
		if l != tc.expectedLeft || r != tc.expectedRight {
			t.Fatalf("normalizePanelWidthPercents(%d,%d) = (%d,%d), want (%d,%d)", tc.left, tc.right, l, r, tc.expectedLeft, tc.expectedRight)
		}
	}
}

func TestPanelWidthsForDistributions(t *testing.T) {
	app := NewApp()
	app.leftPanelWidthPercent = 50
	app.rightPanelWidthPercent = 50

	left, right := app.panelWidthsFor(120)
	if left != 60 || right != 60 {
		t.Fatalf("expected 60/60 for 50%% split, got %d/%d", left, right)
	}

	app.setPanelWidthPercent(0, 66)
	left, right = app.panelWidthsFor(120)
	if left != 79 || right != 41 {
		t.Fatalf("expected 79/41 for 66%% split, got %d/%d", left, right)
	}

	app.setPanelWidthPercent(0, 100)
	left, right = app.panelWidthsFor(80)
	if left != 80 || right != 0 {
		t.Fatalf("expected 80/0 for full-width left, got %d/%d", left, right)
	}

	app.setPanelWidthPercent(1, 100)
	left, right = app.panelWidthsFor(80)
	if left != 0 || right != 80 {
		t.Fatalf("expected 0/80 for full-width right, got %d/%d", left, right)
	}

	app.setPanelWidthPercent(0, 50)
	left, right = app.panelWidthsFor(1)
	if left+right != 1 {
		t.Fatalf("expected widths to sum to total, got %d/%d", left, right)
	}
}

func TestCyclePanelWidthSequence(t *testing.T) {
	app := NewApp()

	sequenceLeft := []int{66, 75, 100, 100}
	for _, expected := range sequenceLeft {
		app.cyclePanelWidth(0)
		if app.leftPanelWidthPercent != expected {
			t.Fatalf("left cycle expected %d, got %d", expected, app.leftPanelWidthPercent)
		}
	}

	app.setPanelWidthPercent(0, 50)
	sequenceRight := []int{66, 75, 100, 100}
	for _, expected := range sequenceRight {
		app.cyclePanelWidth(1)
		if app.rightPanelWidthPercent != expected {
			t.Fatalf("right cycle expected %d, got %d", expected, app.rightPanelWidthPercent)
		}
	}
}

func TestTabDisabledWhenPanelHidden(t *testing.T) {
	app := NewApp()
	app.setPanelWidthPercent(0, 100)
	app.activePanel = 0
	app.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if app.activePanel != 0 {
		t.Fatalf("expected active panel to remain left when right hidden, got %d", app.activePanel)
	}

	app.setPanelWidthPercent(1, 100)
	app.activePanel = 1
	app.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if app.activePanel != 1 {
		t.Fatalf("expected active panel to remain right when left hidden, got %d", app.activePanel)
	}
}

func TestAltFDisallowedWhenPanelHidden(t *testing.T) {
	app := NewApp()
	app.setPanelWidthPercent(1, 100) // right full, left hidden
	app.activePanel = 1
	if app.modalManager.IsModalVisible() {
		t.Fatalf("modal should not be visible before test")
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyF1, Mod: tea.ModAlt})
	if app.modalManager.IsModalVisible() {
		t.Fatalf("left view options should not open when left panel hidden")
	}

	app.setPanelWidthPercent(0, 100) // left full, right hidden
	app.activePanel = 0
	app.Update(tea.KeyPressMsg{Code: tea.KeyF2, Mod: tea.ModAlt})
	if app.modalManager.IsModalVisible() {
		t.Fatalf("right view options should not open when right panel hidden")
	}
}
