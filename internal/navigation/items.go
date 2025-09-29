package navigation

import (
    lipgloss "github.com/charmbracelet/lipgloss/v2"
    table "github.com/sttts/kc/internal/table"
)

// BackItem renders the ".." entry and marks a back action.
type BackItem struct{}

var _ Back = (*BackItem)(nil)
var _ Item = (*BackItem)(nil)

func (b BackItem) Columns() (string, []string, []*lipgloss.Style, bool) {
    s := GreenStyle()
    return "__back__", []string{".."}, []*lipgloss.Style{s}, true
}

func (b BackItem) Details() string { return "Back" }
func (b BackItem) Path() []string  { return nil }

// SimpleItem is a reusable Item backed by a table.SimpleRow plus a details string.
type SimpleItem struct {
    Row     table.SimpleRow
    details string
    path    []string
}

var _ Item = (*SimpleItem)(nil)

// NewSimpleItem constructs an Item with a stable ID, cells and an optional style
// applied to all cells (default: green). Styles are per-cell per Row interface contract.
func NewSimpleItem(id string, cells []string, path []string, style *lipgloss.Style) *SimpleItem {
    if style == nil { style = GreenStyle() }
    styles := make([]*lipgloss.Style, len(cells))
    for i := range styles { styles[i] = style }
    return &SimpleItem{Row: table.SimpleRow{ID: id, Cells: cells, Styles: styles}, path: append([]string(nil), path...)}
}

func (s *SimpleItem) Columns() (string, []string, []*lipgloss.Style, bool) {
    return s.Row.Columns()
}

func (s *SimpleItem) Details() string { return s.details }

// WithDetails sets the details string and returns the same item for chaining.
func (s *SimpleItem) WithDetails(d string) *SimpleItem { s.details = d; return s }

// Path returns the absolute path segments for this item.
func (s *SimpleItem) Path() []string { return append([]string(nil), s.path...) }
