package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	textinput "github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	uistyles "github.com/sttts/kc/internal/ui/styles"
)

// CopyToLocalResultMsg captures the outcome of the copy dialog.
type CopyToLocalResultMsg struct {
	Path    string
	Confirm bool
	Close   bool
}

type copyFieldFocus int

const (
	copyFocusPath copyFieldFocus = iota
	copyFocusConfirm
	copyFocusCancel
)

// CopyToLocalModel renders the "Copy to local" dialog with a single path input.
type CopyToLocalModel struct {
	width, height int
	subject       string
	pathInput     textinput.Model
	focus         copyFieldFocus
	err           string
	buttons       [2]buttonRect
}

func newCopyTextInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	styles := textinput.DefaultLightStyles()
	base := lipgloss.NewStyle().Foreground(lipgloss.Color(uistyles.ColorModalFg))
	styles.Focused.Text = base
	styles.Focused.Prompt = base
	styles.Focused.Placeholder = base.Copy().Foreground(lipgloss.Color(uistyles.ColorDarkGrey))
	styles.Cursor.Color = lipgloss.Color(uistyles.ColorModalSelBg)
	styles.Blurred = styles.Focused
	ti.Styles = styles
	ti.VirtualCursor = false
	ti.CharLimit = 0
	ti.Placeholder = ""
	ti.Blur()
	return ti
}

// NewCopyToLocalModel constructs an empty copy dialog model.
func NewCopyToLocalModel() *CopyToLocalModel {
	model := &CopyToLocalModel{
		pathInput: newCopyTextInput(),
		focus:     copyFocusPath,
	}
	return model
}

func (m *CopyToLocalModel) Init() tea.Cmd          { return nil }
func (m *CopyToLocalModel) SetDimensions(w, h int) { m.width, m.height = w, h }

// Configure sets the dialog subject and default path.
func (m *CopyToLocalModel) Configure(subject, path string) {
	m.subject = subject
	m.err = ""
	m.buttons = [2]buttonRect{}
	m.setPath(path)
	m.setFocus(copyFocusPath)
}

// FocusPath focuses the file path input.
func (m *CopyToLocalModel) FocusPath() tea.Cmd { return m.setFocus(copyFocusPath) }

// BlurInputs blurs the input control.
func (m *CopyToLocalModel) BlurInputs() {
	m.pathInput.Blur()
}

func (m *CopyToLocalModel) setFocus(next copyFieldFocus) tea.Cmd {
	m.focus = next
	switch next {
	case copyFocusPath:
		return m.pathInput.Focus()
	default:
		m.pathInput.Blur()
		return nil
	}
}

func (m *CopyToLocalModel) setPath(p string) {
	p = expandedPath(strings.TrimSpace(p))
	if p == "" {
		p = "resource.txt"
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	m.pathInput.SetValue(p)
}

func (m *CopyToLocalModel) pathValue() string { return strings.TrimSpace(m.pathInput.Value()) }

func (m *CopyToLocalModel) moveFocus(delta int) tea.Cmd {
	total := 3
	next := (int(m.focus) + delta) % total
	if next < 0 {
		next += total
	}
	return m.setFocus(copyFieldFocus(next))
}

func (m *CopyToLocalModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		switch key.String() {
		case "esc", "ctrl+c", "ctrl+g":
			return m, func() tea.Msg { return CopyToLocalResultMsg{Close: true} }
		case "tab":
			return m, m.moveFocus(1)
		case "shift+tab":
			return m, m.moveFocus(-1)
		case "left":
			if m.focus != copyFocusPath {
				return m, m.moveFocus(-1)
			}
		case "right":
			if m.focus != copyFocusPath {
				return m, m.moveFocus(1)
			}
		case "enter":
			if m.focus == copyFocusCancel {
				return m, func() tea.Msg { return CopyToLocalResultMsg{Close: true} }
			}
			return m, m.trySubmit()
		}
		if m.focus == copyFocusPath {
			before := m.pathInput.Value()
			input, cmd := m.pathInput.Update(key)
			m.pathInput = input
			if m.pathInput.Value() != before {
				m.err = ""
			}
			return m, cmd
		}
	case tea.MouseMsg:
		mouse := key.Mouse()
		if mouse.Button != tea.MouseLeft {
			return m, nil
		}
		for idx, rect := range m.buttons {
			if !rect.contains(mouse.X, mouse.Y) {
				continue
			}
			switch msg.(type) {
			case tea.MouseClickMsg:
				if idx == 0 {
					m.setFocus(copyFocusConfirm)
				} else {
					m.setFocus(copyFocusCancel)
				}
				return m, nil
			case tea.MouseReleaseMsg:
				if idx == 0 {
					return m, m.trySubmit()
				}
				return m, func() tea.Msg { return CopyToLocalResultMsg{Close: true} }
			}
		}
	}
	return m, nil
}

func (m *CopyToLocalModel) trySubmit() tea.Cmd {
	path, err := m.validate()
	if err != nil {
		m.err = err.Error()
		return nil
	}
	return func() tea.Msg {
		return CopyToLocalResultMsg{
			Path:    path,
			Confirm: true,
			Close:   true,
		}
	}
}

