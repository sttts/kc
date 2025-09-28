package main

import "testing"

func TestRenderRowsSelectedOverlay(t *testing.T) {
    rows := []Row{
        SimpleRow{ID: "a", Cells: []string{"A", "B"}},
        SimpleRow{ID: "b", Cells: []string{"X", "Y"}},
    }
    sel := map[string]struct{}{"a": {}}
    out := renderRowsFromSlice(rows, sel)
    if len(out) != 2 { t.Fatalf("expected 2 rows") }
    a0 := out[0][0]
    b0 := out[1][0]
    if a0 == "A" || b0 != "X" {
        // sanity: b0 should be plain; a0 should not be plain
    }
    if len(a0) == 1 || a0 == "A" {
        t.Fatalf("expected selected cell to be styled, got %q", a0)
    }
    if b0 != "X" {
        t.Fatalf("expected unselected cell plain 'X', got %q", b0)
    }
}

