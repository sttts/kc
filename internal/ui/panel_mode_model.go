package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
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
	base := lipgloss.NewStyle().
		Background(lipgloss.Color("238")).
		Foreground(lipgloss.Color("255"))
	sel := lipgloss.NewStyle().
		Background(lipgloss.Cyan).
		Foreground(lipgloss.Color("0")).
		Bold(true)
	lines := make([]string, len(m.modes))
	for i, mode := range m.modes {
		label := modeLabel(mode)
		line := label
		if m.width > 0 {
			line = trimToWidth(line, m.width)
		}
		if i == m.cursor {
			lines[i] = sel.Render(line)
		} else {
			lines[i] = base.Render(line)
		}
	}
	start := 0
	end := len(lines)
	if m.height > 0 && len(lines) > m.height {
		if m.cursor < m.height {
			end = m.height
		} else {
			start = m.cursor - m.height + 1
			end = start + m.height
		}
	}
	view := strings.Join(lines[start:end], "\n")
	if m.height > 0 && (end-start) < m.height {
		view += strings.Repeat("\n", m.height-(end-start))
	}
	return base.Width(m.width).Height(m.height).Render(view)
}

// FooterHints supplies modal footer hints.
func (m *PanelModeModel) FooterHints() [][2]string {
	return [][2]string{
		{"Enter", "Apply"},
		{"Esc", "Cancel"},
	}
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
