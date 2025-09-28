package main

import (
    "strings"

    table "github.com/charmbracelet/bubbles/v2/table"
    viewport "github.com/charmbracelet/bubbles/v2/viewport"
    tea "github.com/charmbracelet/bubbletea/v2"
    "github.com/charmbracelet/lipgloss/v2"
    "github.com/muesli/reflow/truncate"
)

type GridMode int

const (
    ModeScroll GridMode = iota // viewport pans; columns wide enough for full plain text
    ModeFit                    // pre-truncate plain text to fit; then style
)

type BigTable struct {
    vp    viewport.Model
    tbl   table.Model
    mode  GridMode
    w, h  int

    cols []table.Column // base titles (ASCII)
    list List           // data provider

    desired []int // initial width hints

    // selection & windowing
    window     []Row               // rows currently rendered in the table (order matches tbl rows)
    selected   map[string]struct{} // multi-select set by row ID
    selStyle   lipgloss.Style      // selection style applied over cell styles
    top        int                 // absolute index of top row in provider
    cursor     int                 // absolute cursor index in provider
    focusedID  string              // ID of the currently focused row (for stability across updates)
    widthCache []int               // incremental max width cache for columns
}

func NewBigTable(cols []table.Column, list List, w, h int) BigTable {
    desired := make([]int, len(cols))
    for i := range cols {
        if cols[i].Width <= 0 { cols[i].Width = 14 }
        desired[i] = cols[i].Width
    }

    // Prime with empty rows; content will be set in applyMode/sync via provider.
    t := table.New(table.WithColumns(cols), table.WithRows(nil), table.WithFocused(true))
    t.SetHeight(max(10, h-2))

    st := table.DefaultStyles()
    st.Header = lipgloss.NewStyle().Bold(true) // no border
    st.Cell = lipgloss.NewStyle()              // exact widths (no padding)
    st.Selected = lipgloss.NewStyle().Background(lipgloss.Color("12")).Foreground(lipgloss.Color("0"))
    t.SetStyles(st)

    vp := viewport.New(viewport.WithWidth(max(20, w)), viewport.WithHeight(max(6, h)))
    vp.SoftWrap = false
    vp.SetHorizontalStep(8)

    bt := BigTable{
        vp:         vp,
        tbl:        t,
        mode:       ModeScroll,
        w:          max(20, w),
        h:          max(6, h),
        cols:       append([]table.Column(nil), cols...),
        list:       list,
        desired:    desired,
        selected:   make(map[string]struct{}),
        selStyle:   lipgloss.NewStyle().Background(lipgloss.Color("12")).Foreground(lipgloss.Color("0")),
        top:        0,
        cursor:     0,
        focusedID:  "",
        widthCache:  initWidthCache(cols),
    }
    bt.applyMode()
    bt.sync()
    return bt
}

func (m *BigTable) SetSize(w, h int) {
    if w < 20 { w = 20 }
    if h < 6  { h = 6 }
    m.w, m.h = w, h
    m.vp.SetWidth(w); m.vp.SetHeight(h)
    m.tbl.SetHeight(h-2)
    m.applyMode()
    m.sync()
}

func (m *BigTable) SetMode(md GridMode) { if m.mode != md { m.mode = md; m.applyMode(); m.sync() } }
func (m *BigTable) ToggleMode()         { if m.mode == ModeScroll { m.SetMode(ModeFit) } else { m.SetMode(ModeScroll) } }

// SetList swaps the data provider and repositions the cursor according to
// the focused row ID. If the focused row disappeared, the cursor moves to the
// next row; if none, to the previous; otherwise it clamps within bounds.
func (m *BigTable) SetList(list List) {
    m.list = list
    m.repositionOnDataChange()
    m.rebuildWindow()
}

// GetList exposes the current data provider (for demo mutations).
func (m *BigTable) GetList() List { return m.list }

// CurrentID returns the focused row ID, if any.
func (m *BigTable) CurrentID() (string, bool) {
    if row := m.list.Lines(m.cursor, 1); len(row) == 1 {
        id, _, _, ok := row[0].Columns(); return id, ok
    }
    return "", false
}

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
                if m.cursor < m.top { m.top = m.cursor }
                m.rebuildWindow()
            }
        case "down", "j":
            if m.cursor+1 < m.list.Len() {
                m.cursor++
                h := m.tbl.Height(); if h <= 0 { h = m.h-2 }
                if m.cursor >= m.top+h { m.top = m.cursor - (h - 1) }
                m.rebuildWindow()
            }
        case "pgup":
            h := m.tbl.Height(); if h <= 0 { h = m.h-2 }
            if h < 1 { h = 1 }
            m.cursor -= h; if m.cursor < 0 { m.cursor = 0 }
            if m.cursor < m.top { m.top = m.cursor }
            m.rebuildWindow()
        case "pgdown":
            h := m.tbl.Height(); if h <= 0 { h = m.h-2 }
            if h < 1 { h = 1 }
            m.cursor += h; if m.cursor >= m.list.Len() { m.cursor = m.list.Len()-1 }
            if m.cursor >= m.top+h { m.top = max(0, m.cursor-(h-1)) }
            m.rebuildWindow()
        case "home":
            m.cursor = 0; m.top = 0; m.rebuildWindow()
        case "end":
            if n := m.list.Len(); n > 0 {
                m.cursor = n-1
                h := m.tbl.Height(); if h <= 0 { h = m.h-2 }
                m.top = max(0, n-h)
                m.rebuildWindow()
            }
        }
    }
    m.vp, c1 = m.vp.Update(msg)
    // Avoid forwarding movement keys—we control cursor explicitly.
    switch v := msg.(type) {
    case tea.KeyMsg:
        switch v.String() {
        case "up", "k", "down", "j", "pgup", "pgdown", "home", "end", "ctrl+t", "insert":
            // handled
        default:
            m.tbl, c2 = m.tbl.Update(msg)
        }
    default:
        m.tbl, c2 = m.tbl.Update(msg)
    }
    m.sync()
    return c1, c2
}

