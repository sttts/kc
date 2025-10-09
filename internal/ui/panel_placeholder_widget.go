package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea/v2"
)

// placeholderWidget renders a static message for modes that aren't implemented yet.
type placeholderWidget struct {
	panel   *Panel
	message string
}

func newPlaceholderWidget(panel *Panel, msg string) PanelWidget {
	return &placeholderWidget{panel: panel, message: msg}
}

func (w *placeholderWidget) Init(context.Context) tea.Cmd { return nil }

func (w *placeholderWidget) Update(context.Context, tea.Msg) (tea.Cmd, bool) {
	return nil, false
}

func (w *placeholderWidget) View(ctx context.Context, focused bool) string {
	if w.panel == nil {
		return ""
	}
	content := w.message
	if content == "" {
		content = "Mode not yet available"
	}
	style := PanelContentStyle.Width(max(1, w.panel.width)).Height(max(1, w.panel.height))
	if focused {
		style = style.Copy().Bold(true)
	}
	return style.Render(content)
}

func (w *placeholderWidget) Resize(context.Context, int, int) {}

func (w *placeholderWidget) SetFocus(context.Context, bool) {}
