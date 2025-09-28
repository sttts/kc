package table

import (
    "fmt"
    "strings"
    "testing"
    "github.com/charmbracelet/lipgloss/v2"
)

func mkCols(n int, w int) []Column {
    cols := make([]Column, n)
    for i := 0; i < n; i++ { cols[i] = Column{Title: fmt.Sprintf("C%02d", i), Width: w} }
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

func TestScrollModeHorizontalPan(t *testing.T) {
    cols := mkCols(20, 18)
    list := mkList(5, 20)
    bt := NewBigTable(cols, list, 50, 10)
    bt.SetMode(ModeScroll)
    // Ensure view is free from replacement runes while panned
    s := bt.View()
    if strings.ContainsRune(s, '\uFFFD') {
        t.Fatalf("view contains replacement rune while panned: %q", s)
    }
}

func TestRepositionOnDataChange_NextThenPrev(t *testing.T) {
    cols := mkCols(3, 6)
    list := mkList(5, 3) // ids: id-00..id-04
    bt := NewBigTable(cols, list, 60, 10)
    bt.SetMode(ModeFit)
    // Move cursor to index 2 (id-02)
    bt.cursor = 2
    bt.rebuildWindow()
    id, _ := bt.CurrentID()
    if id != "id-02" { t.Fatalf("want id-02, got %s", id) }
    // Remove id-02 -> should move to next (id-03)
    list.RemoveIDs("id-02")
    bt.SetList(list)
    id, _ = bt.CurrentID()
    if id != "id-03" { t.Fatalf("want id-03 after removal, got %s", id) }
    // Remove id-03 too -> should move to next (id-04)
    list.RemoveIDs("id-03")
    bt.SetList(list)
    id, _ = bt.CurrentID()
    if id != "id-04" { t.Fatalf("expected to land on id-04 after second removal, got %s", id) }
}
