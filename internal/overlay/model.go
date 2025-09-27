package overlay

import (
	tea "github.com/charmbracelet/bubbletea/v2"
)

// Model composes Foreground over Background at the requested position/offsets.
type viewer interface{ View() (string, *tea.Cursor) }

// Model composes Foreground over Background at the requested position/offsets.
type Model struct {
	Foreground viewer
	Background viewer
	XPosition  Position
	YPosition  Position
	XOffset    int
	YOffset    int
}

func New(fore viewer, back viewer, xPos Position, yPos Position, xOff int, yOff int) *Model {
	return &Model{
		Foreground: fore,
		Background: back,
		XPosition:  xPos,
		YPosition:  yPos,
		XOffset:    xOff,
		YOffset:    yOff,
	}
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }

func (m *Model) View() string {
	if m.Foreground == nil && m.Background == nil {
		return ""
	}
	if m.Foreground == nil {
		s, _ := m.Background.View()
		return s
	}
	if m.Background == nil {
		s, _ := m.Foreground.View()
		return s
	}
	fg, _ := m.Foreground.View()
	bg, _ := m.Background.View()
	return Composite(
		fg,
		bg,
		m.XPosition, m.YPosition,
		m.XOffset, m.YOffset,
	)
}
