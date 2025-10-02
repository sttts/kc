package models

import (
	"github.com/charmbracelet/lipgloss/v2"
	table "github.com/sttts/kc/internal/table"
)

// RowItem is the minimal table-backed item implementation shared by all rows.
type RowItem struct {
	table.SimpleRow
	details string
	path    []string
}

func NewRowItem(id string, cells []string, path []string, style *lipgloss.Style) *RowItem {
	if style == nil {
		style = GreenStyle()
	}
	styles := make([]*lipgloss.Style, len(cells))
	for i := range styles {
		styles[i] = style
	}
	return NewRowItemStyled(id, cells, path, styles)
}

func NewRowItemStyled(id string, cells []string, path []string, styles []*lipgloss.Style) *RowItem {
	cloned := make([]string, len(cells))
	copy(cloned, cells)
	sty := make([]*lipgloss.Style, len(styles))
	copy(sty, styles)
	return &RowItem{
		SimpleRow: table.SimpleRow{ID: id, Cells: cloned, Styles: sty},
		path:      append([]string(nil), path...),
	}
}

func (r *RowItem) Details() string { return r.details }
func (r *RowItem) Path() []string  { return append([]string(nil), r.path...) }
func (r *RowItem) ID() string      { return r.SimpleRow.ID }

func (r *RowItem) copyFrom(other *RowItem) {
	if r == nil || other == nil {
		return
	}
	r.SimpleRow = other.SimpleRow
	r.details = other.details
	r.path = append([]string(nil), other.path...)
}
