package table

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/lipgloss/v2/ansi"
	lgtable "github.com/charmbracelet/lipgloss/v2/table"
	"github.com/muesli/reflow/truncate"
)

// GridMode controls how the table renders horizontally.
//   - ModeScroll: no pre-truncation; horizontally pannable content.
//   - ModeFit: truncate ASCII to the available width, then apply styles.
type GridMode int

const (
	// ModeScroll sizes columns to full plain widths and allows horizontal panning.
	ModeScroll GridMode = iota
	// ModeFit pre-truncates ASCII text to fit the viewport, then applies styles.
	ModeFit
)

// BigTable is a reusable Bubble Tea component that renders large, dynamic
// tables backed by a List provider. It is optimized for very large datasets
// via windowed rendering and maintains selection stability using row IDs.
type BigTable struct {
	mode GridMode
	w, h int

	cols []Column // base titles (ASCII)
	list List     // data provider

	desired []int // initial width hints

	// selection & windowing
	window     []Row               // rows currently rendered in the table (order matches tbl rows)
	selected   map[string]struct{} // multi-select set by row ID
	selStyle   lipgloss.Style      // selection style applied over cell styles
	top        int                 // absolute index of top row in provider
	cursor     int                 // absolute cursor index in provider
	focusedID  string              // ID of the currently focused row (for stability across updates)
	widthCache []int               // incremental max width cache for columns
	xOff       int                 // horizontal offset (scroll) in cells across all columns
	hStep      int                 // horizontal step for left/right

	headerRow string // sticky header row (rendered outside viewport)
	bodyRow   string // rendered body content cached

	styles    Styles // external styles
	truncTail string // tail used when truncating in FIT mode (default: "")

	// Header/body tables holding direct border configuration
	header *lgtable.Table
	body   *lgtable.Table

	// Minimal flags needed for layout decisions (height, spacing)
	bTop    bool // header top border
	bBottom bool // body bottom border
	bLeft   bool
	bRight  bool
	bColumn bool
	bHeader bool // header underline
}

// Styles groups all externally configurable styles.
type Styles struct {
	Outer    lipgloss.Style
	Header   lipgloss.Style
	Footer   lipgloss.Style
	Selector lipgloss.Style
	Cell     lipgloss.Style
	Border   lipgloss.Style
}

// DefaultStyles returns a set of defaults for the table.
func DefaultStyles() Styles {
	return Styles{
		Outer:    lipgloss.NewStyle().Padding(0, 1),
		Header:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Green),
		Footer:   lipgloss.NewStyle().Faint(true).Foreground(lipgloss.White),
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
		selStyle:   lipgloss.NewStyle().Background(lipgloss.Color("12")).Foreground(lipgloss.Color("0")),
		top:        0,
		cursor:     0,
		focusedID:  "",
		widthCache: initWidthCache(cols),
		xOff:       0,
		hStep:      8,
		styles:     DefaultStyles(),
		truncTail:  "",
		header:     lgtable.New().Wrap(false),
		body:       lgtable.New().Wrap(false),
		bTop:       false,
		bBottom:    false,
		bLeft:      false,
		bRight:     false,
		bColumn:    false,
		bHeader:    false,
	}
	bt.applyMode()
	bt.sync()
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
	m.sync()
}

// --- Border configuration (1:1 with lipgloss/table) ---

func (m *BigTable) Border(b lipgloss.Border) *BigTable {
	m.header = m.header.Border(b)
	m.body = m.body.Border(b)
	m.rebuildWindow()
	return m
}

// BorderTop applies above the header only.
func (m *BigTable) BorderTop(v bool) *BigTable {
	m.bTop = v
	m.header = m.header.BorderTop(v)
	m.body = m.body.BorderTop(false)
	m.rebuildWindow()
	return m
}

// BorderBottom applies to body only (footer line).
func (m *BigTable) BorderBottom(v bool) *BigTable {
	m.bBottom = v
	m.header = m.header.BorderBottom(false)
	m.body = m.body.BorderBottom(v)
	m.rebuildWindow()
	return m
}

