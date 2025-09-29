package navigation

import (
    "strings"
    lipgloss "github.com/charmbracelet/lipgloss/v2"
    table "github.com/sttts/kc/internal/table"
)

// EnterableItem is a reusable Item that is also Enterable via a provided function.
type EnterableItem struct {
    Row     table.SimpleRow
    details string
    enter   func() (Folder, error)
    path    []string
}

var _ Item = (*EnterableItem)(nil)
var _ Enterable = (*EnterableItem)(nil)

func NewEnterableItem(id string, cells []string, path []string, enter func() (Folder, error), style *lipgloss.Style) *EnterableItem {
    if style == nil { style = GreenStyle() }
    // Ensure enterable items show a leading "/" in the first cell for generic folder UI
    if len(cells) > 0 && !strings.HasPrefix(cells[0], "/") {
        cells = append([]string(nil), cells...)
        cells[0] = "/" + cells[0]
    }
    styles := make([]*lipgloss.Style, len(cells))
    for i := range styles { styles[i] = style }
    return &EnterableItem{Row: table.SimpleRow{ID: id, Cells: cells, Styles: styles}, enter: enter, path: append([]string(nil), path...)}
}

// NewEnterableItemStyled constructs an EnterableItem with per-cell styles.
func NewEnterableItemStyled(id string, cells []string, path []string, styles []*lipgloss.Style, enter func() (Folder, error)) *EnterableItem {
    // ensure styles slice length matches cells
    // Ensure leading "/" in the first cell for enterable rows
    if len(cells) > 0 && !strings.HasPrefix(cells[0], "/") {
        cells = append([]string(nil), cells...)
        cells[0] = "/" + cells[0]
    }
    ss := make([]*lipgloss.Style, len(cells))
    copy(ss, styles)
    for i := range ss {
        if ss[i] == nil { s := lipgloss.NewStyle(); ss[i] = &s }
    }
    return &EnterableItem{Row: table.SimpleRow{ID: id, Cells: cells, Styles: ss}, enter: enter, path: append([]string(nil), path...)}
}

func (e *EnterableItem) Columns() (string, []string, []*lipgloss.Style, bool) { return e.Row.Columns() }
func (e *EnterableItem) Details() string { return e.details }
func (e *EnterableItem) WithDetails(d string) *EnterableItem { e.details = d; return e }
func (e *EnterableItem) Enter() (Folder, error) { if e.enter == nil { return nil, nil }; return e.enter() }
func (e *EnterableItem) Path() []string { return append([]string(nil), e.path...) }
