package overlay

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// Model composes Foreground over Background at the requested position/offsets.
type viewer interface{ View() tea.View }

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

func (m *Model) View() tea.View {
	background := func() tea.View {
		if m.Background == nil {
			return tea.NewView("")
		}
		return m.Background.View()
	}()

	foreground := func() tea.View {
		if m.Foreground == nil {
			return tea.NewView("")
		}
		return m.Foreground.View()
	}()

	fg := viewString(foreground)
	bg := viewString(background)

	return tea.NewView(Composite(
		fg,
		bg,
		m.XPosition, m.YPosition,
		m.XOffset, m.YOffset,
	))
}

func viewString(view tea.View) string {
	if view.Content == nil {
		return ""
	}
	return fmt.Sprint(view.Content)
}
