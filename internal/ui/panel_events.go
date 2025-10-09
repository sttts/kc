package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea/v2"
)

// PanelSelectionChangedMsg is emitted when the selection within a panel changes.
type PanelSelectionChangedMsg struct {
	Panel       *Panel
	SelectionID string
	Path        string
}

func (p *Panel) selectionChangedCmd(ctx context.Context) tea.Cmd {
	if p == nil {
		return nil
	}
	currentID := p.currentSelectionID(ctx)
	if currentID == p.lastSelectionID {
		return nil
	}
	p.lastSelectionID = currentID
	panel := p
	path := panel.currentPath
	return func() tea.Msg {
		return PanelSelectionChangedMsg{
			Panel:       panel,
			SelectionID: currentID,
			Path:        path,
		}
	}
}