func (m *BigTable) BorderLeft(v bool) *BigTable {
	m.bLeft = v
	m.header = m.header.BorderLeft(v)
	m.body = m.body.BorderLeft(v)
	m.rebuildWindow()
	return m
}

func (m *BigTable) BorderRight(v bool) *BigTable {
	m.bRight = v
	m.header = m.header.BorderRight(v)
	m.body = m.body.BorderRight(v)
	m.rebuildWindow()
	return m
}

func (m *BigTable) BorderRow(v bool) *BigTable {
	m.body = m.body.BorderRow(v)
	m.rebuildWindow()
	return m
}

func (m *BigTable) BorderColumn(v bool) *BigTable {
	m.bColumn = v
	m.header = m.header.BorderColumn(v)
	m.body = m.body.BorderColumn(v)
	m.rebuildWindow()
	return m
}

func (m *BigTable) BorderHeader(v bool) *BigTable {
	m.bHeader = v
	m.header = m.header.BorderHeader(v)
	m.rebuildWindow()
	return m
}

func (m *BigTable) BorderStyle(s lipgloss.Style) *BigTable {
	m.header = m.header.BorderStyle(s)
	m.body = m.body.BorderStyle(s)
	m.rebuildWindow()
	return m
}

// SetMode switches between ModeScroll and ModeFit and refreshes the view.
func (m *BigTable) SetMode(md GridMode) {
	if m.mode != md {
		m.mode = md
		m.applyMode()
		m.sync()
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

// SetTruncationTail sets the ASCII tail used when truncating in FIT mode.
// Use "" to disable ellipsis completely.
func (m *BigTable) SetTruncationTail(tail string) {
	m.truncTail = tail
	m.rebuildWindow()
}

// SetList swaps the data provider and repositions the cursor according to
// the focused row ID. If the focused row disappeared, the cursor moves to the
// next row; if none, to the previous; otherwise it clamps within bounds.
// SetList swaps the data provider and repositions the cursor based on the
// previously focused row ID. If that row disappeared, the cursor moves to the
// next row, or previous when no next exists.
func (m *BigTable) SetList(list List) {
	m.list = list
	m.repositionOnDataChange()
	m.rebuildWindow()
}

// GetList exposes the current data provider (for demo mutations).
// GetList returns the current data provider.
func (m *BigTable) GetList() List { return m.list }

// CurrentID returns the focused row ID, if any.
// CurrentID returns the focused row ID, if any.
func (m *BigTable) CurrentID() (string, bool) {
	if row := m.list.Lines(m.cursor, 1); len(row) == 1 {
		id, _, _, ok := row[0].Columns()
		return id, ok
	}
	return "", false
}

// Update handles key navigation and selection toggling; it also forwards other
// messages to the internal bubbles components. It returns a batchable pair of
// commands for external composition.
func (m *BigTable) Update(msg tea.Msg) (tea.Cmd, tea.Cmd) {
	var c1, c2 tea.Cmd
	var consumed bool
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
				consumed = true
			}
		case "down", "j":
			if m.cursor+1 < m.list.Len() {
				m.cursor++
				vis := m.bodyRowsHeight()
				if m.cursor >= m.top+vis {
					m.top = m.cursor - (vis - 1)
				}
				m.rebuildWindow()
				consumed = true
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
			consumed = true
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
			consumed = true
		case "home":
			m.cursor = 0
			m.top = 0
			m.rebuildWindow()
			consumed = true
		case "end":
			if n := m.list.Len(); n > 0 {
				m.cursor = n - 1
				vis := m.bodyRowsHeight()
				m.top = max(0, n-vis)
				m.rebuildWindow()
			}
			consumed = true
		case "left":
			if m.xOff > 0 {
				m.xOff -= m.hStep
				if m.xOff < 0 {
					m.xOff = 0
				}
				m.applyMode() // recalc columns + rows for scroll window
				consumed = true
			}
		case "right":
			// Advance the window while total width beyond viewport.
			total := 0
			for _, w := range m.widthCache {
				total += w
			}
			if total > m.w {
				m.xOff += m.hStep
				if m.xOff > total-m.w {
					m.xOff = total - m.w
				}
				if m.xOff < 0 {
					m.xOff = 0
				}
				m.applyMode()
				consumed = true
			}
		}
	}
	m.sync()
	return c1, c2
}

