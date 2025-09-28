package table

import (
    "strings"
    "testing"

    lg "github.com/charmbracelet/lipgloss/v2"
    lgtable "github.com/charmbracelet/lipgloss/v2/table"
)

// Test that Width(w) on lipgloss/table produces lines that include the frame
// within the target width (i.e., borders + separators are budgeted inside w).
func TestLipglossTableWidthIncludesBorders(t *testing.T) {
    w := 40

    tb := lgtable.New().
        Wrap(false).
        Width(w).
        Height(4)

    // Explicit, predictable border: normal outside + column separators + header underline.
    tb.Border(lg.NormalBorder()).
        BorderTop(true).BorderBottom(true).BorderLeft(true).BorderRight(true).
        BorderColumn(true).
        BorderHeader(true)

    tb.Headers("Col01", "Col02", "Col03")
    tb.Rows(
        []string{"id-0001", "ERROR", "row=0001 sample"},
        []string{"id-0002", "OK", "row=0002 sample"},
        []string{"id-0003", "OK", "row=0003 sample"},
    )

    out := tb.Render()
    lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
    if len(lines) == 0 {
        t.Fatalf("table rendered no lines")
    }
    for i, ln := range lines {
        got := lg.Width(ln)
        if got != w {
            t.Fatalf("line %d width=%d, want=%d; line=%q", i, got, w, ln)
        }
    }
}

