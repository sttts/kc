package table

import (
	"testing"

	"github.com/charmbracelet/lipgloss/v2"
)

func makeRow(id string, n int) Row {
	r := SimpleRow{ID: id, Cells: make([]string, n), Styles: make([]*lipgloss.Style, n)}
	return r
}

func TestSliceListBasicOps(t *testing.T) {
	l := NewSliceList([]Row{makeRow("a", 2), makeRow("b", 2)})
	ctx := t.Context()
	if l.Len(ctx) != 2 {
		t.Fatalf("len want 2 got %d", l.Len(ctx))
	}
	// Insert after a
	l.InsertAfter("a", makeRow("a1", 2))
	if idx, _, ok := l.Find(ctx, "a1"); !ok || idx != 1 {
		t.Fatalf("a1 at 1, got %d ok=%v", idx, ok)
	}
	// Insert before b
	l.InsertBefore("b", makeRow("x", 2))
	if idx, _, ok := l.Find(ctx, "x"); !ok || idx != 2 {
		t.Fatalf("x at 2, got %d ok=%v", idx, ok)
	}
	// Above/Below
	above := l.Above(ctx, "b", 2)
	if len(above) == 0 {
		t.Fatalf("expected above rows")
	}
	below := l.Below(ctx, "a", 2)
	if len(below) == 0 {
		t.Fatalf("expected below rows")
	}
	// Remove
	l.RemoveIDs("a1", "x")
	if l.Len(ctx) != 2 {
		t.Fatalf("len want 2 after remove got %d", l.Len(ctx))
	}
}
