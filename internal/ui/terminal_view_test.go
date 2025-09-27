package ui

import (
    "strings"
    "testing"
    tea "github.com/charmbracelet/bubbletea/v2"
)

// trimRight trims only spaces on the right to ignore lipgloss width padding in comparisons.
func trimRight(s string) string { return strings.TrimRight(s, " ") }

func TestRenderTwoLineFrom_NilCursor_LastTwoLines(t *testing.T) {
    view := "L0\nL1\nL2"
    out, cur := renderTwoLineFrom(view, nil, 20)
    if cur != nil {
        t.Fatalf("expected nil cursor, got %+v", cur)
    }
    lines := strings.Split(out, "\n")
    if len(lines) != 2 { t.Fatalf("expected 2 lines, got %d", len(lines)) }
    if got, want := trimRight(lines[0]), "L1"; got != want { t.Errorf("line0=%q want %q", got, want) }
    if got, want := trimRight(lines[1]), "L2"; got != want { t.Errorf("line1=%q want %q", got, want) }
}

func TestRenderTwoLineFrom_ClampCursorBeyondEnd(t *testing.T) {
    view := "A\nB\nC"
    c := tea.NewCursor(0, 10)
    out, cur := renderTwoLineFrom(view, c, 20)
    if cur == nil { t.Fatalf("expected cursor, got nil") }
    if cur.Y != 1 { t.Errorf("expected adjusted cursor Y=1, got %d", cur.Y) }
    lines := strings.Split(out, "\n")
    if len(lines) != 2 { t.Fatalf("expected 2 lines, got %d", len(lines)) }
    if got, want := trimRight(lines[0]), "B"; got != want { t.Errorf("line0=%q want %q", got, want) }
    if got, want := trimRight(lines[1]), "C"; got != want { t.Errorf("line1=%q want %q", got, want) }
}

func TestRenderTwoLineFrom_CursorAtZero(t *testing.T) {
    view := "X\nY"
    c := tea.NewCursor(0, 0)
    out, cur := renderTwoLineFrom(view, c, 20)
    if cur == nil { t.Fatalf("expected cursor, got nil") }
    if cur.Y != 1 { t.Errorf("expected adjusted cursor Y=1, got %d", cur.Y) }
    lines := strings.Split(out, "\n")
    if len(lines) != 2 { t.Fatalf("expected 2 lines, got %d", len(lines)) }
    if got, want := trimRight(lines[0]), ""; got != want { t.Errorf("line0=%q want %q", got, want) }
    if got, want := trimRight(lines[1]), "X"; got != want { t.Errorf("line1=%q want %q", got, want) }
}

func TestRenderTwoLineFrom_SingleLineClamp(t *testing.T) {
    view := "Only"
    c := tea.NewCursor(0, 99)
    out, cur := renderTwoLineFrom(view, c, 20)
    if cur == nil { t.Fatalf("expected cursor, got nil") }
    lines := strings.Split(out, "\n")
    if len(lines) != 2 { t.Fatalf("expected 2 lines, got %d", len(lines)) }
    if got, want := trimRight(lines[0]), ""; got != want { t.Errorf("line0=%q want %q", got, want) }
    if got, want := trimRight(lines[1]), "Only"; got != want { t.Errorf("line1=%q want %q", got, want) }
}

func TestRenderTwoLineFrom_EmptyView(t *testing.T) {
    view := ""
    out, cur := renderTwoLineFrom(view, nil, 10)
    if cur != nil { t.Fatalf("expected nil cursor, got %+v", cur) }
    lines := strings.Split(out, "\n")
    if len(lines) != 2 { t.Fatalf("expected 2 lines, got %d", len(lines)) }
    if got, want := trimRight(lines[0]), ""; got != want { t.Errorf("line0=%q want %q", got, want) }
    if got, want := trimRight(lines[1]), ""; got != want { t.Errorf("line1=%q want %q", got, want) }
}

