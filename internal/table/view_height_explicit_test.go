package table

import (
    "fmt"
    "strings"
    "testing"

    "github.com/charmbracelet/lipgloss/v2"
)

// trimRightEachLine trims trailing spaces on every line to make visual
// comparisons robust while keeping human‑readable, aligned expectations.
func trimRightEachLine(s string) string {
    lines := strings.Split(s, "\n")
    for i, ln := range lines {
        lines[i] = strings.TrimRight(ln, " ")
    }
    return strings.Join(lines, "\n")
}

// mkTwoColList creates n rows with 2 columns: ID and a fixed-width value.
func mkTwoColList(n int) *SliceList {
    rows := make([]Row, 0, n)
    for i := 1; i <= n; i++ {
        id := fmt.Sprintf("id-%04d", i)
        val := fmt.Sprintf("val-%04d", i)
        r := SimpleRow{ID: id}
        r.SetColumn(0, id, nil)
        r.SetColumn(1, val, nil)
        rows = append(rows, r)
    }
    return NewSliceList(rows)
}

// Expect header + 5 body rows at 25x6, and no vertical separators.
func TestView_25x6_NoBorders(t *testing.T) {
    cols := []Column{{Title: "A", Width: 12}, {Title: "B", Width: 12}}
    list := mkTwoColList(10)
    for _, tc := range []struct {
        name string
        mode GridMode
    }{{"auto", ModeScroll}, {"fit", ModeFit}} {
        t.Run(tc.name, func(t *testing.T) {
            bt := NewBigTable(cols, list, 25, 6)
            st := DefaultStyles()
            st.Header = lipgloss.NewStyle()
            st.Cell = lipgloss.NewStyle()
            st.Selector = lipgloss.NewStyle()
            st.Border = lipgloss.NewStyle()
            bt.SetStyles(st)
            bt.SetMode(tc.mode)
            bt.Select("id-0006")
            got := trimRightEachLine(bt.View())
            lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
            if len(lines) != 6 {
                t.Fatalf("want 6 lines (header+5 body), got %d\n%s", len(lines), got)
            }
            if strings.ContainsRune(lines[0], '│') {
                t.Fatalf("header contains vertical separator but borders disabled: %q", lines[0])
            }
        })
    }
}

// Only vertical column separators (no outer or horizontal lines) with two columns.
// Expect header + 5 body rows at 25x6 and clear column separation.
func TestView_25x6_VerticalOnly_TwoColumns(t *testing.T) {
    cols := []Column{{Title: "A", Width: 12}, {Title: "B", Width: 12}}
    list := mkTwoColList(10)
    for _, tc := range []struct {
        name string
        mode GridMode
    }{{"auto", ModeScroll}, {"fit", ModeFit}} {
        t.Run(tc.name, func(t *testing.T) {
            bt := NewBigTable(cols, list, 25, 6)
            bt.BorderVertical(true)
            st := DefaultStyles()
            st.Header = lipgloss.NewStyle()
            st.Cell = lipgloss.NewStyle()
            st.Selector = lipgloss.NewStyle()
            st.Border = lipgloss.NewStyle()
            bt.SetStyles(st)
            bt.SetMode(tc.mode)
            bt.Select("id-0008")
            got := trimRightEachLine(bt.View())
            lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
            if len(lines) != 6 {
                t.Fatalf("want 6 lines (header+5 body), got %d\n%s", len(lines), got)
            }
            if !strings.ContainsRune(lines[0], '│') {
                t.Fatalf("header lacks vertical separator with vertical borders enabled: %q", lines[0])
            }
        })
    }
}

