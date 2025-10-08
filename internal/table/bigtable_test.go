package table

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/lipgloss/v2"
)

func mkCols(n int, w int) []Column {
	cols := make([]Column, n)
	for i := 0; i < n; i++ {
		cols[i] = Column{Title: fmt.Sprintf("C%02d", i), Width: w}
	}
	return cols
}

func mkRow(id string, cols int) Row {
	r := SimpleRow{ID: id}
	for i := 0; i < cols; i++ {
		s := lipgloss.NewStyle()
		r.SetColumn(i, id, &s)
	}
	return r
}

func mkList(n, cols int) *SliceList {
	rows := make([]Row, 0, n)
	for i := 0; i < n; i++ {
		id := "id-" + pad2(i)
		rows = append(rows, mkRow(id, cols))
	}
	return NewSliceList(rows)
}

func pad2(i int) string { return fmt.Sprintf("%02d", i) }

// Horizontal panning is no longer supported; only Auto/Fit modes remain.

func TestRepositionOnDataChange_NextThenPrev(t *testing.T) {
	cols := mkCols(3, 6)
	list := mkList(5, 3) // ids: id-00..id-04
	ctx := t.Context()
	bt := NewBigTable(cols, list, 60, 10)
	bt.SetMode(ctx, ModeFit)
	bt.Refresh(ctx)
	// Move cursor to index 2 (id-02)
	bt.cursor = 2
	bt.rebuildWindow(ctx)
	id, _ := bt.CurrentID(ctx)
	if id != "id-02" {
		t.Fatalf("want id-02, got %s", id)
	}
	// Remove id-02 -> should move to next (id-03)
	list.RemoveIDs("id-02")
	bt.SetList(ctx, list)
	id, _ = bt.CurrentID(ctx)
	if id != "id-03" {
		t.Fatalf("want id-03 after removal, got %s", id)
	}
	// Remove id-03 too -> should move to next (id-04)
	list.RemoveIDs("id-03")
	bt.SetList(ctx, list)
	id, _ = bt.CurrentID(ctx)
	if id != "id-04" {
		t.Fatalf("expected to land on id-04 after second removal, got %s", id)
	}
}
