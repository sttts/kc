package models

import "github.com/charmbracelet/lipgloss/v2"

// SimpleItem embeds RowItem and can optionally expose view content.
type SimpleItem struct {
	*RowItem
	viewFn ViewContentFunc
}

func NewSimpleItem(id string, cells []string, path []string, style *lipgloss.Style) *SimpleItem {
	return &SimpleItem{RowItem: NewRowItem(id, cells, path, style)}
}

func (s *SimpleItem) WithViewContent(fn ViewContentFunc) *SimpleItem {
	s.viewFn = fn
	return s
}

func (s *SimpleItem) ViewContent() (string, string, string, string, string, error) {
	if s.viewFn == nil {
		return "", "", "", "", "", ErrNoViewContent
	}
	return s.viewFn()
}
