package ui

import (
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
)

// PanelViewMode identifies the active widget/view embedded inside a panel.
type PanelViewMode int

const (
	PanelModeList PanelViewMode = iota
	PanelModeDescribe
	PanelModeManifest
)

// PanelWidget abstracts panel content implementations.
type PanelWidget = panelcontent.Widget

// PanelWidgetFactory constructs a widget bound to the provided panel shell.
type PanelWidgetFactory func(*Panel) panelcontent.Widget

var panelModeOrder = []PanelViewMode{PanelModeList, PanelModeDescribe, PanelModeManifest}

// PanelMouseType identifies mouse intents routed to widgets.
type PanelMouseType = panelcontent.MouseIntent

const (
	PanelMouseClick PanelMouseType = panelcontent.MouseIntentClick
	PanelMouseWheel PanelMouseType = panelcontent.MouseIntentWheel
)

// PanelMouseMsg conveys mouse events with panel-relative context.
type PanelMouseMsg = panelcontent.MouseMsg

// NextPanelMode returns the next mode in the default ordering.
func NextPanelMode(current PanelViewMode) PanelViewMode {
	for i, mode := range panelModeOrder {
		if mode == current {
			return panelModeOrder[(i+1)%len(panelModeOrder)]
		}
	}
	return panelModeOrder[0]
}

// PanelModeOrder returns the canonical ordering of panel modes.
func PanelModeOrder() []PanelViewMode {
	return append([]PanelViewMode(nil), panelModeOrder...)
}
