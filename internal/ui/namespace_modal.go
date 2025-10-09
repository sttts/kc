package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"k8s.io/apimachinery/pkg/util/validation"
)

// NamespaceCreateResultMsg signals the outcome of the namespace creation dialog.
type NamespaceCreateResultMsg struct {
	Name    string
	Confirm bool
	Close   bool
}

// NamespaceCreateModel provides a minimal inline text input for namespace name.
type NamespaceCreateModel struct {
	width, height int
	runes         []rune
	cursor        int
	err           string
	buttons       []buttonRect
}

// NewNamespaceCreateModel constructs an empty namespace creation dialog model.
func NewNamespaceCreateModel() *NamespaceCreateModel {
	return &NamespaceCreateModel{}
}

func (m *NamespaceCreateModel) Init() tea.Cmd          { return nil }
func (m *NamespaceCreateModel) SetDimensions(w, h int) { m.width, m.height = w, h }

// Reset clears the input state.
func (m *NamespaceCreateModel) Reset() {
	m.runes = m.runes[:0]
	m.cursor = 0
	m.err = ""
	m.buttons = nil
}

func (m *NamespaceCreateModel) value() string { return string(m.runes) }

func (m *NamespaceCreateModel) insertRunes(rs []rune) {
	if len(rs) == 0 {
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.runes) {
		m.cursor = len(m.runes)
	}
	before := append([]rune{}, m.runes[:m.cursor]...)
	after := append([]rune{}, m.runes[m.cursor:]...)
	m.runes = append(before, append(rs, after...)...)
	m.cursor += len(rs)
	m.err = ""
}

func (m *NamespaceCreateModel) deleteBackward() {
	if m.cursor <= 0 || len(m.runes) == 0 {
		return
	}
	m.runes = append(m.runes[:m.cursor-1], m.runes[m.cursor:]...)
	m.cursor--
}

func (m *NamespaceCreateModel) deleteForward() {
	if m.cursor < 0 || m.cursor >= len(m.runes) {
		return
	}
	m.runes = append(m.runes[:m.cursor], m.runes[m.cursor+1:]...)
}

func (m *NamespaceCreateModel) clampCursor() {
	if m.cursor < 0 {
		m.cursor = 0
	} else if m.cursor > len(m.runes) {
		m.cursor = len(m.runes)
	}
}

func (m *NamespaceCreateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		switch key.String() {
		case "ctrl+c", "ctrl+g", "esc":
			return m, func() tea.Msg { return NamespaceCreateResultMsg{Close: true} }
		case "ctrl+h":
			m.deleteBackward()
			return m, nil
		}
		k := key.Key()
		switch k.Code {
		case tea.KeyEnter:
			name := strings.TrimSpace(m.value())
			if name == "" {
				m.err = "Name is required"
				return m, nil
			}
			if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
				m.err = errs[0]
				return m, nil
			}
			return m, func() tea.Msg {
				return NamespaceCreateResultMsg{
					Name:    name,
					Confirm: true,
					Close:   true,
				}
			}
		case tea.KeyBackspace:
			m.deleteBackward()
			return m, nil
		case tea.KeyDelete:
			m.deleteForward()
			return m, nil
		case tea.KeyLeft:
			m.cursor--
			m.clampCursor()
			return m, nil
		case tea.KeyRight:
			m.cursor++
			m.clampCursor()
			return m, nil
		case tea.KeyHome:
			m.cursor = 0
			return m, nil
		case tea.KeyEnd:
			m.cursor = len(m.runes)
			return m, nil
		}
		if text := k.Text; text != "" {
			if k.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModMeta|tea.ModSuper|tea.ModHyper) == 0 {
				m.insertRunes([]rune(text))
			}
		}
		return m, nil
	case tea.MouseMsg:
		mouse := key.Mouse()
		if mouse.Button != tea.MouseLeft {
			return m, nil
		}
		for idx, r := range m.buttons {
			if !r.contains(mouse.X, mouse.Y) {
				continue
			}
			if _, ok := msg.(tea.MouseClickMsg); ok {
				// Focus change not tracked yet; ignore.
				return m, nil
			}
			if _, ok := msg.(tea.MouseReleaseMsg); ok {
				return m, m.executeButton(idx)
			}
		}
	}
	return m, nil
}

