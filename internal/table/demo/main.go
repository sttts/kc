package main

import (
    "fmt"
    "math/rand"
    "os"
    "strings"
    "time"

    tea "github.com/charmbracelet/bubbletea/v2"
    "github.com/charmbracelet/lipgloss/v2"
    table "github.com/sttts/kc/internal/table"
    "github.com/sttts/kc/pkg/appconfig"
)

type app struct{
    left     table.BigTable
    right    table.BigTable
    focus    int // 0=left, 1=right
    bstateL  int
    bstateR  int
    ticks    int
}

func newApp(provider string) app {
    cols := make([]table.Column, 10)
    for c := 0; c < 10; c++ {
        // No fixed widths in the demo; BigTable computes widths from visible data in ModeScroll.
        cols[c] = table.Column{Title: fmt.Sprintf("Col%02d", c+1)}
    }
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
    rows := make([]table.Row, 0, len(base))
    for i := range base {
        cells := base[i]
		// Build styles per cell; override status column based on value.
		styles := make([]*lipgloss.Style, len(cells))
		for c := range cells {
			var st *lipgloss.Style
			if c < len(colStyles) {
				st = colStyles[c]
			}
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
			if st == nil {
				s := lipgloss.NewStyle()
				st = &s
			}
			styles[c] = st
		}
        rows = append(rows, &table.SimpleRow{ID: fmt.Sprintf("id-%04d", i+1), Cells: cells, Styles: styles})
    }
	var list table.List
	switch strings.ToLower(provider) {
	case "linked", "ll", "dll":
		list = table.NewLinkedList(rows)
	default:
		list = table.NewSliceList(rows)
	}
    left := table.NewBigTable(cols, list, 100, 28)
    right := table.NewBigTable(cols, list, 100, 28)
	// Apply Norton Commander-inspired color scheme (table only)
	st := table.DefaultStyles()
	// Classic NC inside the table: light gray text, cyan selection, yellow headers
	st.Cell = st.Cell.Foreground(lipgloss.Color("#C0C0C0")).Background(lipgloss.Color("#0000AA"))
	st.Header = st.Header.Foreground(lipgloss.Color("#FFFF00")).Background(lipgloss.Color("#0000AA"))
	st.Selector = lipgloss.NewStyle().Background(lipgloss.Color("#00AAAA")).Foreground(lipgloss.Color("#000000"))
    left.SetStyles(st)
    right.SetStyles(st)
    // Load config and apply horizontal scroll step if configured.
    if cfg, err := appconfig.Load(); err == nil {
        if cfg.Panel.Scrolling.Horizontal.Step > 0 {
            left.SetHorizontalStep(cfg.Panel.Scrolling.Horizontal.Step)
            right.SetHorizontalStep(cfg.Panel.Scrolling.Horizontal.Step)
        }
    }
    // start with no inner separators (no outside borders are ever rendered)
    left.BorderVertical(false)
    right.BorderVertical(false)
    left.Focus()
    right.Blur()
    return app{left: left, right: right, focus: 0, bstateL: 0, bstateR: 0}
}