// View renders the component.
func (m *BigTable) View() string {
	sticky := m.styles.Header.Render(strings.TrimRight(m.headerRow, "\n"))
	body := strings.TrimRight(m.bodyRow, "\n")
	if sticky != "" {
		return strings.Join([]string{sticky, body}, "\n")
	}
	return body
}

// no app header/help line inside BigTable; outer app should render it
// no BigTable footer; outer app should render any footers/help lines

func (m *BigTable) sync() {}

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
	// Update width cache from visible rows (plain ASCII cells)
	for _, row := range m.window {
		_, cells, _, _ := row.Columns()
		for i := 0; i < len(m.cols) && i < len(cells); i++ {
			if w := lipgloss.Width(cells[i]); w > m.widthCache[i] {
				m.widthCache[i] = w
			}
		}
	}
	// Use persistent body table holding direct border configuration.
	t := m.body
	if m.mode == ModeFit {
		desired := make([]int, len(m.cols))
		for i := range desired {
			desired[i] = max(m.widthCache[i], m.desired[i])
		}
		target := computeFitWidths(m.w, desired, 3)
		ht := m.header
		headers := make([]string, len(m.cols))
		// spacing only when no outside verticals and no inner verticals
		outsideVertical := m.bLeft || m.bRight
		vcol := m.bColumn
		tt := append([]int(nil), target...)
		if !outsideVertical && !vcol {
			for i := 0; i < len(tt)-1; i++ {
				if tt[i] > 0 {
					tt[i]--
				}
			}
		}
		for i, c := range m.cols {
			headers[i] = m.truncPad(c.Title, tt[i])
		}
		if !outsideVertical && !vcol {
			for i := 0; i < len(headers)-1; i++ {
				headers[i] += " "
			}
		}
		ht.Headers(headers...)
		ht.Width(m.w)
		ht.StyleFunc(func(row, col int) lipgloss.Style { return m.styles.Header })
		m.headerRow = strings.TrimRight(ht.Render(), "\n")
		trRows := truncateRowsWithTail(m.window, tt, m.truncTail)
		if !outsideVertical && !vcol {
			trRows = addSpacing(trRows)
		}
		t = t.Rows(rowsToStringRows(trRows)...)
		t.Width(m.w)
	} else {
		full := make([]int, len(m.cols))
		for i := range full {
			full[i] = max(m.widthCache[i], m.desired[i])
		}
		offs, target := computeScrollWindowFrozen(full, 2, m.xOff, m.w)
		ht := m.header
		headers := make([]string, len(m.cols))
		outsideVertical := m.bLeft || m.bRight
		vcol := m.bColumn
		tt := append([]int(nil), target...)
		if !outsideVertical && !vcol {
			for i := 0; i < len(tt)-1; i++ {
				if tt[i] > 0 {
					tt[i]--
				}
			}
		}
		for i, c := range m.cols {
			headers[i] = m.truncPad(c.Title, tt[i])
		}
		if !outsideVertical && !vcol {
			for i := 0; i < len(headers)-1; i++ {
				headers[i] += " "
			}
		}
		ht.Headers(headers...)
		ht.Width(m.w)
		ht.StyleFunc(func(row, col int) lipgloss.Style { return m.styles.Header })
		m.headerRow = strings.TrimRight(ht.Render(), "\n")
		sliced := sliceRowsWindow(m.window, offs, tt)
		if !outsideVertical && !vcol {
			sliced = addSpacing(sliced)
		}
		t = t.Rows(rowsToStringRows(sliced)...)
		t.Width(m.w)
	}
	stylesPerRow := captureStyles(m.window)
	selected := m.selected
	t.StyleFunc(func(row, col int) lipgloss.Style {
		if row == lgtable.HeaderRow {
			return m.styles.Header
		}
		if row < 0 || row >= len(stylesPerRow) {
			return lipgloss.NewStyle()
		}
		// Compose per-cell style overriding base cell style
		st := m.styles.Cell
		if col < len(stylesPerRow[row]) && stylesPerRow[row][col] != nil {
			st = (*stylesPerRow[row][col]).Inherit(st)
		}
		id, _, _, _ := m.window[row].Columns()
		// Focus line overlay (selector): set bg/fg from selector but keep per-cell fg if selector fg is unset
		if row == (m.cursor - m.top) {
			st = m.styles.Selector.Inherit(st)
		}
		// Multi-select overlay
		if _, ok := selected[id]; ok {
			st = m.styles.Selector.Inherit(st)
		}
		return st
	})
	// Cache body rows without trimming content; ensure no trailing newline.
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
// after subtracting sticky header lines and any body border lines.
func (m *BigTable) bodyRowsHeight() int {
	// Reserve lines: sticky header, optional top border, optional header underline, optional bottom border
	reserved := 1
	if m.bTop {
		reserved++
	}
	if m.bHeader {
		reserved++
	}
	if m.bBottom {
		reserved++
	}
	rows := m.h - reserved
	if rows < 1 {
		rows = 1
	}
	return rows
}

