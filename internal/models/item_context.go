package models

import "github.com/charmbracelet/lipgloss/v2"

// ContextItem represents a kubeconfig context entry, viewable and enterable.
type ContextItem struct {
	*RowItem
	enter  func() (Folder, error)
	viewFn ViewContentFunc
}

func NewContextItem(id string, cells []string, path []string, style *lipgloss.Style, enter func() (Folder, error)) *ContextItem {
	return &ContextItem{RowItem: NewRowItem(id, cells, path, style), enter: enter}
}

func (c *ContextItem) Enter() (Folder, error) {
	if c.enter == nil {
		return nil, nil
	}
	return c.enter()
}

func (c *ContextItem) WithViewContent(fn ViewContentFunc) *ContextItem {
	c.viewFn = fn
	return c
}

func (c *ContextItem) ViewContent() (string, string, string, string, string, error) {
	if c.viewFn == nil {
		return "", "", "", "", "", ErrNoViewContent
	}
	return c.viewFn()
}

// ContextListItem lists contexts and is enterable only.
type ContextListItem struct {
	*RowItem
	enter    func() (Folder, error)
	contexts func() []string
}

func NewContextListItem(id string, cells []string, path []string, style *lipgloss.Style, contextsFn func() []string, enter func() (Folder, error)) *ContextListItem {
	return &ContextListItem{RowItem: NewRowItem(id, cells, path, style), enter: enter, contexts: contextsFn}
}

func (c *ContextListItem) Enter() (Folder, error) {
	if c.enter == nil {
		return nil, nil
	}
	return c.enter()
}

func (c *ContextListItem) Count() int {
	if c.contexts == nil {
		return 0
	}
	return len(c.contexts())
}

func (c *ContextListItem) Empty() bool {
	if c.contexts == nil {
		return true
	}
	return len(c.contexts()) == 0
}
