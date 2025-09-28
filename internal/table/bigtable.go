package table

import (
	"fmt"
	"strings"

	viewport "github.com/charmbracelet/bubbles/v2/viewport"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
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
	vp   viewport.Model
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

	styles     Styles // external styles
	borderMode int    // current border variant
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
		Header:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8AFF80")),
		Footer:   lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#7D7D7D")),
		Selector: lipgloss.NewStyle().Background(lipgloss.Color("12")).Foreground(lipgloss.Color("0")),
		Cell:     lipgloss.NewStyle(),
		Border:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")),
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

	vp := viewport.New(viewport.WithWidth(max(20, w)), viewport.WithHeight(max(6, h)))
	vp.SoftWrap = false
	// Disable viewport's internal horizontal panning; we implement ANSI-safe
	// horizontal slicing ourselves.
	vp.SetHorizontalStep(0)

	bt := BigTable{
		vp:         vp,
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
		borderMode: 0,
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
	m.vp.SetWidth(w)
	m.vp.SetHeight(h)
	m.applyMode()
	m.sync()
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
				h := m.h - 2
				if m.cursor >= m.top+h {
					m.top = m.cursor - (h - 1)
				}
				m.rebuildWindow()
				consumed = true
			}
		case "pgup":
			h := m.h - 2
			if h < 1 {
				h = 1
			}
			m.cursor -= h
			if m.cursor < 0 {
				m.cursor = 0
			}
			if m.cursor < m.top {
				m.top = m.cursor
			}
			m.rebuildWindow()
			consumed = true
		case "pgdown":
			h := m.h - 2
			if h < 1 {
				h = 1
			}
			m.cursor += h
			if m.cursor >= m.list.Len() {
				m.cursor = m.list.Len() - 1
			}
			if m.cursor >= m.top+h {
				m.top = max(0, m.cursor-(h-1))
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
				h := m.h - 2
				m.top = max(0, n-h)
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
	if !consumed {
		m.vp, c1 = m.vp.Update(msg)
	}
	// Avoid forwarding movement keys—we control cursor explicitly.
	switch v := msg.(type) {
	case tea.KeyMsg:
		switch v.String() {
		case "up", "k", "down", "j", "pgup", "pgdown", "home", "end", "ctrl+t", "insert", "left", "right":
			// handled
		default:
			// nothing
		}
	default:
		// nothing
	}
	m.sync()
	return c1, c2
}

// View renders the component.
func (m *BigTable) View() string {
	sticky := m.styles.Header.Render(strings.TrimRight(m.headerRow, "\n"))
	body := strings.TrimRight(m.vp.View(), "\n")
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
	// Sticky header uses 1 line; app chrome should be outside the component.
	reserved := 1
	h := m.h - reserved
	if h < 1 {
		h = 1
	}
	n := m.list.Len()
	if n < 0 {
		n = 0
	}
	if m.cursor >= n {
		m.cursor = max(0, n-1)
	}
	maxTop := max(0, n-h)
	if m.top > maxTop {
		m.top = maxTop
	}
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+h {
		m.top = max(0, m.cursor-(h-1))
	}

	m.window = m.list.Lines(m.top, h)
	// Update width cache from visible rows (plain ASCII cells)
	for _, row := range m.window {
		_, cells, _, _ := row.Columns()
		for i := 0; i < len(m.cols) && i < len(cells); i++ {
			if w := lipgloss.Width(cells[i]); w > m.widthCache[i] {
				m.widthCache[i] = w
			}
		}
	}
	// Build lipgloss table content sized to body height.
	t := lgtable.New().Wrap(false).Height(h)
	m.configureBodyBorders(t)
	if m.mode == ModeFit {
		desired := make([]int, len(m.cols))
		for i := range desired {
			desired[i] = max(m.widthCache[i], m.desired[i])
		}
		target := computeFitWidths(m.w, desired, 3)
		ht := lgtable.New().Wrap(false)
		m.configureHeaderBorders(ht)
		headers := make([]string, len(m.cols))
		// spacing only when no outside and no inner verticals
		outside, vcol, _, _ := m.borderFlags()
		tt := append([]int(nil), target...)
		if !outside && !vcol {
			for i := 0; i < len(tt)-1; i++ {
				if tt[i] > 0 {
					tt[i]--
				}
			}
		}
		for i, c := range m.cols {
			headers[i] = asciiTruncatePad(c.Title, tt[i])
		}
		if !outside && !vcol {
			for i := 0; i < len(headers)-1; i++ {
				headers[i] += " "
			}
		}
		ht.Headers(headers...)
		ht.Width(m.w)
		ht.StyleFunc(func(row, col int) lipgloss.Style { return m.styles.Header })
		m.headerRow = strings.TrimRight(ht.Render(), "\n")
		trRows := truncateRows(m.window, tt)
		if !outside && !vcol {
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
		ht := lgtable.New().Wrap(false)
		m.configureHeaderBorders(ht)
		headers := make([]string, len(m.cols))
		outside, vcol, _, _ := m.borderFlags()
		tt := append([]int(nil), target...)
		if !outside && !vcol {
			for i := 0; i < len(tt)-1; i++ {
				if tt[i] > 0 {
					tt[i]--
				}
			}
		}
		for i, c := range m.cols {
			headers[i] = asciiTruncatePad(c.Title, tt[i])
		}
		if !outside && !vcol {
			for i := 0; i < len(headers)-1; i++ {
				headers[i] += " "
			}
		}
		ht.Headers(headers...)
		ht.Width(m.w)
		ht.StyleFunc(func(row, col int) lipgloss.Style { return m.styles.Header })
		m.headerRow = strings.TrimRight(ht.Render(), "\n")
		sliced := sliceRowsWindow(m.window, offs, tt)
		if !outside && !vcol {
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
	// Set viewport height to body height and set content
	m.vp.SetHeight(h)
	body := strings.TrimRight(t.Render(), "\n")
	m.vp.SetContent(body)
	// Track focused ID for stability across updates.
	if row := m.list.Lines(m.cursor, 1); len(row) == 1 {
		id, _, _, ok := row[0].Columns()
		if ok {
			m.focusedID = id
		}
	}
}

func (m *BigTable) applyMode() { m.rebuildWindow() }

// Border mode configuration
// 0: none
// 1: outside only
// 2: inside verticals
// 3: header underline
// 4: verticals + header underline
// 5: verticals + header + outside
// 6: double outside + single inside (verticals + header)
func (m *BigTable) configureHeaderBorders(t *lgtable.Table) {
	outside, vcol, hline, dbl := m.borderFlags()
	t.BorderRow(false)
	t.BorderHeader(hline)
	t.BorderTop(outside)
	t.BorderBottom(false)
	t.BorderLeft(outside)
	t.BorderRight(outside)
	// In outside-only mode, draw header column separators for clarity.
	if outside && !vcol {
		t.BorderColumn(true)
	} else {
		t.BorderColumn(vcol)
	}
	b := buildBorder(outside, vcol, dbl)
	// When outside is off but header underline is on, draw a full-width rule
	// using the normal border's Top and Middle glyphs.
	if !outside && hline {
		nb := lipgloss.NormalBorder()
		b.Top = nb.Top
		if vcol {
			b.Middle = nb.Middle
		} else {
			b.Middle = nb.Top
		}
	}
	t.Border(b)
	t.BorderStyle(m.styles.Border.Inherit(m.styles.Header))
}

func (m *BigTable) configureBodyBorders(t *lgtable.Table) {
	outside, vcol, _, dbl := m.borderFlags()
	t.BorderRow(false)
	t.BorderHeader(false)
	t.BorderTop(false)
	t.BorderLeft(outside)
	t.BorderRight(outside)
	t.BorderBottom(outside)
	t.BorderColumn(vcol || (outside && !vcol))
	b := buildBorder(outside, vcol, dbl)
	t.Border(b)
	t.BorderStyle(m.styles.Border.Inherit(m.styles.Cell))
}

func buildBorder(outside, vcol, dbl bool) lipgloss.Border {
	var b lipgloss.Border
	if outside {
		if dbl {
			b = lipgloss.DoubleBorder()
		} else {
			b = lipgloss.NormalBorder()
		}
	} else {
		b = lipgloss.NormalBorder()
		// remove outside
		b.Top = ""
		b.Bottom = ""
		b.Left = ""
		b.Right = ""
		b.TopLeft = ""
		b.TopRight = ""
		b.BottomLeft = ""
		b.BottomRight = ""
	}
	// When outside is off and vcol is false we rely on addSpacing in content.
	return b
}

func (m *BigTable) borderFlags() (outside bool, vcol bool, hline bool, dbl bool) {
	switch m.borderMode % 8 {
	case 0:
		return false, false, false, false
	case 1:
		return true, false, false, false
	case 2:
		return false, true, false, false
	case 3:
		return false, false, true, false
	case 4:
		return false, true, true, false
	case 5:
		return true, true, true, false
	case 6:
		return true, true, true, true
	default:
		return false, false, false, false
	}
}

// CycleBorderMode increments the border mode.
func (m *BigTable) CycleBorderMode() { m.borderMode = (m.borderMode + 1) % 8; m.rebuildWindow() }

// --- sizing helpers (measure only plain ASCII) ---

func measurePlainWidthsFromProvider(cols []Column, list List) []int { // retained for reference/tests
	n := len(cols)
	w := make([]int, n)
	for i := 0; i < n; i++ {
		w[i] = lipgloss.Width(cols[i].Title)
	}
	capScan := 2000
	remaining := list.Len()
	if remaining > capScan {
		remaining = capScan
	}
	offset := 0
	step := 256
	for remaining > 0 {
		take := step
		if take > remaining {
			take = remaining
		}
		for _, row := range list.Lines(offset, take) {
			_, cells, _, _ := row.Columns()
			for i := 0; i < n && i < len(cells); i++ {
				if cw := lipgloss.Width(cells[i]); cw > w[i] {
					w[i] = cw
				}
			}
		}
		offset += take
		remaining -= take
	}
	return w
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

func asciiTruncatePad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		if pad := w - lipgloss.Width(s); pad > 0 {
			return s + strings.Repeat(" ", pad)
		}
		return s
	}
	if w <= 3 {
		return strings.Repeat(".", w)
	}
	out := truncate.StringWithTail(s, uint(w), "...") // ASCII tail only
	if pad := w - lipgloss.Width(out); pad > 0 {
		out += strings.Repeat(" ", pad)
	}
	return out
}

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

// padRightToWidth pads each rendered line to target width using the given
// style for the padding so background remains consistent.
func padRightToWidth(s string, width int, st lipgloss.Style) string {
	if width <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		w := lipgloss.Width(ln)
		if w < width {
			pad := strings.Repeat(" ", width-w)
			lines[i] = ln + st.Render(pad)
		} else if w > width {
			// hard clamp if needed
			lines[i] = truncate.StringWithTail(ln, uint(width), "")
		}
	}
	return strings.Join(lines, "\n")
}

func truncateRows(rows []Row, target []int) []Row {
	tr := make([]Row, len(rows))
	for r := range rows {
		id, cells, styles, _ := rows[r].Columns()
		truncated := make([]string, len(target))
		for c := range target {
			s := ""
			if c < len(cells) {
				s = cells[c]
			}
			truncated[c] = asciiTruncatePad(s, target[c])
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

// mselStyle returns the global selection style used when overlaying selected rows.
func mselStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(lipgloss.Color("12")).Foreground(lipgloss.Color("0"))
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

// --- styles for demo rendering ---
var (
	outerStyle  = lipgloss.NewStyle().Padding(0, 1)
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8AFF80"))
	footerStyle = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#7D7D7D"))
)

// --- utilities ---

func collectAll(list List) []Row { return list.Lines(0, list.Len()) }

func sum(xs []int) int {
	s := 0
	for _, v := range xs {
		s += v
	}
	return s
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func initWidthCache(cols []Column) []int {
	out := make([]int, len(cols))
	for i := range cols {
		out[i] = lipgloss.Width(cols[i].Title)
	}
	return out
}

// renderHeaderRow renders a single header row with the given titles and
// target widths (no horizontal scrolling applied to titles).
func renderHeaderRow(cols []Column, _ []int, target []int) string {
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = asciiTruncatePad(c.Title, target[i])
	}
	ht := lgtable.New().Wrap(false)
	// Borders configured by caller after creating the row; keep minimal here
	ht = ht.BorderLeft(false).BorderRight(false).BorderTop(false).BorderBottom(false).BorderRow(false).BorderHeader(false).BorderColumn(false)
	ht.Headers(headers...)
	return strings.TrimRight(ht.Render(), "\n")
}

func fmtPercent(p int) string {
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return fmt.Sprintf("%3d%%", p)
}
func fmtPos(first, total int) string { return fmt.Sprintf("%d/%d", first, total) }
