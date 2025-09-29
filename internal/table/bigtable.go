package table

import (
    "strings"

    tea "github.com/charmbracelet/bubbletea/v2"
    "github.com/charmbracelet/lipgloss/v2"
    lgtable "github.com/charmbracelet/lipgloss/v2/table"
)

// GridMode controls how the table renders horizontally.
//   - ModeScroll: automatic column widths (lipgloss.table decides).
//   - ModeFit: constrain columns to fit the total width (no manual slicing).
type GridMode int

const (
    // ModeScroll uses automatic column widths.
    ModeScroll GridMode = iota
    // ModeFit constrains columns to fit the viewport width.
    ModeFit
)

// BigTable is a reusable Bubble Tea component that renders large, dynamic
// tables backed by a List provider using a single lipgloss.table.
// Only inner vertical separators are supported; no outside borders or underline.
type BigTable struct {
    mode GridMode
    w, h int

    cols []Column // base titles (ASCII)
    list List     // data provider

    desired []int // initial width hints

    // selection & windowing
    window     []Row               // rows currently rendered (order matches table rows)
    selected   map[string]struct{} // multi-select set by row ID
    top        int                 // absolute index of top row in provider
    cursor     int                 // absolute cursor index in provider
    focusedID  string              // ID of the currently focused row (for stability across updates)

    // cached rendered table (header + rows)
    bodyRow string

    styles Styles // external styles

    // Only inner vertical separators (no outside borders, no underline)
    bColumn bool
}

// Styles groups all externally configurable styles.
type Styles struct {
    Header   lipgloss.Style
    Selector lipgloss.Style
    Cell     lipgloss.Style
    Border   lipgloss.Style
}

// DefaultStyles returns a set of defaults for the table.
func DefaultStyles() Styles {
    return Styles{
        Header:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Green),
        Selector: lipgloss.NewStyle().Background(lipgloss.Cyan).Foreground(lipgloss.Black),
        Cell:     lipgloss.NewStyle(),
        Border:   lipgloss.NewStyle().Foreground(lipgloss.Yellow),
    }
}

// SetStyles overrides the component styles.
func (m *BigTable) SetStyles(s Styles) { m.styles = s }

// NewBigTable constructs a table with the given columns, data provider and
// initial size (content width and height). Titles are treated as plain ASCII.
func NewBigTable(cols []Column, list List, w, h int) BigTable {
    desired := make([]int, len(cols))
    for i := range cols {
        if cols[i].Width <= 0 {
            cols[i].Width = 14
        }
        desired[i] = cols[i].Width
    }

    bt := BigTable{
        mode:       ModeScroll,
        w:          max(20, w),
        h:          max(6, h),
        cols:       append([]Column(nil), cols...),
        list:       list,
        desired:    desired,
        selected:   make(map[string]struct{}),
        top:        0,
        cursor:     0,
        focusedID:  "",
        styles:     DefaultStyles(),
        bColumn:    false,
    }
    bt.applyMode()
    return bt
}

// SetSize updates the component size (content area). Width/height are clamped
// to sensible minimums and trigger a relayout.
func (m *BigTable) SetSize(w, h int) {
    if w < 20 {
        w = 20
    }
    if h < 6 {
        h = 6
    }
    m.w, m.h = w, h
    m.applyMode()
}

// --- Border configuration (only inner verticals) ---

// BorderVertical toggles vertical separators between columns (inner verticals).
func (m *BigTable) BorderVertical(v bool) *BigTable {
    m.bColumn = v
    m.rebuildWindow()
    return m
}

// SetMode switches between ModeScroll and ModeFit and refreshes the view.
func (m *BigTable) SetMode(md GridMode) {
    if m.mode != md {
        m.mode = md
        m.applyMode()
    }
}

// ToggleMode flips the current GridMode.
func (m *BigTable) ToggleMode() {
    if m.mode == ModeScroll {
        m.SetMode(ModeFit)
    } else {
        m.SetMode(ModeScroll)
    }
}

// SetList swaps the data provider and repositions the cursor based on the
// previously focused row ID. If that row disappeared, the cursor moves to the
// next row, or previous when no next exists.
func (m *BigTable) SetList(list List) {
    m.list = list
    m.repositionOnDataChange()
    m.rebuildWindow()
}

// GetList returns the current data provider.
func (m *BigTable) GetList() List { return m.list }

// CurrentID returns the focused row ID, if any.
func (m *BigTable) CurrentID() (string, bool) {
    if row := m.list.Lines(m.cursor, 1); len(row) == 1 {
        id, _, _, ok := row[0].Columns()
        return id, ok
    }
    return "", false
}

