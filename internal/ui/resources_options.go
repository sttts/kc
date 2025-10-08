package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

const (
	optInclude = iota
	optOrder
	optTableMode
)

type ResourcesOptionsModel struct {
	width, height int
	options       []int
	focus         int

	includeEmpty bool
	orderIdx     int
	tableIdx     int
}

var orderLabels = []string{"Alphabetic", "Group", "Favorites"}
var orderKeys = []string{"alpha", "group", "favorites"}
var tableModeLabels = []string{"Scroll", "Fit"}
var tableModeKeys = []string{"scroll", "fit"}

func NewResourcesOptionsModel(showInclude, showOrder bool, tableMode string, showNonEmpty bool, order string) *ResourcesOptionsModel {
	opts := make([]int, 0, 3)
	if showInclude {
		opts = append(opts, optInclude)
	}
	if showOrder {
		opts = append(opts, optOrder)
	}
	opts = append(opts, optTableMode)

	orderIdx := 0
	switch strings.ToLower(order) {
	case "group":
		orderIdx = 1
	case "favorites":
		orderIdx = 2
	default:
		orderIdx = 0
	}

	tableIdx := 0
	if strings.EqualFold(tableMode, "fit") {
		tableIdx = 1
	}

	return &ResourcesOptionsModel{
		options:      opts,
		includeEmpty: !showNonEmpty,
		orderIdx:     orderIdx,
		tableIdx:     tableIdx,
	}
}

func (m *ResourcesOptionsModel) Init() tea.Cmd          { return nil }
func (m *ResourcesOptionsModel) SetDimensions(w, h int) { m.width, m.height = w, h }

type ResourcesOptionsChangedMsg struct {
	ShowNonEmptyOnly bool
	Order            string
	TableMode        string
	HasInclude       bool
	HasOrder         bool
	Accept           bool
	Close            bool
	SaveDefault      bool
}

func (m *ResourcesOptionsModel) hasOption(opt int) bool {
	for _, existing := range m.options {
		if existing == opt {
			return true
		}
	}
	return false
}

func (m *ResourcesOptionsModel) optionAtFocus() int {
	if len(m.options) == 0 {
		return optTableMode
	}
	if m.focus < 0 {
		m.focus = 0
	}
	if m.focus >= len(m.options) {
		m.focus = len(m.options) - 1
	}
	return m.options[m.focus]
}

func (m *ResourcesOptionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch t := msg.(type) {
	case tea.KeyMsg:
		switch t.String() {
		case "up", "k":
			if m.focus > 0 {
				m.focus--
			}
		case "down", "j":
			if m.focus < len(m.options)-1 {
				m.focus++
			}
		case "left", "right", " ", "space":
			switch m.optionAtFocus() {
			case optInclude:
				m.includeEmpty = !m.includeEmpty
			case optOrder:
				if t.String() == "left" {
					if m.orderIdx > 0 {
						m.orderIdx--
					} else {
						m.orderIdx = len(orderKeys) - 1
					}
				} else {
					if m.orderIdx < len(orderKeys)-1 {
						m.orderIdx++
					} else {
						m.orderIdx = 0
					}
				}
			case optTableMode:
				if t.String() == "left" {
					if m.tableIdx > 0 {
						m.tableIdx--
					} else {
						m.tableIdx = len(tableModeKeys) - 1
					}
				} else {
					if m.tableIdx < len(tableModeKeys)-1 {
						m.tableIdx++
					} else {
						m.tableIdx = 0
					}
				}
			}
		case "ctrl+s":
			return m, func() tea.Msg {
				return ResourcesOptionsChangedMsg{
					ShowNonEmptyOnly: !m.includeEmpty,
					Order:            orderKeys[m.orderIdx],
					TableMode:        tableModeKeys[m.tableIdx],
					HasInclude:       m.hasOption(optInclude),
					HasOrder:         m.hasOption(optOrder),
					SaveDefault:      true,
				}
			}
		case "enter":
			return m, func() tea.Msg {
				return ResourcesOptionsChangedMsg{
					ShowNonEmptyOnly: !m.includeEmpty,
					Order:            orderKeys[m.orderIdx],
					TableMode:        tableModeKeys[m.tableIdx],
					HasInclude:       m.hasOption(optInclude),
					HasOrder:         m.hasOption(optOrder),
					Accept:           true,
					Close:            true,
				}
			}
		}
	}
	return m, nil
}

func (m *ResourcesOptionsModel) View() string {
	if len(m.options) == 0 {
		return ""
	}
	labels := make([]string, 0, len(m.options))
	values := make([]string, 0, len(m.options))
	for _, opt := range m.options {
		switch opt {
		case optInclude:
			labels = append(labels, "Include empty")
			if m.includeEmpty {
				values = append(values, "Yes")
			} else {
				values = append(values, "No")
			}
		case optOrder:
			labels = append(labels, "Order")
			values = append(values, orderLabels[m.orderIdx])
		case optTableMode:
			labels = append(labels, "Table mode")
			values = append(values, tableModeLabels[m.tableIdx])
		}
	}

	maxLabel := 0
	for _, l := range labels {
		if w := lipgloss.Width(l); w > maxLabel {
			maxLabel = w
		}
	}

	rowStyle := lipgloss.NewStyle().Background(lipgloss.Color("250")).Foreground(lipgloss.Black)
	focusStyle := lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.White).Bold(true)

	rows := make([]string, 0, len(labels))
	for i := range labels {
		marker := " "
		st := rowStyle
		if i == m.focus {
			marker = ">"
			st = focusStyle
		}
		label := labels[i]
		if w := lipgloss.Width(label); w < maxLabel {
			label += strings.Repeat(" ", maxLabel-w)
		}
		line := marker + " " + label + ": " + values[i]
		rows = append(rows, st.Width(m.width).Render(line))
	}

	for len(rows) < m.height {
		rows = append(rows, rowStyle.Width(m.width).Render(""))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m *ResourcesOptionsModel) FooterHints() [][2]string {
	return [][2]string{
		{"Up/Down", "Move"},
		{"Left/Right/Space", "Toggle"},
		{"Ctrl+S", "Save as defaults"},
		{"Enter", "Apply & Close"},
		{"Esc", "Cancel"},
	}
}
