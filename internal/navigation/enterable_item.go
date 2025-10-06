package navigation

import (
	"strings"

	lipgloss "github.com/charmbracelet/lipgloss/v2"
	"github.com/sttts/kc/internal/navigation/models"
)

// EnterableItem is a reusable Item that is also Enterable via a provided function.
type EnterableItem struct {
	*models.RowItem
	enter  func() (Folder, error)
	viewFn ViewContentFunc
}

var _ Item = (*EnterableItem)(nil)
var _ Enterable = (*EnterableItem)(nil)

func NewEnterableItem(id string, cells []string, path []string, enter func() (Folder, error), style *lipgloss.Style) *EnterableItem {
	// Ensure enterable items show a leading "/" in the first cell for generic folder UI
	if len(cells) > 0 && !strings.HasPrefix(cells[0], "/") {
		cloned := append([]string(nil), cells...)
		cloned[0] = "/" + cloned[0]
		cells = cloned
	}
	return &EnterableItem{RowItem: models.NewRowItem(id, cells, path, style), enter: enter}
}

// NewEnterableItemStyled constructs an EnterableItem with per-cell styles.
func NewEnterableItemStyled(id string, cells []string, path []string, styles []*lipgloss.Style, enter func() (Folder, error)) *EnterableItem {
	if len(cells) > 0 && !strings.HasPrefix(cells[0], "/") {
		cloned := append([]string(nil), cells...)
		cloned[0] = "/" + cloned[0]
		cells = cloned
	}
	return &EnterableItem{RowItem: models.NewRowItemStyled(id, cells, path, styles), enter: enter}
}

func (e *EnterableItem) Enter() (Folder, error) {
	if e.enter == nil {
		return nil, nil
	}
	return e.enter()
}
func (e *EnterableItem) WithViewContent(fn ViewContentFunc) *EnterableItem { e.viewFn = fn; return e }
func (e *EnterableItem) ViewContent() (string, string, string, string, string, error) {
	if e.viewFn == nil {
		return "", "", "", "", "", ErrNoViewContent
	}
	return e.viewFn()
}