// Select moves the focus to the row with the given ID, if present, and
// adjusts the window so the focused row is visible. Returns true if found.
func (m *BigTable) Select(id string) bool {
    n := m.list.Len()
    if n <= 0 {
        return false
    }
    // Find absolute index of the row with the given ID by scanning in chunks.
    step := 256
    found := -1
    for off := 0; off < n; {
        take := step
        if off+take > n {
            take = n - off
        }
        rows := m.list.Lines(off, take)
        for i, r := range rows {
            rid, _, _, ok := r.Columns()
            if ok && rid == id {
                found = off + i
                break
            }
        }
        if found >= 0 {
            break
        }
        off += take
    }
    if found < 0 {
        return false
    }
    m.cursor = found
    // Ensure the focused row is visible within the current window height.
    vis := m.bodyRowsHeight()
    if m.cursor < m.top {
        m.top = m.cursor
    } else if m.cursor >= m.top+vis {
        m.top = max(0, m.cursor-(vis-1))
    }
    m.rebuildWindow()
    return true
}

// Update handles key navigation and selection toggling; it also forwards other
// messages to the internal bubbles components. It returns a batchable pair of
// commands for external composition.
func (m *BigTable) Update(msg tea.Msg) (tea.Cmd, tea.Cmd) {
    var c1, c2 tea.Cmd
    switch v := msg.(type) {
    case tea.KeyMsg:
        switch v.String() {
        case "ctrl+t", "insert":
            if row := m.list.Lines(m.cursor, 1); len(row) == 1 {
                id, _, _, _ := row[0].Columns()
                if _, ok := m.selected[id]; ok {
                    delete(m.selected, id)
                } else {
                    m.selected[id] = struct{}{}
                }
                m.refreshRowsOnly()
            }
        case "up", "k":
            if m.cursor > 0 {
                m.cursor--
                if m.cursor < m.top {
                    m.top = m.cursor
                }
                m.rebuildWindow()
            }
        case "down", "j":
            if m.cursor+1 < m.list.Len() {
                m.cursor++
                vis := m.bodyRowsHeight()
                if m.cursor >= m.top+vis {
                    m.top = m.cursor - (vis - 1)
                }
                m.rebuildWindow()
            }
        case "pgup":
            vis := m.bodyRowsHeight()
            if vis < 1 {
                vis = 1
            }
            m.cursor -= vis
            if m.cursor < 0 {
                m.cursor = 0
            }
            if m.cursor < m.top {
                m.top = m.cursor
            }
            m.rebuildWindow()
        case "pgdown":
            vis := m.bodyRowsHeight()
            if vis < 1 {
                vis = 1
            }
            m.cursor += vis
            if m.cursor >= m.list.Len() {
                m.cursor = m.list.Len() - 1
            }
            if m.cursor >= m.top+vis {
                m.top = max(0, m.cursor-(vis-1))
            }
            m.rebuildWindow()
        case "home":
            m.cursor = 0
            m.top = 0
            m.rebuildWindow()
        case "end":
            if n := m.list.Len(); n > 0 {
                m.cursor = n - 1
                vis := m.bodyRowsHeight()
                m.top = max(0, n-vis)
                m.rebuildWindow()
            }
        }
    }
    return c1, c2
}

// View renders the component.
func (m *BigTable) View() string { return strings.TrimRight(m.bodyRow, "\n") }

// no app header/help line inside BigTable; outer app should render it
// no BigTable footer; outer app should render any footers/help lines

// refreshRowsOnly re-renders rows for the current mode without recomputing widths.
func (m *BigTable) refreshRowsOnly() { m.rebuildWindow() }