func (m *CopyToLocalModel) validate() (string, error) {
	target := expandedPath(m.pathValue())
	if target == "" {
		return "", fmt.Errorf("File path is required")
	}
	absPath, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("File path: %w", err)
	}
	if !filepath.IsAbs(absPath) {
		return "", fmt.Errorf("File path must be absolute")
	}
	dir := filepath.Dir(absPath)
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("Directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("Directory is not valid")
	}
	if info, err := os.Stat(absPath); err == nil && info.IsDir() {
		return "", fmt.Errorf("File path points to a directory")
	}
	return absPath, nil
}

func (m *CopyToLocalModel) FooterHints() []FooterHint {
	return []FooterHint{
		{Key: "Enter", Label: "Copy", Enabled: true},
		{Key: "Esc", Label: "Cancel", Enabled: true},
	}
}

func (m *CopyToLocalModel) View() (string, *tea.Cursor) {
	innerWidth := max(40, m.width-4)
	bg := lipgloss.NewStyle().
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Foreground(lipgloss.Color(uistyles.ColorModalFg)).
		Width(innerWidth)

	lines := make([]string, 0, 8)
	var cursor *tea.Cursor
	fieldWidth := max(30, innerWidth-6)

	title := fmt.Sprintf("Copy %s to local file", strings.TrimSpace(m.subject))
	lines = append(lines, bg.Copy().Bold(true).Render(title))
	lines = append(lines, bg.Copy().Render(""))

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(uistyles.ColorModalFg)).
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Padding(0, 0, 0, 2)
	inputStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(uistyles.ColorModalFg)).
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Width(innerWidth)

	lines = append(lines, labelStyle.Render("File path"))
	cursor = m.appendInputLine(&lines, inputStyle, &m.pathInput, cursor, fieldWidth)

	if m.err != "" {
		errStyle := bg.Copy().
			Foreground(lipgloss.Color("196")).
			PaddingLeft(2)
		lines = append(lines, errStyle.Render(m.err))
	} else {
		lines = append(lines, bg.Copy().Render(""))
	}

	lines = append(lines, bg.Copy().Faint(true).Render("Enter an absolute file path. Existing files will be overwritten."))
	lines = append(lines, bg.Copy().Render(""))

	buttonLineIdx := len(lines)
	row, rects := m.renderButtons(innerWidth, buttonLineIdx)
	m.buttons = rects
	lines = append(lines, bg.Copy().Render(row))

	return lipgloss.JoinVertical(lipgloss.Left, lines...), cursor
}

func (m *CopyToLocalModel) appendInputLine(lines *[]string, style lipgloss.Style, input *textinput.Model, cur *tea.Cursor, fieldWidth int) *tea.Cursor {
	input.SetWidth(fieldWidth)
	view := input.View()
	rendered := style.Render("  " + view)
	*lines = append(*lines, rendered)
	lineIdx := len(*lines) - 1
	if m.focus == copyFocusPath {
		if c := input.Cursor(); c != nil {
			cursor := tea.NewCursor(c.X+2, lineIdx)
			cursor.Blink = c.Blink
			cursor.Shape = c.Shape
			cursor.Color = c.Color
			return cursor
		}
	}
	return cur
}

func (m *CopyToLocalModel) renderButtons(width, lineIdx int) (string, [2]buttonRect) {
	buttonLabels := []string{
		m.renderButtonLabel("Copy", m.focus == copyFocusConfirm),
		m.renderButtonLabel("Cancel", m.focus == copyFocusCancel),
	}
	gap := "   "
	row := lipgloss.JoinHorizontal(lipgloss.Center, buttonLabels[0], gap, buttonLabels[1])
	rowWidth := lipgloss.Width(row)
	leftPad := max(0, (width-rowWidth)/2)
	line := strings.Repeat(" ", leftPad) + row
	if pad := width - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	rects := [2]buttonRect{
		{x: leftPad, y: lineIdx, w: lipgloss.Width(buttonLabels[0]), h: 1},
		{x: leftPad + lipgloss.Width(buttonLabels[0]) + lipgloss.Width(gap), y: lineIdx, w: lipgloss.Width(buttonLabels[1]), h: 1},
	}
	return line, rects
}

func (m *CopyToLocalModel) renderButtonLabel(label string, focused bool) string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(uistyles.ColorModalFg)).
		Background(lipgloss.Color(uistyles.ColorDarkGrey)).
		Padding(0, 2)
	if focused {
		style = style.
			Foreground(lipgloss.Color(uistyles.ColorModalFg)).
			Background(lipgloss.Color(uistyles.ColorModalSelBg)).
			Bold(true)
	}
	return style.Render(label)
}

func expandedPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if value[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			if value == "~" {
				return home
			}
			if len(value) > 1 && (value[1] == '/' || value[1] == '\\') {
				return filepath.Join(home, value[2:])
			}
		}
	}
	return value
}
