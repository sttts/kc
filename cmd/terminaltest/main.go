package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	bubbleterm "github.com/taigrr/bubbleterm"
)

const (
	defaultWidth  = 80
	defaultHeight = 24
	statusLine    = "test"
)

func main() {
	m, err := newModel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init model: %v\n", err)
		os.Exit(1)
	}

	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run program: %v\n", err)
		os.Exit(1)
	}
}

type model struct {
	term   *bubbleterm.Model
	width  int
	height int
}

func newModel() (*model, error) {
	term, err := bubbleterm.New(defaultWidth, defaultHeight-1)
	if err != nil {
		return nil, err
	}
	term.WithCtrlCSignal(true)
	term.WithCtrlZSignal(true)
	return &model{
		term:   term,
		width:  defaultWidth,
		height: defaultHeight,
	}, nil
}

func (m *model) Init() tea.Cmd {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell)
	cmd.Env = os.Environ()
	return tea.Batch(
		m.term.Init(),
		m.term.StartCommand(cmd),
	)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		usableHeight := msg.Height - 1
		if usableHeight < 1 {
			usableHeight = 1
		}
		m.width = msg.Width
		m.height = msg.Height
		msg.Height = usableHeight
		model, cmd := m.term.Update(msg)
		m.term = model.(*bubbleterm.Model)
		if m.term.GetEmulator() != nil && m.term.GetEmulator().IsProcessExited() {
			return m, tea.Quit
		}
		return m, cmd
	default:
		model, cmd := m.term.Update(msg)
		m.term = model.(*bubbleterm.Model)
		if m.term.GetEmulator() != nil && m.term.GetEmulator().IsProcessExited() {
			return m, tea.Quit
		}
		return m, cmd
	}
}

func (m *model) View() tea.View {
	view, cursor := m.terminalView()
	lines := strings.Split(view, "\n")
	status := statusLine
	if m.width > 0 {
		status = padToWidth(statusLine, m.width)
	}
	style := lipgloss.NewStyle().Width(m.width)
	lines = append(lines, style.Render(status))
	content := strings.Join(lines, "\n")
	v := tea.NewView(content)
	v.Cursor = cursor
	v.AltScreen = true
	return v
}

func padToWidth(text string, width int) string {
	w := lipgloss.Width(text)
	if w >= width {
		return text
	}
	return text + strings.Repeat(" ", width-w)
}

func (m *model) terminalView() (string, *tea.Cursor) {
	switch term := any(m.term).(type) {
	case interface{ View() tea.View }:
		view := term.View()
		return viewString(view), view.Cursor
	case interface{ View() (string, *tea.Cursor) }:
		return term.View()
	case interface{ View() string }:
		return term.View(), nil
	default:
		return "", nil
	}
}

func viewString(view tea.View) string {
	if view.Content == nil {
		return ""
	}
	return fmt.Sprint(view.Content)
}
