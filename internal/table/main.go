package main

import (
    "fmt"
    "math/rand"
    "os"
    "strings"
    "time"

    table "github.com/charmbracelet/bubbles/v2/table"
    tea "github.com/charmbracelet/bubbletea/v2"
    "github.com/charmbracelet/lipgloss/v2"
)

type app struct{ bt BigTable }

func newApp(provider string) app {
    cols := make([]table.Column, 20)
    for c := 0; c < 20; c++ { cols[c] = table.Column{Title: fmt.Sprintf("Col%02d", c+1), Width: 18} }
    // Build a demo List provider with ASCII cells and per-cell styles.
    base := makeBaseRows(1000, 20) // ASCII text only
    colStyles := []*lipgloss.Style{
        ptrStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#93C5FD"))), // ID
        ptrStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))), // Status (value-specific override below)
        ptrStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA"))),
        ptrStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#60A5FA"))),
        ptrStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))),
        ptrStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24"))),
        ptrStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#F472B6"))),
    }
    rows := make([]Row, 0, len(base))
    for i := range base {
        cells := base[i]
        // Build styles per cell; override status column based on value.
        styles := make([]*lipgloss.Style, len(cells))
        for c := range cells {
            var st *lipgloss.Style
            if c < len(colStyles) { st = colStyles[c] }
            if c == 1 { // status column override
                switch cells[c] {
                case "ERROR":
                    st = ptrStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")))
                case "WARN":
                    st = ptrStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#EAB308")))
                case "OK":
                    st = ptrStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")))
                }
            }
            if st == nil { s := lipgloss.NewStyle(); st = &s }
            styles[c] = st
        }
        rows = append(rows, SimpleRow{ ID: fmt.Sprintf("id-%04d", i+1), Cells: cells, Styles: styles })
    }
    var list List
    switch strings.ToLower(provider) {
    case "linked", "ll", "dll":
        list = NewLinkedList(rows)
    default:
        list = NewSliceList(rows)
    }
    bt := NewBigTable(cols, list, 100, 28)
    return app{bt: bt}
}

func (a app) Init() tea.Cmd { return nil }

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch v := msg.(type) {
    case tea.KeyMsg:
        switch v.String() {
        case "q", "ctrl+c":
            return a, tea.Quit
        case "m":
            a.bt.ToggleMode()
            return a, nil
        case "i":
            // Insert a new row after current
            if id, ok := a.bt.CurrentID(); ok {
                // Build a sample row
                nr := SimpleRow{ID: fmt.Sprintf("id-%d", time.Now().UnixNano()%1_000_000_000)}
                nr.SetColumn(0, nr.ID, nil)
                nr.SetColumn(1, "OK", nil)
                nr.SetColumn(2, "inserted row", nil)
                switch l := a.bt.GetList().(type) {
                case *SliceList:
                    l.InsertAfter(id, nr)
                    a.bt.SetList(l)
                case *LinkedList:
                    l.InsertAfterID(id, nr)
                    a.bt.SetList(l)
                }
            }
            return a, nil
        case "d", "delete":
            // Remove current row
            if id, ok := a.bt.CurrentID(); ok {
                switch l := a.bt.GetList().(type) {
                case *SliceList:
                    l.RemoveIDs(id)
                    a.bt.SetList(l)
                case *LinkedList:
                    l.RemoveIDs(id)
                    a.bt.SetList(l)
                }
            }
            return a, nil
        case "t":
            // Toggle provider implementation
            src := collectAll(a.bt.GetList())
            if _, isSlice := a.bt.GetList().(*SliceList); isSlice {
                a.bt.SetList(NewLinkedList(src))
            } else {
                a.bt.SetList(NewSliceList(src))
            }
            return a, nil
        }
    case tea.WindowSizeMsg:
        a.bt.SetSize(v.Width-2, v.Height-2)
    }
    c1, c2 := a.bt.Update(msg)
    return a, tea.Batch(c1, c2)
}

func (a app) View() string { return a.bt.View() }

func main() {
    rand.Seed(time.Now().UnixNano())
    provider := ""
    if len(os.Args) > 1 { provider = os.Args[1] }
    if _, err := tea.NewProgram(newApp(provider), tea.WithAltScreen()).Run(); err != nil {
        fmt.Println("error:", err)
    }
}

func makeBaseRows(nRows, nCols int) [][]string {
    rows := make([][]string, nRows)
    for r := 0; r < nRows; r++ {
        row := make([]string, nCols)
        for c := 0; c < nCols; c++ {
            switch c {
            case 0:
                row[c] = fmt.Sprintf("id-%04d", r+1)
            case 1:
                switch {
                case r%15 == 0:
                    row[c] = "ERROR"
                case r%5 == 0:
                    row[c] = "WARN"
                default:
                    row[c] = "OK"
                }
            default:
                row[c] = fmt.Sprintf("row=%04d col=%02d sample colored content that is fairly long", r+1, c+1)
            }
        }
        rows[r] = row
    }
    return rows
}

func ptrStyle(s lipgloss.Style) *lipgloss.Style { return &s }

