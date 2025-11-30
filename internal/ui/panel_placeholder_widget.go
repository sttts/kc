package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
	uistyles "github.com/sttts/kc/internal/ui/styles"
)

// placeholderWidget renders a static message for modes that aren't implemented yet.
type placeholderWidget struct {
	panel   *Panel
	message string
	crumb   string
}

func newPlaceholderWidget(panel *Panel, msg string, breadcrumb string) panelcontent.Widget {
	return &placeholderWidget{panel: panel, message: msg, crumb: breadcrumb}
}

func (w *placeholderWidget) Init(context.Context) tea.Cmd { return nil }

func (w *placeholderWidget) Update(context.Context, tea.Msg) (tea.Cmd, bool) {
	return nil, false
}

func (w *placeholderWidget) View(ctx context.Context, frame panelcontent.Frame) tea.View {
	if w.panel == nil {
		return tea.NewView("")
	}
	width := frame.Size.Width
	if width <= 0 {
		width = w.panel.width
	}
	if width <= 0 {
		width = 1
	}
	height := frame.Size.Height
	if height <= 0 {
		height = w.panel.height
	}
	if height <= 0 {
		height = 1
	}
	content := w.message
	if content == "" {
		content = "Mode not yet available"
	}
	style := uistyles.PanelContentStyle.
		Width(width).
		Height(height).
		Background(lipgloss.Color(uistyles.ColorBlack)).
		Foreground(lipgloss.Color(uistyles.ColorWhite)).
		Padding(0, 1)
	if frame.Focused {
		style = style.Copy().Bold(true)
	}
	return tea.NewView(style.Render(content))
}

func (w *placeholderWidget) Resize(context.Context, panelcontent.Size) {}

func (w *placeholderWidget) SetFocus(context.Context, bool) {}

func (w *placeholderWidget) Teardown(context.Context) {}

func (w *placeholderWidget) FrameInfo(context.Context, panelcontent.FrameInfoRequest) panelcontent.FrameInfo {
	crumb := w.crumb
	if crumb == "" {
		crumb = w.message
	}
	return panelcontent.FrameInfo{
		Breadcrumb:      fmt.Sprintf("%s", crumb),
		SuppressFooter:  true,
		TopIndicator:    "─",
		BottomIndicator: "─",
	}
}
