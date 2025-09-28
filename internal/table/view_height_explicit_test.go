package table

import (
    "fmt"
    "strings"
    "testing"

    "github.com/charmbracelet/lipgloss/v2"
)

// No ANSI stripping: tests set neutral styles to avoid color codes.

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

    // Neutral styles to avoid ANSI escapes
    st := DefaultStyles()
    st.Header = lipgloss.NewStyle()
    st.Cell = lipgloss.NewStyle()
    st.Selector = lipgloss.NewStyle()
    st.Border = lipgloss.NewStyle()
    bt.SetStyles(st)

    // Move to bottom and rebuild
    bt.cursor = list.Len() - 1 // id-0008
    bt.rebuildWindow()

    got := trimRightEachLine(bt.View())
    // Expect header + last 5 rows
    want := `A
id-0004
id-0005
id-0006
id-0007
id-0008`
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

// With vertical column separators only (no outside borders, no underline),
// height is the same as borderless: header + 5 body rows at 25x6.
func TestView_25x6_VerticalOnly_LastRowVisible(t *testing.T) {
    cols := []Column{{Title: "A", Width: 8}}
    list := mkSimpleList(8)
    bt := NewBigTable(cols, list, 25, 6)
    bt.SetMode(ModeScroll)
    bt.BorderColumn(true)

    // Move to bottom
    // Neutral styles to avoid ANSI escapes
    st := DefaultStyles()
    st.Header = lipgloss.NewStyle()
    st.Cell = lipgloss.NewStyle()
    st.Selector = lipgloss.NewStyle()
    st.Border = lipgloss.NewStyle()
    bt.SetStyles(st)

    bt.cursor = list.Len() - 1
    bt.rebuildWindow()

    got := trimRightEachLine(bt.View())
    want := `A
id-0004
id-0005
id-0006
id-0007
id-0008`
    if got != want {
        t.Fatalf("unexpected view\nwant:\n%s\n---\ngot:\n%s", want, got)
    }
}

// Header underline only consumes one extra line. At 25x6 expect:
// header + underline + 4 body rows, last row visible at bottom.
func TestView_25x6_HeaderUnderline_LastRowVisible(t *testing.T) {
    cols := []Column{{Title: "A", Width: 8}}
    list := mkSimpleList(8)
    bt := NewBigTable(cols, list, 25, 6)
    bt.SetMode(ModeScroll)
    bt.BorderHeader(true)

    bt.cursor = list.Len() - 1
    bt.rebuildWindow()

    // Neutral styles to avoid ANSI escapes
    st := DefaultStyles()
    st.Header = lipgloss.NewStyle()
    st.Cell = lipgloss.NewStyle()
    st.Selector = lipgloss.NewStyle()
    st.Border = lipgloss.NewStyle()
    bt.SetStyles(st)

    got := trimRightEachLine(bt.View())
    want := `A
─────────────────────────
id-0005
id-0006
id-0007
id-0008`
    if got != want {
        t.Fatalf("unexpected view\nwant:\n%s\n---\ngot:\n%s", want, got)
    }
}

// Verticals + header underline: still header + underline + 4 body rows at 25x6.
func TestView_25x6_VerticalsAndUnderline_LastRowVisible(t *testing.T) {
    cols := []Column{{Title: "A", Width: 8}}
    list := mkSimpleList(8)
    bt := NewBigTable(cols, list, 25, 6)
    bt.SetMode(ModeScroll)
    bt.BorderColumn(true).BorderHeader(true)

    bt.cursor = list.Len() - 1
    bt.rebuildWindow()

    // Neutral styles to avoid ANSI escapes
    st := DefaultStyles()
    st.Header = lipgloss.NewStyle()
    st.Cell = lipgloss.NewStyle()
    st.Selector = lipgloss.NewStyle()
    st.Border = lipgloss.NewStyle()
    bt.SetStyles(st)

    got := trimRightEachLine(bt.View())
    want := `A
─────────────────────────
id-0005
id-0006
id-0007
id-0008`
    if got != want {
        t.Fatalf("unexpected view\nwant:\n%s\n---\ngot:\n%s", want, got)
    }
}

// Outside + verticals + header underline: at 25x6 expect top border, header,
// header rule, then 2 body rows, then bottom border.
func TestView_25x6_Outside_Verticals_Header_LastRowsVisible(t *testing.T) {
    cols := []Column{{Title: "A", Width: 8}}
    list := mkSimpleList(8)
    bt := NewBigTable(cols, list, 25, 6)
    bt.SetMode(ModeScroll)
    bt.BorderTop(true).BorderBottom(true).BorderLeft(true).BorderRight(true).BorderColumn(true).BorderHeader(true)

    bt.cursor = list.Len() - 1
    bt.rebuildWindow()

    // Neutral styles to avoid ANSI escapes
    st := DefaultStyles()
    st.Header = lipgloss.NewStyle()
    st.Cell = lipgloss.NewStyle()
    st.Selector = lipgloss.NewStyle()
    st.Border = lipgloss.NewStyle()
    bt.SetStyles(st)

    got := trimRightEachLine(bt.View())
    want := `┌───────────────────────┐
│A                      │
├───────────────────────┤
│id-0007                │
│id-0008                │
└───────────────────────┘`
    if got != want {
        t.Fatalf("unexpected view\nwant:\n%s\n---\ngot:\n%s", want, got)
    }
}

// Double outside border variant behaves the same height-wise; verify 2 body rows
// and correct double border glyphs are used.
func TestView_25x6_DoubleOutside_Verticals_Header_LastRowsVisible(t *testing.T) {
    cols := []Column{{Title: "A", Width: 8}}
    list := mkSimpleList(8)
    bt := NewBigTable(cols, list, 25, 6)
    bt.SetMode(ModeScroll)
    bt.Border(lipgloss.DoubleBorder()).
        BorderTop(true).BorderBottom(true).BorderLeft(true).BorderRight(true).BorderColumn(true).BorderHeader(true)

    // Neutral styles to avoid ANSI escapes
    st := DefaultStyles()
    st.Header = lipgloss.NewStyle()
    st.Cell = lipgloss.NewStyle()
    st.Selector = lipgloss.NewStyle()
    st.Border = lipgloss.NewStyle()
    bt.SetStyles(st)

    bt.cursor = list.Len() - 1
    bt.rebuildWindow()

    got := trimRightEachLine(bt.View())
    want := `╔═══════════════════════╗
║A                      ║
╠═══════════════════════╣
║id-0007                ║
║id-0008                ║
╚═══════════════════════╝`
    if got != want {
        t.Fatalf("unexpected view\nwant:\n%s\n---\ngot:\n%s", want, got)
    }
}