func (m *BigTable) View() string {
    header := headerStyle.Render(m.header())
    footer := footerStyle.Render(m.footer())
    return outerStyle.Render(strings.Join([]string{header, m.vp.View(), footer}, "\n"))
}

func (m *BigTable) header() string {
    if m.mode == ModeScroll { return "Left/Right horizontal | Up/Down | PgUp/PgDn | Home/End | m: FIT | i: insert | d/Del: delete | t: toggle provider" }
    return "FIT MODE (no horizontal scroll) | Up/Down | PgUp/PgDn | Home/End | m: SCROLL | i: insert | d/Del: delete | t: toggle provider"
}
func (m *BigTable) footer() string {
    if m.mode == ModeScroll { return "Columns sized to full plain content; use Left/Right to pan" }
    return "Columns truncated (ASCII ...) to fit; then styled"
}

func (m *BigTable) sync() { m.vp.SetContent(m.tbl.View()) }

// refreshRowsOnly re-renders rows for the current mode without recomputing widths.
func (m *BigTable) refreshRowsOnly() { m.rebuildWindow() }

// rebuildWindow sets the table rows to the current window [top:top+height)
// and positions the table cursor at (cursor-top), updating the width cache.
func (m *BigTable) rebuildWindow() {
    h := m.tbl.Height(); if h <= 0 { h = m.h - 2 }
    if h < 1 { h = 1 }
    n := m.list.Len()
    if n < 0 { n = 0 }
    if m.cursor >= n { m.cursor = max(0, n-1) }
    maxTop := max(0, n-h)
    if m.top > maxTop { m.top = maxTop }
    if m.cursor < m.top { m.top = m.cursor }
    if m.cursor >= m.top+h { m.top = max(0, m.cursor-(h-1)) }

    m.window = m.list.Lines(m.top, h)
    // Update width cache from visible rows (plain ASCII cells)
    for _, row := range m.window {
        _, cells, _, _ := row.Columns()
        for i := 0; i < len(m.cols) && i < len(cells); i++ {
            if w := lipgloss.Width(cells[i]); w > m.widthCache[i] { m.widthCache[i] = w }
        }
    }
    var rows []table.Row
    if m.mode == ModeFit {
        cols := m.tbl.Columns()
        target := make([]int, len(cols))
        for i := range cols { target[i] = cols[i].Width }
        rows = renderRowsFromSlice(truncateRows(m.window, target), m.selected)
    } else {
        rows = renderRowsFromSlice(m.window, m.selected)
    }
    m.tbl.SetRows(rows)
    m.tbl.SetCursor(m.cursor - m.top)
    // Track focused ID for stability across updates.
    if row := m.list.Lines(m.cursor, 1); len(row) == 1 {
        id, _, _, ok := row[0].Columns()
        if ok { m.focusedID = id }
    }
}

func (m *BigTable) applyMode() {
    switch m.mode {
    case ModeScroll:
        // Use incremental width cache seeded by header titles
        widths := make([]int, len(m.cols))
        for i := range widths { widths[i] = max(m.widthCache[i], m.desired[i]) }

        cols := make([]table.Column, len(m.cols))
        for i, c := range m.cols { c.Width = widths[i]; cols[i] = c }
        m.tbl.SetColumns(cols)
        m.tbl.SetWidth(sum(widths))

        m.rebuildWindow()

    case ModeFit:
        desired := make([]int, len(m.cols))
        for i := range desired { desired[i] = max(m.widthCache[i], m.desired[i]) }
        target := computeFitWidths(m.w, desired, 3)

        // Truncate titles (plain) first; rows are truncated per-window in rebuild.
        trTitles := make([]table.Column, len(m.cols))
        for i, c := range m.cols {
            c.Title = asciiTruncatePad(c.Title, target[i])
            c.Width = target[i]
            trTitles[i] = c
        }
        m.tbl.SetColumns(trTitles)
        m.tbl.SetWidth(m.w)
        m.rebuildWindow()
    }
}

