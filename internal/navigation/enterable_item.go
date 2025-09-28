package navigation

import (
    lipgloss "github.com/charmbracelet/lipgloss/v2"
    table "github.com/sttts/kc/internal/table"
)

// EnterableItem is a reusable Item that is also Enterable via a provided function.
type EnterableItem struct {
    Row     table.SimpleRow
    details string
    enter   func() (Folder, error)
}

var _ Item = (*EnterableItem)(nil)
var _ Enterable = (*EnterableItem)(nil)

func NewEnterableItem(id string, cells []string, enter func() (Folder, error), style *lipgloss.Style) *EnterableItem {
    if style == nil { style = GreenStyle() }
    styles := make([]*lipgloss.Style, len(cells))
    for i := range styles { styles[i] = style }
    return &EnterableItem{Row: table.SimpleRow{ID: id, Cells: cells, Styles: styles}, enter: enter}
}

func (e *EnterableItem) Columns() (string, []string, []*lipgloss.Style, bool) { return e.Row.Columns() }
func (e *EnterableItem) Details() string { return e.details }
func (e *EnterableItem) WithDetails(d string) *EnterableItem { e.details = d; return e }
func (e *EnterableItem) Enter() (Folder, error) { if e.enter == nil { return nil, nil }; return e.enter() }

