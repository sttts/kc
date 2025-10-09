package ui

import (
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"strings"
)

// ObjectOptionsModel controls object list display options (table mode, columns, order).
type ObjectOptionsModel struct {
	width, height int
	focus         int // 0: table mode, 1: columns, 2: order
	modeIdx       int // 0=scroll, 1=fit
	columnsIdx    int // 0=normal, 1=wide
	orderIdx      int // 0=name,1=-name,2=creation,3=-creation
}

var objModeLabels = []string{"Scroll", "Fit"}
var objModeKeys = []string{"scroll", "fit"}
var objColumnsLabels = []string{"Normal", "Wide"}
var objColumnsKeys = []string{"normal", "wide"}
var objOrderLabels = []string{"Name", "-Name", "Creation", "-Creation"}
var objOrderKeys = []string{"name", "-name", "creation", "-creation"}

func NewObjectOptionsModel(mode, columns, order string) *ObjectOptionsModel {
	m := &ObjectOptionsModel{}
	if mode == "fit" {
		m.modeIdx = 1
	}
	if columns == "wide" {
		m.columnsIdx = 1
	}
	switch order {
	case "name":
		m.orderIdx = 0
	case "-name":
		m.orderIdx = 1
	case "creation":
		m.orderIdx = 2
	case "-creation":
		m.orderIdx = 3
	default:
		m.orderIdx = 0
	}
	return m
}

func (m *ObjectOptionsModel) Init() tea.Cmd          { return nil }
func (m *ObjectOptionsModel) SetDimensions(w, h int) { m.width, m.height = w, h }

type ObjectOptionsChangedMsg struct {
	TableMode    string
	Columns      string
	ObjectsOrder string
	Accept       bool
	Close        bool
	SaveDefault  bool
}

func (m *ObjectOptionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch t := msg.(type) {
	case tea.KeyMsg:
		switch t.String() {
		case "up", "k":
			if m.focus > 0 {
				m.focus--
			}
		case "down", "j":
			if m.focus < 2 {
				m.focus++
			}
		case "left", "right", " ", "space":
			switch m.focus {
			case 0: // mode
				if m.modeIdx == 0 {
					m.modeIdx = 1
				} else {
					m.modeIdx = 0
				}
			case 1: // columns
				if m.columnsIdx == 0 {
					m.columnsIdx = 1
				} else {
					m.columnsIdx = 0
				}
			case 2: // order
				if t.String() == "left" {
					if m.orderIdx > 0 {
						m.orderIdx--
					} else {
						m.orderIdx = len(objOrderKeys) - 1
					}
				} else {
					if m.orderIdx < len(objOrderKeys)-1 {
						m.orderIdx++
					} else {
						m.orderIdx = 0
					}
				}
			}
			return m, nil
		case "ctrl+s":
			return m, func() tea.Msg {
				return ObjectOptionsChangedMsg{TableMode: objModeKeys[m.modeIdx], Columns: objColumnsKeys[m.columnsIdx], ObjectsOrder: objOrderKeys[m.orderIdx], SaveDefault: true}
			}
		case "enter":
			return m, func() tea.Msg {
				return ObjectOptionsChangedMsg{TableMode: objModeKeys[m.modeIdx], Columns: objColumnsKeys[m.columnsIdx], ObjectsOrder: objOrderKeys[m.orderIdx], Accept: true, Close: true}
			}
		}
	}
	return m, nil
}

func (m *ObjectOptionsModel) View() string {
	labels := []string{"Table mode", "Columns", "Objects order"}
	values := []string{objModeLabels[m.modeIdx], objColumnsLabels[m.columnsIdx], objOrderLabels[m.orderIdx]}
	maxLabel := 0
	for _, l := range labels {
		if w := lipgloss.Width(l); w > maxLabel {
			maxLabel = w
		}
	}
	rowStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(ColorModalBg)).
		Foreground(lipgloss.Color(ColorModalFg))
	focusStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(ColorModalSelBg)).
		Foreground(lipgloss.Color(ColorModalFg)).
		Bold(true)
	rows := make([]string, 0, len(labels))
	for i := range labels {
		marker := " "
		st := rowStyle
		if i == m.focus {
			marker = ">"
			st = focusStyle
		}
		lpad := labels[i]
		if w := lipgloss.Width(lpad); w < maxLabel {
			lpad = lpad + strings.Repeat(" ", maxLabel-w)
		}
		s := marker + " " + lpad + ": " + values[i]
		rows = append(rows, st.Width(m.width).Render(s))
	}
	for len(rows) < m.height {
		rows = append(rows, rowStyle.Width(m.width).Render(""))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m *ObjectOptionsModel) FooterHints() [][2]string {
	return [][2]string{{"Up/Down", "Move"}, {"Left/Right/Space", "Toggle"}, {"Ctrl+S", "Save as defaults"}, {"Enter", "Apply & Close"}, {"Esc", "Cancel"}}
}
