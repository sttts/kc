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
	a.leftPanel.SetFolder(leftRoot, false)
	a.rightPanel.SetFolder(rightRoot, false)
	// Set initial breadcrumbs from navigators
	a.leftPanel.SetCurrentPath(a.leftNav.Path())
	a.rightPanel.SetCurrentPath(a.rightNav.Path())

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
