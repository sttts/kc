package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

// DeleteConfirmMsg signals the result of the delete confirmation dialog.
type DeleteConfirmMsg struct {
	Confirm bool
	Close   bool
}

// DeleteConfirmModel renders a simple Yes/No confirmation prompt.
type DeleteConfirmModel struct {
	width, height int
	target        string
	namespace     string
	focus         int // 0=yes, 1=no
	buttonRect    [2]rect
}

type rect struct{ x, y, w, h int }

func NewDeleteConfirmModel() *DeleteConfirmModel {
	return &DeleteConfirmModel{focus: 1}
}

func (m *DeleteConfirmModel) Init() tea.Cmd          { return nil }
func (m *DeleteConfirmModel) SetDimensions(w, h int) { m.width, m.height = w, h }

// Configure sets the resource details displayed in the dialog.
func (m *DeleteConfirmModel) Configure(target, namespace string) {
	m.target = target
	m.namespace = namespace
	m.focus = 1 // default to "No"
}

func (m *DeleteConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		switch strings.ToLower(key.String()) {
		case "esc", "ctrl+c", "ctrl+g":
			return m, func() tea.Msg { return DeleteConfirmMsg{Confirm: false, Close: true} }
		case "shift+tab":
			m.focus = (m.focus + 1) % 2
			return m, nil
		case "y":
			return m, func() tea.Msg { return DeleteConfirmMsg{Confirm: true, Close: true} }
		case "n":
			return m, func() tea.Msg { return DeleteConfirmMsg{Confirm: false, Close: true} }
		}
		k := key.Key()
		switch k.Code {
		case tea.KeyEnter:
			return m, func() tea.Msg {
				return DeleteConfirmMsg{
					Confirm: m.focus == 0,
					Close:   true,
				}
			}
		case tea.KeyLeft, tea.KeyRight, tea.KeyTab:
			m.focus = (m.focus + 1) % 2
			return m, nil
		}
	case tea.MouseMsg:
		switch ev := msg.(type) {
		case tea.MouseClickMsg, tea.MouseReleaseMsg:
			pos := ev.Mouse()
			for idx, r := range m.buttonRect {
				if r.contains(pos.X, pos.Y) {
					m.focus = idx
					if _, ok := ev.(tea.MouseReleaseMsg); ok {
						return m, func() tea.Msg {
							return DeleteConfirmMsg{
								Confirm: idx == 0,
								Close:   true,
							}
						}
					}
					return m, nil
				}
			}
		}
	}
	return m, nil
}

func (m *DeleteConfirmModel) View() string {
	innerWidth := max(30, m.width-4)
	const buttonWidth = 8
	for i := range m.buttonRect {
		m.buttonRect[i] = rect{}
	}
	bg := lipgloss.NewStyle().
		Background(lipgloss.Color("250")).
		Foreground(lipgloss.Black).
		Width(innerWidth)
	title := fmt.Sprintf("Delete %s?", m.target)
	if m.namespace != "" {
		title = fmt.Sprintf("Delete %s in namespace %q?", m.target, m.namespace)
	}
	titleView := bg.Copy().Bold(true).Align(lipgloss.Center).Render(title)
	helpView := bg.Copy().Faint(true).Align(lipgloss.Center).Render("←/→ Switch • Enter: Confirm • Esc: Cancel")
	options := []string{
		m.renderOption("Yes", buttonWidth, m.focus == 0),
		m.renderOption("No", buttonWidth, m.focus != 0),
	}
	separator := lipgloss.NewStyle().
		Background(lipgloss.Color("250")).
		Render(" ")
	bodyRow := lipgloss.JoinHorizontal(lipgloss.Center, options[0], separator, options[1])
	leftX := (innerWidth / 2) - (buttonWidth + 1)
	if leftX < 0 {
		leftX = 0
	}
	bodyY := m.height/2 + 1
	m.buttonRect[0] = rect{x: leftX, y: bodyY, w: buttonWidth, h: 1}
	m.buttonRect[1] = rect{x: leftX + buttonWidth + 1, y: bodyY, w: buttonWidth, h: 1}
	bodyView := bg.Copy().Align(lipgloss.Center).Render(bodyRow)
	spacer := bg.Copy().Render("")
	return lipgloss.JoinVertical(lipgloss.Left, titleView, spacer, bodyView, spacer, helpView)
}

func (m *DeleteConfirmModel) renderOption(label string, width int, focused bool) string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("240")).
		Width(width).
		Align(lipgloss.Center)
	if focused {
		style = style.
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("203")).
			Bold(true)
	} else {
		style = style.
			Foreground(lipgloss.Color("0"))
	}
	return style.Render(label)
}

func (m *DeleteConfirmModel) FooterHints() [][2]string {
	return [][2]string{{"Enter", "Confirm"}, {"Esc", "Cancel"}}
}

type rect struct{ x, y, w, h int }

func (r rect) contains(px, py int) bool {
	return px >= r.x && px < r.x+r.w && py >= r.y && py < r.y+r.h
}
