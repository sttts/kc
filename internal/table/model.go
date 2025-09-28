package table

import (
    "github.com/charmbracelet/lipgloss/v2"
)

// Row represents a single logical row in the table.
// It must return a stable ID, ASCII cell strings, and a style for each cell.
type Row interface {
    Columns() (id string, cells []string, styles []*lipgloss.Style, exists bool)
}

// List provides windowed access to rows. Implementations should be efficient
// for large datasets (10s of thousands), serving only the requested slices.
type List interface {
    Lines(top, num int) []Row
    Above(rowID string, num int) []Row
    Below(rowID string, num int) []Row
    Len() int
    // Find returns the absolute index and row for a given ID if present.
    Find(rowID string) (int, Row, bool)
}

// SimpleRow is a reusable row type with a stable ID and per-cell styles.
// It implements Row and can be used by any List implementation.
type SimpleRow struct {
    ID     string
    Cells  []string
    Styles []*lipgloss.Style
}

func (r SimpleRow) Columns() (string, []string, []*lipgloss.Style, bool) {
    return r.ID, r.Cells, r.Styles, true
}

// SetColumn sets the text and optional style at the given column index.
// If style is nil, the previous style is kept; if it doesn't exist, a neutral style is used.
func (r *SimpleRow) SetColumn(col int, text string, style *lipgloss.Style) {
    if col < 0 { return }
    // grow cells slice as needed
    if col >= len(r.Cells) {
        grow := make([]string, col+1)
        copy(grow, r.Cells)
        r.Cells = grow
    }
    r.Cells[col] = text
    // ensure Styles slice length matches
    if col >= len(r.Styles) {
        grow := make([]*lipgloss.Style, col+1)
        copy(grow, r.Styles)
        r.Styles = grow
    }
    if style != nil {
        r.Styles[col] = style
    } else if r.Styles[col] == nil {
        s := lipgloss.NewStyle()
        r.Styles[col] = &s
    }
}

// SliceList is a default slice-backed data source with copy-on-insert/remove.
type SliceList struct {
    rows  []Row
    index map[string]int // id -> position
}

func NewSliceList(rows []Row) *SliceList {
    l := &SliceList{rows: append([]Row(nil), rows...)}
    l.rebuildIndex()
    return l
}

func (l *SliceList) rebuildIndex() {
    l.index = make(map[string]int, len(l.rows))
    for i, r := range l.rows { id, _, _, _ := r.Columns(); l.index[id] = i }
}

// Mutations (not part of List interface) ------------------------------------

// InsertBefore inserts rows before the row with anchorID. Returns the insert index or -1 if anchor not found.
func (l *SliceList) InsertBefore(anchorID string, newRows ...Row) int {
    i, ok := l.index[anchorID]
    if !ok { return -1 }
    pre := append([]Row(nil), l.rows[:i]...)
    post := append([]Row(nil), l.rows[i:]...)
    l.rows = append(pre, append(newRows, post...)...)
    l.rebuildIndex()
    return i
}

// InsertAfter inserts rows after the row with anchorID. Returns the first insert index or -1.
func (l *SliceList) InsertAfter(anchorID string, newRows ...Row) int {
    i, ok := l.index[anchorID]
    if !ok { return -1 }
    i++
    pre := append([]Row(nil), l.rows[:i]...)
    post := append([]Row(nil), l.rows[i:]...)
    l.rows = append(pre, append(newRows, post...)...)
    l.rebuildIndex()
    return i
}

func (l *SliceList) Append(newRows ...Row) { l.rows = append(l.rows, newRows...); l.rebuildIndex() }
func (l *SliceList) Prepend(newRows ...Row) { l.rows = append(append([]Row(nil), newRows...), l.rows...); l.rebuildIndex() }

// RemoveIDs removes all rows with the provided IDs. Returns count removed.
func (l *SliceList) RemoveIDs(ids ...string) int {
    if len(ids) == 0 { return 0 }
    rm := make(map[string]struct{}, len(ids))
    for _, id := range ids { rm[id] = struct{}{} }
    kept := make([]Row, 0, len(l.rows))
    removed := 0
    for _, r := range l.rows {
        id, _, _, _ := r.Columns()
        if _, drop := rm[id]; drop { removed++; continue }
        kept = append(kept, r)
    }
    if removed > 0 { l.rows = kept; l.rebuildIndex() }
    return removed
}

// RemoveAt removes a contiguous range [i, i+count). Returns count actually removed.
func (l *SliceList) RemoveAt(i, count int) int {
    if count <= 0 || i < 0 || i >= len(l.rows) { return 0 }
    end := i + count
    if end > len(l.rows) { end = len(l.rows) }
    kept := append([]Row(nil), l.rows[:i]...)
    kept = append(kept, l.rows[end:]...)
    removed := len(l.rows) - len(kept)
    if removed > 0 { l.rows = kept; l.rebuildIndex() }
    return removed
}

// List interface implementation ---------------------------------------------

func (l *SliceList) Len() int { return len(l.rows) }

func (l *SliceList) Lines(top, num int) []Row {
    if num <= 0 || top >= len(l.rows) { return nil }
    if top < 0 { top = 0 }
    end := top + num
    if end > len(l.rows) { end = len(l.rows) }
    return l.rows[top:end]
}

// LinesToRows is a helper to copy a slice of Row values.
func LinesToRows(in []Row) []Row {
    out := make([]Row, len(in))
    copy(out, in)
    return out
}

func (l *SliceList) Above(rowID string, num int) []Row {
    i, ok := l.index[rowID]
    if !ok { return nil }
    start := i - num
    if start < 0 { start = 0 }
    out := make([]Row, i-start)
    for j := start; j < i; j++ { out[j-start] = l.rows[j] }
    return out
}

func (l *SliceList) Below(rowID string, num int) []Row {
    i, ok := l.index[rowID]
    if !ok { return nil }
    end := i + 1 + num
    if end > len(l.rows) { end = len(l.rows) }
    out := make([]Row, end-(i+1))
    k := 0
    for j := i + 1; j < end; j++ { out[k] = l.rows[j]; k++ }
    return out
}

func (l *SliceList) Find(rowID string) (int, Row, bool) {
    i, ok := l.index[rowID]
    if !ok || i < 0 || i >= len(l.rows) { return -1, nil, false }
    return i, l.rows[i], true
}

var _ List = (*SliceList)(nil)
