package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
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

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
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

func (m *model) View() (string, *tea.Cursor) {
	view, cursor := m.term.View()
	lines := strings.Split(view, "\n")
	status := statusLine
	if m.width > 0 {
		status = padToWidth(statusLine, m.width)
	}
	style := lipgloss.NewStyle().Width(m.width)
	lines = append(lines, style.Render(status))
	return strings.Join(lines, "\n"), cursor
}

func padToWidth(text string, width int) string {
	w := lipgloss.Width(text)
	if w >= width {
		return text
	}
	return text + strings.Repeat(" ", width-w)
}
