package ui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sttts/kc/internal/models"
	"github.com/sttts/kc/internal/table"
)

type mockFolder struct {
	items []models.Item
}

func (f *mockFolder) Columns() []table.Column { return []table.Column{{Title: "Name", Width: 10}} }
func (f *mockFolder) Title() string           { return "Mock" }
func (f *mockFolder) Path() []string          { return []string{"mock"} }
func (f *mockFolder) ItemByID(ctx context.Context, id string) (models.Item, bool) {
	for _, it := range f.items {
		if col, _, _, _ := it.Columns(); col == id {
			return it, true
		}
	}
	return nil, false
}
func (f *mockFolder) Lines(ctx context.Context, top, num int) []table.Row {
	res := make([]table.Row, 0, num)
	for i := top; i < top+num && i < len(f.items); i++ {
		res = append(res, f.items[i])
	}
	return res
}
func (f *mockFolder) Above(ctx context.Context, rowID string, num int) []table.Row { return nil }
func (f *mockFolder) Below(ctx context.Context, rowID string, num int) []table.Row { return nil }
func (f *mockFolder) Find(ctx context.Context, query string) (int, table.Row, bool) {
	return -1, nil, false
}
func (f *mockFolder) Len(ctx context.Context) int { return len(f.items) }

func (f *mockFolder) RegisterDirtyListener(func()) func() { return func() {} }

type mockBackItem struct{}

func (m mockBackItem) Columns() (string, []string, []*lipgloss.Style, bool) {
	return "__back__", []string{".."}, nil, true
}
func (m mockBackItem) IsBack() bool    { return true }
func (m mockBackItem) Details() string { return "Back" }
func (m mockBackItem) Path() []string  { return nil }

type mockItem struct {
	id string
}

func (m mockItem) Columns() (string, []string, []*lipgloss.Style, bool) {
	return m.id, []string{m.id}, nil, true
}
func (m mockItem) Details() string { return "Item" }
func (m mockItem) Path() []string  { return []string{m.id} }

func TestPanelCapabilities_BackItem(t *testing.T) {
	p := NewPanel("Test")
	ctx := t.Context()

	// Case 1: Normal item
	folder := &mockFolder{
		items: []models.Item{mockItem{id: "item1"}},
	}
	p.UseFolder(ctx, true)
	if model, _ := p.Update(tea.WindowSizeMsg{Width: 80, Height: 24}); model != nil {
		p = model.(*Panel)
	}
	p.SetFolder(ctx, folder, false)
	p.SelectByRowID(ctx, "item1")

	// We need to ensure the widget is initialized and updated
	p.Init()
	// Force update to sync folder
	p.RefreshFolder(ctx)

	// Check capabilities
	item, ok := p.SelectedNavItem(ctx)
	if !ok {
		t.Logf("SelectedNavItem failed. Current selection ID: %q", p.currentSelectionID(ctx))
		t.Errorf("Expected item selected, got none")
	} else if item == nil {
		t.Errorf("Expected item, got nil")
	}

	// Case 2: Back item
	// When hasBack=true, the list widget injects a back item.
	// We simulate selecting it by selecting index 0 or ID "__back__"
	emptyFolder := &mockFolder{items: []models.Item{}}
	p.SetFolder(ctx, emptyFolder, true)
	p.RefreshFolder(ctx)
	p.ResetSelectionTop(ctx) // Should select ".."

	// Verify selection is back
	item, ok = p.SelectedNavItem(ctx)
	if ok {
		t.Errorf("Expected SelectedNavItem to return false for Back item, got true: %v", item)
	}

	caps := p.Capabilities(ctx)
	if caps.CanView {
		t.Errorf("Expected CanView=false for Back item")
	}
}
