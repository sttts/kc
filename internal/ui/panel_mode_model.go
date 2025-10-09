package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
)

// PanelModeModel is a simple selector for panel modes.
type PanelModeModel struct {
	panelIdx int
	modes    []PanelViewMode
	cursor   int
	width    int
	height   int
}

func NewPanelModeModel(panelIdx int, modes []PanelViewMode, active PanelViewMode) *PanelModeModel {
	cur := 0
	for i := range modes {
		if modes[i] == active {
			cur = i
			break
		}
	}
	return &PanelModeModel{
		panelIdx: panelIdx,
		modes:    append([]PanelViewMode(nil), modes...),
		cursor:   cur,
	}
}

func (m *PanelModeModel) Init() tea.Cmd { return nil }

func (m *PanelModeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch t := msg.(type) {
	case tea.KeyMsg:
		switch t.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.modes)-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.modes) {
				mode := m.modes[m.cursor]
				return m, func() tea.Msg {
					return PanelModeSelectedMsg{PanelIndex: m.panelIdx, Mode: mode}
				}
			}
			return m, nil
		case "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *PanelModeModel) View() string {
	lines := make([]string, len(m.modes))
	for i, mode := range m.modes {
		label := modeLabel(mode)
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		lines[i] = prefix + label
		if m.width > 0 {
			lines[i] = trimToWidth(lines[i], m.width)
		}
	}
	view := strings.Join(lines, "\n")
	if m.height > 0 && len(lines) < m.height {
		view += strings.Repeat("\n", m.height-len(lines))
	}
	return view
}

func (m *PanelModeModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
}

func modeLabel(mode PanelViewMode) string {
	switch mode {
	case PanelModeList:
		return "List"
	case PanelModeDescribe:
		return "Describe"
	case PanelModeManifest:
		return "Manifest"
	case PanelModeFile:
		return "File"
	default:
		return "Unknown"
	}
}

func trimToWidth(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	return s[:width]
}