// rebuildWindow sets the table rows to the current window [top:top+height)
// and positions the table cursor at (cursor-top), updating the width cache.
func (m *BigTable) rebuildWindow() {
    // Compute how many data rows are visible and the total body height.
    rowsVisible := m.bodyRowsHeight()
    // rowsVisible already accounts for header and border lines.
    n := m.list.Len()
    if n < 0 {
        n = 0
    }
    if m.cursor >= n {
        m.cursor = max(0, n-1)
    }
    // rowsVisible already accounts for borders via bodyRowsHeight()
    maxTop := max(0, n-rowsVisible)
    if m.top > maxTop {
        m.top = maxTop
    }
    if m.cursor < m.top {
        m.top = m.cursor
    }
    if m.cursor >= m.top+rowsVisible {
        m.top = max(0, m.cursor-(rowsVisible-1))
    }

    // Render exactly the number of visible rows.
    m.window = m.list.Lines(m.top, rowsVisible)

    // Single lipgloss table: headers + data rows; no outside borders or underline.
    t := lgtable.New().Wrap(false).Height(m.h).Width(m.w).WithOverflowRow(false)
    t = t.Border(lipgloss.NormalBorder()).BorderStyle(m.styles.Border)
    t = t.BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false)
    t = t.BorderHeader(false).BorderRow(false).BorderColumn(m.bColumn)

    // Fit mode is handled by lipgloss.table's Width; no manual slicing.

    headers := make([]string, len(m.cols))
    for i, c := range m.cols {
        headers[i] = c.Title
    }
    t = t.Headers(headers...)
    t = t.Rows(rowsToStringRows(m.window)...)

    // Style cells: base cell style + per-cell style; overlay selection on data rows.
    stylesPerRow := captureStyles(m.window)
    selected := m.selected
    t = t.StyleFunc(func(row, col int) lipgloss.Style {
        if row == lgtable.HeaderRow {
            return m.styles.Header
        }
        if row < 0 || row >= len(stylesPerRow) {
            return lipgloss.NewStyle()
        }
        st := m.styles.Cell
        if col < len(stylesPerRow[row]) && stylesPerRow[row][col] != nil {
            st = (*stylesPerRow[row][col]).Inherit(st)
        }
        id, _, _, _ := m.window[row].Columns()
        if row == (m.cursor-m.top) {
            st = m.styles.Selector.Inherit(st)
        }
        if _, ok := selected[id]; ok {
            st = m.styles.Selector.Inherit(st)
        }
        return st
    })

    m.bodyRow = strings.TrimRight(t.Render(), "\n")
    // Track focused ID for stability across updates.
    if row := m.list.Lines(m.cursor, 1); len(row) == 1 {
        id, _, _, ok := row[0].Columns()
        if ok {
            m.focusedID = id
        }
    }
}

func (m *BigTable) applyMode() { m.rebuildWindow() }

// bodyRowsHeight returns the number of data rows visible within the viewport
// after subtracting sticky header lines.
func (m *BigTable) bodyRowsHeight() int {
    // Reserve exactly one line for the table header; the rest is body space.
    rows := m.h - 1
    if rows < 1 {
        rows = 1
    }
    return rows
}

// (no custom column width allocator; Fit relies on lipgloss.table sizing)

// rowsToStringRows converts []Row to [][]string for lipgloss table.
func rowsToStringRows(rows []Row) [][]string {
    out := make([][]string, len(rows))
    for i := range rows {
        _, cells, _, _ := rows[i].Columns()
        out[i] = cells
    }
    return out
}

// captureStyles extracts per-cell styles for visible rows to use in StyleFunc.
func captureStyles(rows []Row) [][]*lipgloss.Style {
    out := make([][]*lipgloss.Style, len(rows))
    for i := range rows {
        _, _, styles, _ := rows[i].Columns()
        out[i] = styles
    }
    return out
}

// repositionOnDataChange keeps the cursor stable by ID. If the previous
// focused row vanished, move to the next row; if none, to the previous; else
// clamp to bounds.
func (m *BigTable) repositionOnDataChange() {
    n := m.list.Len()
    if n <= 0 {
        m.cursor, m.top, m.focusedID = 0, 0, ""
        m.selected = map[string]struct{}{}
        return
    }
    // Prune selection for rows that disappeared.
    for id := range m.selected {
        if _, _, ok := m.list.Find(id); !ok {
            delete(m.selected, id)
        }
    }

    if m.focusedID == "" {
        if m.cursor >= n {
            m.cursor = n - 1
        }
        if m.cursor < 0 {
            m.cursor = 0
        }
        return
    }
    if idx, _, ok := m.list.Find(m.focusedID); ok {
        m.cursor = idx
        return
    }
    if below := m.list.Below(m.focusedID, 1); len(below) > 0 {
        if id, _, _, ok := below[0].Columns(); ok {
            if idx, _, ok := m.list.Find(id); ok {
                m.cursor = idx
                m.focusedID = id
                return
            }
        }
    }
    if above := m.list.Above(m.focusedID, 1); len(above) > 0 {
        if id, _, _, ok := above[len(above)-1].Columns(); ok {
            if idx, _, ok := m.list.Find(id); ok {
                m.cursor = idx
                m.focusedID = id
                return
            }
        }
    }
    if m.cursor >= n {
        m.cursor = n - 1
    }
    if m.cursor < 0 {
        m.cursor = 0
    }
}

// small helpers
func max(a, b int) int {
    if a > b { return a }
    return b
}
