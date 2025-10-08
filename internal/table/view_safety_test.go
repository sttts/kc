package table

import (
	"strings"
	"testing"
)

func TestViewNoReplacementRune_Scroll(t *testing.T) {
	cols := []Column{{Title: "Col1", Width: 8}, {Title: "Col2", Width: 8}, {Title: "Col3", Width: 8}}
	rows := []Row{
		SimpleRow{ID: "a", Cells: []string{"id-0001", "ERROR", "row-0001 col-03 sample"}},
		SimpleRow{ID: "b", Cells: []string{"id-0002", "OK", "row-0002 col-03 sample"}},
	}
	ctx := t.Context()
	bt := NewBigTable(cols, NewSliceList(rows), 24, 8)
	bt.SetMode(ctx, ModeScroll)
	bt.Refresh(ctx)
	s := bt.View()
	if strings.ContainsRune(s, '\uFFFD') {
		t.Fatalf("view contains replacement rune in Scroll mode: %q", s)
	}
}

func TestViewNoReplacementRune_Fit(t *testing.T) {
	cols := []Column{{Title: "Col1", Width: 8}, {Title: "Col2", Width: 8}, {Title: "Col3", Width: 8}}
	rows := []Row{
		SimpleRow{ID: "a", Cells: []string{"id-0001", "ERROR", "row-0001 col-03 sample"}},
		SimpleRow{ID: "b", Cells: []string{"id-0002", "OK", "row-0002 col-03 sample"}},
	}
	ctx := t.Context()
	bt := NewBigTable(cols, NewSliceList(rows), 24, 8)
	bt.SetMode(ctx, ModeFit)
	bt.Refresh(ctx)
	s := bt.View()
	if strings.ContainsRune(s, '\uFFFD') {
		t.Fatalf("view contains replacement rune in Fit mode: %q", s)
	}
}