func (a app) Init() tea.Cmd {
    // Kick off a 1s tick to drive periodic row updates.
    return tea.Tick(time.Second, func(t time.Time) tea.Msg { return t })
}

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.KeyMsg:
		switch v.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
        case "m":
            cur := &a.left
            if a.focus == 1 { cur = &a.right }
            cur.ToggleMode()
            return a, nil
        case "i":
            // Insert a new row after current
            cur := &a.left
            if a.focus == 1 { cur = &a.right }
            if id, ok := cur.CurrentID(); ok {
                nr := table.SimpleRow{ID: fmt.Sprintf("id-%d", time.Now().UnixNano()%1_000_000_000)}
                nr.SetColumn(0, nr.ID, nil)
                nr.SetColumn(1, "OK", nil)
                nr.SetColumn(2, "inserted row", nil)
                switch l := cur.GetList().(type) {
                case *table.SliceList:
                    l.InsertAfter(id, nr)
                    cur.SetList(l)
                case *table.LinkedList:
                    l.InsertAfterID(id, nr)
                    cur.SetList(l)
                }
            }
            return a, nil
        case "d", "delete":
            // Remove current row
            cur := &a.left
            if a.focus == 1 { cur = &a.right }
            if id, ok := cur.CurrentID(); ok {
                switch l := cur.GetList().(type) {
                case *table.SliceList:
                    l.RemoveIDs(id)
                    cur.SetList(l)
                case *table.LinkedList:
                    l.RemoveIDs(id)
                    cur.SetList(l)
                }
            }
            return a, nil
        case "t":
            // Toggle provider implementation
            cur := &a.left
            if a.focus == 1 { cur = &a.right }
            src := table.LinesToRows(cur.GetList().Lines(0, cur.GetList().Len()))
            if _, isSlice := cur.GetList().(*table.SliceList); isSlice {
                cur.SetList(table.NewLinkedList(src))
            } else {
                cur.SetList(table.NewSliceList(src))
            }
            return a, nil
        case "b":
            // Cycle inner border presets only (no outside frame, no underline)
            // 0: none, 1: verticals
            if a.focus == 0 {
                a.bstateL = (a.bstateL + 1) % 2
                a.left.BorderVertical(a.bstateL == 1)
            } else {
                a.bstateR = (a.bstateR + 1) % 2
                a.right.BorderVertical(a.bstateR == 1)
            }
            return a, nil
        case "tab":
            // Switch focus between left and right tables (outer routes keys)
            if a.focus == 0 {
                a.focus = 1
                a.left.Blur()
                a.right.Focus()
            } else {
                a.focus = 0
                a.right.Blur()
                a.left.Focus()
            }
            return a, nil
        }
    case tea.WindowSizeMsg:
        // Reserve one line for the demo help header
        totalW := v.Width
        bodyH := v.Height - 1
        sep := 1
        lw := (totalW - sep) / 2
        rw := totalW - sep - lw
        a.left.SetSize(lw, bodyH)
        a.right.SetSize(rw, bodyH)
        // keep ticking once per second
        return a, tea.Tick(time.Second, func(t time.Time) tea.Msg { return t })
    }
    var cmds []tea.Cmd
    // Handle 1s tick by time.Time message
    if _, ok := msg.(time.Time); ok {
        // Randomly update ~10% of rows' Col02 status on each table.
        a.randomlyUpdateStatuses(&a.left, 0.10)
        a.randomlyUpdateStatuses(&a.right, 0.10)
        // Schedule next tick
        cmds = append(cmds, tea.Tick(time.Second, func(t time.Time) tea.Msg { return t }))
    }
    if a.focus == 0 {
        if c1, c2 := a.left.Update(msg); c1 != nil || c2 != nil { cmds = append(cmds, c1, c2) }
    } else {
        if c1, c2 := a.right.Update(msg); c1 != nil || c2 != nil { cmds = append(cmds, c1, c2) }
    }
    return a, tea.Batch(cmds...)
}

func (a app) View() string {
    modes := []string{"none", "verticals"}
    cur := modes[a.bstateL]
    if a.focus == 1 { cur = modes[a.bstateR] }
    help := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A8FF60")).Render(
        fmt.Sprintf("Tab: switch | Left/Right | Up/Down | PgUp/PgDn | Home/End | m: mode(Auto/Fit) | b: borders(%s) | i: insert | d/Del: delete | t: provider", cur),
    )
    // Join table views side by side with a space separator.
    leftView := a.left.View()
    rightView := a.right.View()
    body := joinSideBySide(leftView, rightView, " ")
    return strings.Join([]string{help, body}, "\n")
}

// joinSideBySide merges two multi-line strings with a separator between columns.
func joinSideBySide(aStr, bStr, sep string) string {
    aLines := strings.Split(strings.TrimRight(aStr, "\n"), "\n")
    bLines := strings.Split(strings.TrimRight(bStr, "\n"), "\n")
    n := len(aLines)
    if len(bLines) > n { n = len(bLines) }
    out := make([]string, n)
    for i := 0; i < n; i++ {
        var l, r string
        if i < len(aLines) { l = aLines[i] }
        if i < len(bLines) { r = bLines[i] }
        out[i] = l + sep + r
    }
    return strings.Join(out, "\n")
}

func main() {
	rand.Seed(time.Now().UnixNano())
	provider := ""
	if len(os.Args) > 1 {
		provider = os.Args[1]
	}
	if _, err := tea.NewProgram(newApp(provider), tea.WithAltScreen()).Run(); err != nil {
		fmt.Println("error:", err)
	}
}

// randomlyUpdateStatuses updates approx p fraction of rows' status (Col02) to a random value.
// It mutates the underlying SimpleRow in place and refreshes the table view without touching the list.
func (a app) randomlyUpdateStatuses(bt *table.BigTable, p float64) {
    list := bt.GetList()
    n := list.Len()
    if n == 0 || p <= 0 { bt.Refresh(); return }
    count := int(float64(n) * p)
    if count < 1 { count = 1 }
    statuses := []string{"OK", "WARN", "ERROR"}
    for i := 0; i < count; i++ {
        idx := rand.Intn(n)
        rows := list.Lines(idx, 1)
        if len(rows) != 1 { continue }
        if r, ok := rows[0].(*table.SimpleRow); ok {
            val := statuses[rand.Intn(len(statuses))]
            // Optional: color-code style similar to initial demo setup
            var st *lipgloss.Style
            switch val {
            case "ERROR": st = ptrStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")))
            case "WARN":  st = ptrStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#EAB308")))
            case "OK":    st = ptrStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")))
            }
            r.SetColumn(1, val, st)
        }
    }
    bt.Refresh()
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
