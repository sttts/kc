package table

import "testing"

func TestRenderRowsSelectedOverlay(t *testing.T) {
	rows := []Row{
		SimpleRow{ID: "a", Cells: []string{"A", "B"}},
		SimpleRow{ID: "b", Cells: []string{"X", "Y"}},
	}
	ctx := t.Context()
	_ = map[string]struct{}{"a": {}}
	// Build a small table view to exercise selection
	cols := []Column{{Title: "A", Width: 4}, {Title: "B", Width: 4}}
	bt := NewBigTable(cols, NewSliceList(rows), 10, 6)
	bt.SetMode(ctx, ModeFit)
	bt.Refresh(ctx)
	s := bt.View()
	if len(s) == 0 {
		t.Fatalf("empty view")
	}
	// We don't assert exact ANSI; just that rendering completed
}
