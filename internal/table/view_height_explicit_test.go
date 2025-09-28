package table

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// trimRightEachLine trims trailing spaces on every line to make visual
// comparisons robust while keeping human‑readable, aligned expectations.
func trimRightEachLine(s string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " ")
	}
	return strings.Join(lines, "\n")
}

// mkSimpleList creates n rows with 1 column equal to the ID.
func mkSimpleList(n int) *SliceList {
	rows := make([]Row, 0, n)
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("id-%04d", i)
		r := SimpleRow{ID: id}
		r.SetColumn(0, id, nil)
		rows = append(rows, r)
	}
	return NewSliceList(rows)
}

// Test a 25x6 table (width x height) with more than 6 rows. Height includes the
// sticky header. Expect 1 header line + 5 body lines. At the bottom, last row is visible.
func TestView_25x6_NoBorders_BottomShowsLastRow(t *testing.T) {
	cols := []Column{{Title: "A", Width: 8}}
	list := mkSimpleList(8)              // rows id-0001..id-0008
	bt := NewBigTable(cols, list, 25, 6) // 6 lines total (1 sticky header + 5 rows)
    bt.SetMode(ModeScroll)

	// Move to bottom and rebuild
	bt.cursor = list.Len() - 1 // id-0008
	bt.rebuildWindow()

	got := trimRightEachLine(stripANSI(bt.View()))
	// Expect header + last 5 rows
	want := "" +
		"A\n" +
		"id-0004\n" +
		"id-0005\n" +
		"id-0006\n" +
		"id-0007\n" +
		"id-0008"
	if got != want {
		t.Fatalf("unexpected view\nwant:\n%s\n---\ngot:\n%s", want, got)
	}
}

// Same geometry, but with outside border only. Expect a framed body with a
// bottom border visible on the last line.
func TestView_25x6_OutsideOnly_BottomBorderVisible(t *testing.T) {
	cols := []Column{{Title: "A", Width: 8}}
	list := mkSimpleList(8)
	bt := NewBigTable(cols, list, 25, 6)
    bt.SetMode(ModeScroll)
    // outside only: header top + body bottom + left/right
    bt.BorderTop(true).BorderBottom(true).BorderLeft(true).BorderRight(true)

	bt.cursor = list.Len() - 1
	bt.rebuildWindow()

	got := trimRightEachLine(stripANSI(bt.View()))

	// Exact expected layout at 25x6, outside border only: top border + 1 header + 3 rows + bottom border
	want := "" +
		"┌───────────────────────┐\n" +
		"│A                      │\n" +
		"│id-0006                │\n" +
		"│id-0007                │\n" +
		"│id-0008                │\n" +
		"└───────────────────────┘"
	if got != want {
		t.Fatalf("unexpected view\nwant:\n%s\n---\ngot:\n%s", want, got)
	}
}