func computeFitWidths(total int, desired []int, minCol int) []int {
	n := len(desired)
	if n == 0 {
		return nil
	}
	if minCol < 1 {
		minCol = 1
	}
	sumDesired := 0
	for _, d := range desired {
		if d < minCol {
			d = minCol
		}
		sumDesired += d
	}
	if sumDesired <= total {
		out := make([]int, n)
		for i, d := range desired {
			out[i] = max(d, minCol)
		}
		return out
	}
	out := make([]int, n)
	base := 0
	for i, d := range desired {
		if d < minCol {
			d = minCol
		}
		q := d * total / sumDesired
		if q < minCol {
			q = minCol
		}
		out[i] = q
		base += q
	}
	rem := total - base
	for rem > 0 {
		for i := range out {
			if rem == 0 {
				break
			}
			out[i]++
			rem--
		}
	}
	return out
}

// computeScrollWindow maps a global horizontal offset into per-column start
// offsets and visible widths. 'full' is the full width of each column, 'xOff'
// the horizontal offset across the entire row, and 'total' the viewport width.
func computeScrollWindow(full []int, xOff, total int) ([]int, []int) {
	n := len(full)
	offs := make([]int, n)
	vis := make([]int, n)
	if total <= 0 || n == 0 {
		return offs, vis
	}
	// Consume offset across columns
	off := xOff
	for i := 0; i < n; i++ {
		w := full[i]
		if off >= w {
			offs[i] = w
			vis[i] = 0
			off -= w
			continue
		}
		offs[i] = max(0, off)
		break
	}
	// Compute visible widths given remaining window space
	rem := total
	for i := 0; i < n && rem > 0; i++ {
		w := full[i]
		if offs[i] >= w {
			vis[i] = 0
			continue
		}
		avail := w - offs[i]
		if avail > rem {
			avail = rem
		}
		vis[i] = avail
		rem -= avail
	}
	return offs, vis
}

// computeScrollWindowFrozen behaves like computeScrollWindow but keeps the
// first freezeN columns fully visible and un-sliced. The remaining viewport
// width is used for horizontal panning across the remaining columns.
func computeScrollWindowFrozen(full []int, freezeN, xOff, total int) ([]int, []int) {
	n := len(full)
	offs := make([]int, n)
	vis := make([]int, n)
	if n == 0 || total <= 0 {
		return offs, vis
	}
	if freezeN < 0 {
		freezeN = 0
	}
	if freezeN > n {
		freezeN = n
	}
	// reduce freeze count until it fits in total
	for freezeN > 0 {
		sum := 0
		for i := 0; i < freezeN; i++ {
			sum += full[i]
		}
		if sum <= total {
			break
		}
		freezeN--
	}
	// assign frozen columns
	frozenWidth := 0
	for i := 0; i < freezeN; i++ {
		vis[i] = full[i]
		frozenWidth += full[i]
	}
	remTotal := total - frozenWidth
	if remTotal <= 0 {
		// No room for scrolling part
		for i := freezeN; i < n; i++ {
			vis[i] = 0
			offs[i] = full[i]
		}
		return offs, vis
	}
	// Compute window across the scrolled columns
	offsTail, visTail := computeScrollWindow(full[freezeN:], xOff, remTotal)
	for i := freezeN; i < n; i++ {
		offs[i] = offsTail[i-freezeN]
		vis[i] = visTail[i-freezeN]
	}
	return offs, vis
}

