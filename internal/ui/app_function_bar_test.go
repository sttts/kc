package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sttts/kc/internal/models"
	"github.com/sttts/kc/internal/table"
	uistyles "github.com/sttts/kc/internal/ui/styles"
)

// Reusing mocks from panel_capabilities_test.go (copying here for self-containment)
type mockFolderFB struct {
	items []models.Item
}

func (f *mockFolderFB) Columns() []table.Column { return []table.Column{{Title: "Name", Width: 10}} }
func (f *mockFolderFB) Title() string           { return "Mock" }
func (f *mockFolderFB) Path() []string          { return []string{"mock"} }
func (f *mockFolderFB) ItemByID(ctx context.Context, id string) (models.Item, bool) {
	for _, it := range f.items {
		if col, _, _, _ := it.Columns(); col == id {
			return it, true
		}
	}
	return nil, false
}
func (f *mockFolderFB) Lines(ctx context.Context, top, num int) []table.Row {
	res := make([]table.Row, 0, num)
	for i := top; i < top+num && i < len(f.items); i++ {
		res = append(res, f.items[i])
	}
	return res
}
func (f *mockFolderFB) Above(ctx context.Context, rowID string, num int) []table.Row { return nil }
func (f *mockFolderFB) Below(ctx context.Context, rowID string, num int) []table.Row { return nil }
func (f *mockFolderFB) Find(ctx context.Context, query string) (int, table.Row, bool) {
	return -1, nil, false
}
func (f *mockFolderFB) Len(ctx context.Context) int         { return len(f.items) }
func (f *mockFolderFB) RegisterDirtyListener(func()) func() { return func() {} }

type mockBackItemFB struct{}

func (m mockBackItemFB) Columns() (string, []string, []*lipgloss.Style, bool) {
	return "__back__", []string{".."}, nil, true
}
func (m mockBackItemFB) IsBack() bool    { return true }
func (m mockBackItemFB) Details() string { return "Back" }
func (m mockBackItemFB) Path() []string  { return nil }

type mockItemFB struct {
	id string
}

func (m mockItemFB) Columns() (string, []string, []*lipgloss.Style, bool) {
	return m.id, []string{m.id}, nil, true
}
func (m mockItemFB) Details() string { return "Item" }
func (m mockItemFB) Path() []string  { return []string{m.id} }

// Implement Viewable to enable F3
func (m mockItemFB) ViewContent() (string, string, string, string, string, error) {
	return "title", "content", "txt", "text/plain", "file.txt", nil
}

func TestAppFunctionBarUpdate(t *testing.T) {
	app := NewApp()
	app.width = 100
	app.height = 30
	ctx := t.Context()

	// Setup Left Panel: Normal item selected
	leftFolder := &mockFolderFB{
		items: []models.Item{mockItemFB{id: "item1"}},
	}
	app.leftPanel.UseFolder(ctx, true)
	app.leftPanel.SetDimensions(ctx, 50, 20)
	app.leftPanel.SetFolder(ctx, leftFolder, false)
	app.leftPanel.SelectByRowID(ctx, "item1")
	app.leftPanel.Init() // Ensure widget init
	app.leftPanel.RefreshFolder(ctx)

	// Verify Left Panel Capabilities
	capsLeft := app.leftPanel.Capabilities(ctx)
	if !capsLeft.CanView {
		t.Fatal("Left panel should have CanView=true")
	}

	// Setup Right Panel: Fresh folder with Back item
	// We simulate entering a folder.
	// 1. Start with a folder having an item (to set stale lastSelection)
	staleFolder := &mockFolderFB{
		items: []models.Item{mockItemFB{id: "stale"}},
	}
	app.rightPanel.UseFolder(ctx, true)
	app.rightPanel.SetDimensions(ctx, 50, 20)
	app.rightPanel.SetFolder(ctx, staleFolder, false)
	app.rightPanel.SelectByRowID(ctx, "stale")

	// Verify stale state
	capsRightStale := app.rightPanel.Capabilities(ctx)
	if !capsRightStale.CanView {
		t.Fatal("Right panel should have CanView=true (stale setup)")
	}

	// 2. Now simulate entering a new folder (empty with back)
	emptyFolder := &mockFolderFB{items: []models.Item{}}
	app.rightPanel.SetFolder(ctx, emptyFolder, true) // hasBack=true
	app.rightPanel.ResetSelectionTop(ctx)

	// Verify Right Panel Capabilities (should be empty)
	capsRight := app.rightPanel.Capabilities(ctx)
	if capsRight.CanView {
		t.Fatal("Right panel should have CanView=false (after entering fresh folder)")
	}

	enabledF3 := uistyles.FunctionKeyStyle.Render("F3") + uistyles.FunctionKeyDescriptionStyle.Render("View")
	disabledF3 := uistyles.FunctionKeyStyle.Render("F3") + uistyles.FunctionKeyDisabledStyle.Render("View")

	// Test App switching
	app.activePanel = 0 // Left active
	app.invalidateFunctionBar("test: left active")
	barLeft := app.renderFunctionKeys()
	if !strings.Contains(barLeft, enabledF3) {
		t.Fatalf("expected enabled F3 entry in function bar, got:\n%s", barLeft)
	}

	app.activePanel = 1 // Switch to Right
	app.invalidateFunctionBar("test: right active")
	barRight := app.renderFunctionKeys()
	if !strings.Contains(barRight, disabledF3) {
		t.Fatalf("expected disabled F3 entry in function bar, got:\n%s", barRight)
	}
	if strings.Contains(barRight, enabledF3) {
		t.Fatalf("unexpected enabled F3 entry in function bar, got:\n%s", barRight)
	}
}
