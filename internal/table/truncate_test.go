package table

import (
    "testing"

    "github.com/charmbracelet/lipgloss/v2"
)

func TestAsciiTruncatePadWidth(t *testing.T) {
    src := "abcdefghijklmnopqrstuvwxyz"
    for w := 1; w <= 30; w++ {
        out := asciiTruncatePad(src, w)
        if lipgloss.Width(out) != w {
            t.Fatalf("width=%d got=%d for %q", w, lipgloss.Width(out), out)
        }
    }
}

func TestAsciiTruncatePadShortTail(t *testing.T) {
    if got := asciiTruncatePad("abc", 2); lipgloss.Width(got) != 2 {
        t.Fatalf("expected width 2, got %d", lipgloss.Width(got))
    }
    if got := asciiTruncatePad("", 5); lipgloss.Width(got) != 5 {
        t.Fatalf("expected padding to width 5, got %d", lipgloss.Width(got))
    }
}