// --- truncate (plain ASCII) then style ---

func asciiTruncatePadTail(s string, w int, tail string) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		if pad := w - lipgloss.Width(s); pad > 0 {
			return s + strings.Repeat(" ", pad)
		}
		return s
	}
	// Truncate with the provided tail; empty tail disables ellipsis.
	out := truncate.StringWithTail(s, uint(w), tail)
	if pad := w - lipgloss.Width(out); pad > 0 {
		out += strings.Repeat(" ", pad)
	}
	return out
}

// Backwards-compatible helper used in tests.
func asciiTruncatePad(s string, w int) string { return asciiTruncatePadTail(s, w, "...") }

// instance-aware helper for FIT mode
func (m *BigTable) truncPad(s string, w int) string { return asciiTruncatePadTail(s, w, m.truncTail) }

// asciiSlicePad returns a substring of s that begins at start and extends for
// width cells, padding with spaces if necessary. s is expected to be ASCII.
func asciiSlicePad(s string, start, width int) string {
	if width <= 0 {
		return ""
	}
	if start < 0 {
		start = 0
	}
	// clamp start to length
	if start >= len(s) {
		return strings.Repeat(" ", width)
	}
	end := start + width
	if end > len(s) {
		end = len(s)
	}
	out := s[start:end]
	if pad := width - lipgloss.Width(out); pad > 0 {
		out += strings.Repeat(" ", pad)
	}
	return out
}

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

// addSpacing appends a trailing space to all but the last column cells to act
// as a column separator when no vertical border is drawn.
func addSpacing(rows []Row) []Row {
	out := make([]Row, len(rows))
	for r := range rows {
		id, cells, styles, _ := rows[r].Columns()
		cc := make([]string, len(cells))
		copy(cc, cells)
		for i := 0; i < len(cc)-1; i++ {
			cc[i] += " "
		}
		out[r] = SimpleRow{ID: id, Cells: cc, Styles: styles}
	}
	return out
}

func truncateRowsWithTail(rows []Row, target []int, tail string) []Row {
	tr := make([]Row, len(rows))
	for r := range rows {
		id, cells, styles, _ := rows[r].Columns()
		truncated := make([]string, len(target))
		for c := range target {
			s := ""
			if c < len(cells) {
				s = cells[c]
			}
			truncated[c] = asciiTruncatePadTail(s, target[c], tail)
		}
		tr[r] = SimpleRow{ID: id, Cells: truncated, Styles: styles}
	}
	return tr
}

// sliceRowsWindow slices each cell horizontally according to the provided
// per-column start offsets and visible widths, returning a new []Row of
// SimpleRow to be styled afterwards.
func sliceRowsWindow(rows []Row, offs []int, vis []int) []Row {
	out := make([]Row, len(rows))
	for r := range rows {
		id, cells, styles, _ := rows[r].Columns()
		sliced := make([]string, len(vis))
		for c := range vis {
			start := 0
			if c < len(offs) {
				start = offs[c]
			}
			w := 0
			if c < len(vis) {
				w = vis[c]
			}
			s := ""
			if c < len(cells) {
				s = cells[c]
			}
			sliced[c] = asciiSlicePad(s, start, w)
		}
		out[r] = SimpleRow{ID: id, Cells: sliced, Styles: styles}
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

func initWidthCache(cols []Column) []int {
	out := make([]int, len(cols))
	for i := range cols {
		out[i] = lipgloss.Width(cols[i].Title)
	}
	return out
}
