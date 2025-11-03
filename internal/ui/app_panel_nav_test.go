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
	ctx := t.Context()
	a.leftPanel.UseFolder(ctx, true)
	a.rightPanel.UseFolder(ctx, true)
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

func TestViewOptionsModalTargetsPanels(t *testing.T) {
	a := NewApp()
	ctx := t.Context()
	a.leftPanel.UseFolder(ctx, true)
	a.leftPanel.SetFolder(ctx, mkFolder("rootL"), false)
	a.rightPanel.UseFolder(ctx, true)
	a.rightPanel.SetFolder(ctx, mkFolder("rootR"), false)

	if cmd := a.showViewOptionsModalForPanel(a.leftPanel); cmd != nil {
		if msg := cmd(); msg != nil {
			a.Update(msg)
		}
	}
	modal := a.modalManager.modals["view_options"]
	if modal == nil || !modal.IsVisible() {
		t.Fatalf("view options modal not visible for left panel")
	}
	vm, ok := modal.content.(*ViewOptionsModel)
	if !ok || vm.PanelIndex() != 0 {
		t.Fatalf("expected left panel index 0 in modal, got %v", vm.PanelIndex())
	}
	a.modalManager.Hide()

	if cmd := a.showViewOptionsModalForPanel(a.rightPanel); cmd != nil {
		if msg := cmd(); msg != nil {
			a.Update(msg)
		}
	}
	modal = a.modalManager.modals["view_options"]
	if modal == nil || !modal.IsVisible() {
		t.Fatalf("view options modal not visible for right panel")
	}
	vm, ok = modal.content.(*ViewOptionsModel)
	if !ok || vm.PanelIndex() != 1 {
		t.Fatalf("expected right panel index 1 in modal, got %v", vm.PanelIndex())
	}
}