func (m *NamespaceCreateModel) executeButton(idx int) tea.Cmd {
	switch idx {
	case 0: // create
		name := strings.TrimSpace(m.value())
		if name == "" {
			m.err = "Name is required"
			return nil
		}
		if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
			m.err = errs[0]
			return nil
		}
		return func() tea.Msg {
			return NamespaceCreateResultMsg{Name: name, Confirm: true, Close: true}
		}
	case 1: // cancel
		return func() tea.Msg { return NamespaceCreateResultMsg{Confirm: false, Close: true} }
	default:
		return nil
	}
}

func (m *NamespaceCreateModel) View() string {
	innerWidth := max(30, m.width-4)
	bg := lipgloss.NewStyle().
		Background(lipgloss.Color(ColorModalBg)).
		Foreground(lipgloss.Color(ColorModalFg)).
		Width(innerWidth)

	header := bg.Copy().
		Bold(true).
		Align(lipgloss.Center).
		Render("Enter new namespace name")

	fieldWidth := max(24, innerWidth-6)
	inputField := bg.Copy().
		Align(lipgloss.Center).
		Render(m.renderInput(fieldWidth))

	buttons := []string{
		m.renderButton("Create"),
		m.renderButton("Cancel"),
	}
	separator := lipgloss.NewStyle().
		Background(lipgloss.Color(ColorModalBg)).
		Render(" ")
	buttonRow := lipgloss.JoinHorizontal(lipgloss.Center, buttons[0], separator, buttons[1])
	buttonRowView := bg.Copy().Align(lipgloss.Center).Render(buttonRow)
	sepWidth := lipgloss.Width(separator)
	leftPad := max(0, (innerWidth-lipgloss.Width(buttonRow))/2)
	buttonLine := 4 // header (0), blank (1), input (2), blank (3), buttons (4)
	m.buttons = []buttonRect{
		{x: leftPad, y: buttonLine, w: lipgloss.Width(buttons[0]), h: 1},
		{x: leftPad + lipgloss.Width(buttons[0]) + sepWidth, y: buttonLine, w: lipgloss.Width(buttons[1]), h: 1},
	}

	help := bg.Copy().
		Faint(true).
		Align(lipgloss.Center).
		Render("Enter: Create • Esc: Cancel")

	lines := []string{
		header,
		bg.Copy().Render(""),
		inputField,
		bg.Copy().Render(""),
		buttonRowView,
		bg.Copy().Render(""),
		help,
	}
	if m.err != "" {
		errLine := bg.Copy().
			Foreground(lipgloss.Color(ColorModalSelBg)).
			Render(m.err)
		lines = append(lines, bg.Copy().Render(""), errLine)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m *NamespaceCreateModel) renderButton(label string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorModalFg)).
		Background(lipgloss.Color(ColorModalBg)).
		Padding(0, 3).
		Align(lipgloss.Center).
		Render(label)
}

func (m *NamespaceCreateModel) renderInput(fieldWidth int) string {
	if fieldWidth <= 0 {
		fieldWidth = 1
	}
	cursorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorWhite)).
		Background(lipgloss.Color(ColorModalSelBg)).
		Bold(true)
	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorWhite)).
		Background(lipgloss.Color(ColorDarkGrey))
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.runes) {
		m.cursor = len(m.runes)
	}

	display := m.runes
	cursor := m.cursor
	if len(display) > fieldWidth {
		display = display[:fieldWidth]
		if cursor > fieldWidth {
			cursor = fieldWidth
		}
	}

	var b strings.Builder
	for i := 0; i < fieldWidth; i++ {
		var ch string
		if i < len(display) {
			ch = string(display[i])
		} else {
			ch = " "
		}
		if i == cursor {
			b.WriteString(cursorStyle.Render(ch))
		} else {
			b.WriteString(textStyle.Render(ch))
		}
	}
	return b.String()
}

// FooterHints wires the modal footer hints.
func (m *NamespaceCreateModel) FooterHints() [][2]string {
	return [][2]string{{"Enter", "Create"}, {"Esc", "Cancel"}}
}
