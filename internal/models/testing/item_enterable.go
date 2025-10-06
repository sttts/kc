package modeltesting

import (
	"strings"

	lipgloss "github.com/charmbracelet/lipgloss/v2"
	navmodels "github.com/sttts/kc/internal/models"
)

// EnterableItem is a reusable Item that is also Enterable via a provided function.
type EnterableItem struct {
	*navmodels.RowItem
	enter  func() (navmodels.Folder, error)
	viewFn navmodels.ViewContentFunc
}

var _ navmodels.Item = (*EnterableItem)(nil)
var _ navmodels.Enterable = (*EnterableItem)(nil)

func NewEnterableItem(id string, cells []string, path []string, enter func() (navmodels.Folder, error), style *lipgloss.Style) *EnterableItem {
	if len(cells) > 0 && !strings.HasPrefix(cells[0], "/") {
		cloned := append([]string(nil), cells...)
		cloned[0] = "/" + cloned[0]
		cells = cloned
	}
	return &EnterableItem{RowItem: navmodels.NewRowItem(id, cells, path, style), enter: enter}
}

func NewEnterableItemStyled(id string, cells []string, path []string, styles []*lipgloss.Style, enter func() (navmodels.Folder, error)) *EnterableItem {
	if len(cells) > 0 && !strings.HasPrefix(cells[0], "/") {
		cloned := append([]string(nil), cells...)
		cloned[0] = "/" + cloned[0]
		cells = cloned
	}
	return &EnterableItem{RowItem: navmodels.NewRowItemStyled(id, cells, path, styles), enter: enter}
}

func (e *EnterableItem) Enter() (navmodels.Folder, error) {
	if e.enter == nil {
		return nil, nil
	}
	return e.enter()
}

func (e *EnterableItem) WithViewContent(fn navmodels.ViewContentFunc) *EnterableItem {
	e.viewFn = fn
	return e
}

func (e *EnterableItem) ViewContent() (string, string, string, string, string, error) {
	if e.viewFn == nil {
		return "", "", "", "", "", navmodels.ErrNoViewContent
	}
	return e.viewFn()
}
