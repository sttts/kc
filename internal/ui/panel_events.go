package ui

import panelcontent "github.com/sttts/kc/internal/ui/panelcontent"

// PanelSelectionChangedMsg is emitted when the selection within a panel changes.
type PanelSelectionChangedMsg struct {
	Panel     *Panel
	Selection panelcontent.Selection
}

// PanelModeSelectedMsg is emitted when the panel mode chooser confirms a selection.
type PanelModeSelectedMsg struct {
	PanelIndex int
	Mode       PanelViewMode
}