// --- sizing helpers (measure only plain ASCII) ---

func measurePlainWidthsFromProvider(cols []table.Column, list List) []int { // retained for reference/tests
    n := len(cols)
    w := make([]int, n)
    for i := 0; i < n; i++ { w[i] = lipgloss.Width(cols[i].Title) }
    capScan := 2000
    remaining := list.Len()
    if remaining > capScan { remaining = capScan }
    offset := 0
    step := 256
    for remaining > 0 {
        take := step
        if take > remaining { take = remaining }
        for _, row := range list.Lines(offset, take) {
            _, cells, _, _ := row.Columns()
            for i := 0; i < n && i < len(cells); i++ {
                if cw := lipgloss.Width(cells[i]); cw > w[i] { w[i] = cw }
            }
        }
        offset += take
        remaining -= take
    }
    return w
}

func computeFitWidths(total int, desired []int, minCol int) []int {
    n := len(desired); if n == 0 { return nil }
    if minCol < 1 { minCol = 1 }
    sumDesired := 0
    for _, d := range desired { if d < minCol { d = minCol }; sumDesired += d }
    if sumDesired <= total {
        out := make([]int, n)
        for i, d := range desired { out[i] = max(d, minCol) }
        return out
    }
    out := make([]int, n)
    base := 0
    for i, d := range desired {
        if d < minCol { d = minCol }
        q := d * total / sumDesired
        if q < minCol { q = minCol }
        out[i] = q
        base += q
    }
    rem := total - base
    for rem > 0 {
        for i := range out {
            if rem == 0 { break }
            out[i]++
            rem--
        }
    }
    return out
}

// --- truncate (plain ASCII) then style ---

func asciiTruncatePad(s string, w int) string {
    if w <= 0 { return "" }
    if lipgloss.Width(s) <= w {
        if pad := w - lipgloss.Width(s); pad > 0 { return s + strings.Repeat(" ", pad) }
        return s
    }
    if w <= 3 { return strings.Repeat(".", w) }
    out := truncate.StringWithTail(s, uint(w), "...") // ASCII tail only
    if pad := w - lipgloss.Width(out); pad > 0 { out += strings.Repeat(" ", pad) }
    return out
}

func renderRowsFromSlice(src []Row, selected map[string]struct{}) []table.Row {
    out := make([]table.Row, len(src))
    for r := range src {
        id, cells, styles, _ := src[r].Columns()
        rendered := make([]string, len(cells))
        for c := range cells {
            var st *lipgloss.Style
            if c < len(styles) { st = styles[c] }
            if st == nil { s := lipgloss.NewStyle(); st = &s }
            s := st.Render(cells[c])
            if _, ok := selected[id]; ok {
                s = mselStyle().Render(s)
            }
            rendered[c] = s
        }
        out[r] = table.Row(rendered)
    }
    return out
}

func truncateRows(rows []Row, target []int) []Row {
    tr := make([]Row, len(rows))
    for r := range rows {
        id, cells, styles, _ := rows[r].Columns()
        truncated := make([]string, len(target))
        for c := range target {
            s := ""
            if c < len(cells) { s = cells[c] }
            truncated[c] = asciiTruncatePad(s, target[c])
        }
        tr[r] = SimpleRow{ ID: id, Cells: truncated, Styles: styles }
    }
    return tr
}

// mselStyle returns the global selection style used when overlaying selected rows.
func mselStyle() lipgloss.Style { return lipgloss.NewStyle().Background(lipgloss.Color("12")).Foreground(lipgloss.Color("0")) }

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
        if m.cursor >= n { m.cursor = n-1 }
        if m.cursor < 0 { m.cursor = 0 }
        return
    }
    if idx, _, ok := m.list.Find(m.focusedID); ok {
        m.cursor = idx
        return
    }
    if below := m.list.Below(m.focusedID, 1); len(below) > 0 {
        if id, _, _, ok := below[0].Columns(); ok {
            if idx, _, ok := m.list.Find(id); ok { m.cursor = idx; m.focusedID = id; return }
        }
    }
    if above := m.list.Above(m.focusedID, 1); len(above) > 0 {
        if id, _, _, ok := above[len(above)-1].Columns(); ok {
            if idx, _, ok := m.list.Find(id); ok { m.cursor = idx; m.focusedID = id; return }
        }
    }
    if m.cursor >= n { m.cursor = n-1 }
    if m.cursor < 0 { m.cursor = 0 }
}

// --- styles for demo rendering ---
var (
    outerStyle  = lipgloss.NewStyle().Padding(0, 1)
    headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8AFF80"))
    footerStyle = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#7D7D7D"))
)

// --- utilities ---

func collectAll(list List) []Row { return list.Lines(0, list.Len()) }

func sum(xs []int) int { s := 0; for _, v := range xs { s += v }; return s }
func max(a, b int) int { if a > b { return a }; return b }

func initWidthCache(cols []table.Column) []int {
    out := make([]int, len(cols))
    for i := range cols { out[i] = lipgloss.Width(cols[i].Title) }
    return out
}

