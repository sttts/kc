package ui

import (
	"testing"

	"github.com/sttts/kc/internal/models"
	modeltesting "github.com/sttts/kc/internal/models/testing"
	nav "github.com/sttts/kc/internal/navigation"
	table "github.com/sttts/kc/internal/table"
)

// helper to make a simple folder
func mkFolder(title string) models.Folder {
	return modeltesting.NewSliceFolder(title, []table.Column{{Title: " Name"}}, nil)
}

func TestIndependentPanelNavigation(t *testing.T) {
	a := NewApp()
	// Seed independent navigators with different roots
	leftRoot := mkFolder("LRoot")
	rightRoot := mkFolder("RRoot")
	a.leftNav = nav.NewNavigator(leftRoot)
	a.rightNav = nav.NewNavigator(rightRoot)
	// Bind folders to panels
	a.leftPanel.UseFolder(true)
	a.rightPanel.UseFolder(true)
	ctx := t.Context()
	a.leftPanel.SetFolder(ctx, leftRoot, false)
	a.rightPanel.SetFolder(ctx, rightRoot, false)
	// Set initial breadcrumbs from navigators
	a.leftPanel.SetCurrentPath(a.navigatorPath(a.leftNav))
	a.rightPanel.SetCurrentPath(a.navigatorPath(a.rightNav))

	// Navigate left only
	a.activePanel = 0
	nextL := mkFolder("L2")
	a.handleFolderNav(false, "selL", nextL)
	if got := a.leftPanel.GetCurrentPath(); got != "/L2" {
		t.Fatalf("left panel path = %q, want /L2", got)
	}
	if got := a.rightPanel.GetCurrentPath(); got != "/RRoot" {
		t.Fatalf("right panel path changed to %q", got)
	}

	// Navigate right only
	a.activePanel = 1
	nextR := mkFolder("R2")
	a.handleFolderNav(false, "selR", nextR)
	if got := a.rightPanel.GetCurrentPath(); got != "/R2" {
		t.Fatalf("right panel path = %q, want /R2", got)
	}
	if got := a.leftPanel.GetCurrentPath(); got != "/L2" {
		t.Fatalf("left panel path changed to %q", got)
	}
}

func TestPanelModeModalSwitchesMode(t *testing.T) {
	a := NewApp()
	ctx := t.Context()
	a.leftPanel.UseFolder(true)
	a.leftPanel.SetFolder(ctx, mkFolder("root"), false)
	if cmd := a.showPanelModeModal(0); cmd != nil {
		if msg := cmd(); msg != nil {
			a.Update(msg)
		}
	}
	if !a.modalManager.IsModalVisible() {
		t.Fatalf("mode modal not visible")
	}
	// Simulate modal selection callback.
	a.Update(PanelModeSelectedMsg{PanelIndex: 0, Mode: PanelModeManifest})
	if a.leftPanel.Mode() != PanelModeManifest {
		t.Fatalf("panel mode = %v, want manifest", a.leftPanel.Mode())
	}
	if a.modalManager.IsModalVisible() {
		t.Fatalf("mode modal still visible after selection")
	}
}
