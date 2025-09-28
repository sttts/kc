package table

import (
    "testing"
    "github.com/charmbracelet/lipgloss/v2"
)

func TestSimpleRowSetColumnGrowsAndSets(t *testing.T) {
    var r SimpleRow
    st := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0"))
    r.SetColumn(3, "hello", &st)
    if len(r.Cells) < 4 || r.Cells[3] != "hello" {
        t.Fatalf("expected cell 3 to be set, got %#v", r.Cells)
    }
    if len(r.Styles) < 4 || r.Styles[3] == nil {
        t.Fatalf("expected style at 3 to be set")
    }
}

func TestSimpleRowSetColumnKeepsStyleWhenNil(t *testing.T) {
    var r SimpleRow
    st := lipgloss.NewStyle().Foreground(lipgloss.Color("#0f0"))
    r.SetColumn(0, "a", &st)
    r.SetColumn(0, "b", nil)
    if r.Cells[0] != "b" {
        t.Fatalf("expected updated text 'b', got %q", r.Cells[0])
    }
    if r.Styles[0] == nil {
        t.Fatalf("expected style to be retained")
    }
}
