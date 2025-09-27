package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	bubbleterm "github.com/taigrr/bubbleterm"
)

// ShellExitedMsg is sent when the shell process exits
type ShellExitedMsg struct {
	ExitCode string
}

// Terminal represents the bottom terminal component
type Terminal struct {
	// Bubbleterm terminal emulator
	terminal *bubbleterm.Model
	// Terminal state
	width     int
	height    int
	isRunning bool
	// Display state
	showPanels bool // Whether panels are visible
	// Input tracking
	hasTyped bool // Whether user has typed since last command
	// Exit handling
	shellExited bool
}

// NewTerminal creates a new terminal instance
func NewTerminal() *Terminal {
	return &Terminal{
		width:       80,
		height:      24,
		isRunning:   false,
		showPanels:  true, // Start with panels visible
		hasTyped:    false,
		shellExited: false,
	}
}

// Init initializes the terminal
func (t *Terminal) Init() tea.Cmd {
	// Use the user's shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash" // fallback
	}

	// Create shell command
	cmd := exec.Command(shell)
	cmd.Env = os.Environ()

	// Create bubbleterm terminal
	terminal, err := bubbleterm.New(t.width, t.height)
	if err != nil {
		// If we can't create the terminal (e.g., TTY not available), create a fallback
		// that displays the error but doesn't crash the application
		t.terminal = nil
		t.isRunning = false
		return func() tea.Msg {
			return ShellExitedMsg{ExitCode: fmt.Sprintf("Terminal unavailable: %v", err)}
		}
	}

	t.terminal = terminal

	// Set up exit callback to quit when shell exits
	emulator := t.terminal.GetEmulator()
	if emulator != nil {
		emulator.SetOnExit(func(exitCode string) {
			// When shell exits, set the flag
			t.shellExited = true
		})

		// Note: Cursor should be enabled by default in bubbleterm
		// If cursor is not visible, it might be a focus or rendering issue
	}

	// Start the shell command
	// Note: bubbleterm handles cursor automatically through PTY, no need for ShowCursor()
	return tea.Batch(t.terminal.Init(), t.terminal.StartCommand(cmd))
}

// Update handles messages and updates the terminal state
func (t *Terminal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle window resize events
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		msg.Width = max(1, msg.Width)
		msg.Height = max(1, msg.Height)

		t.width = msg.Width
		t.height = msg.Height
		if t.terminal != nil {
			model, cmd := t.terminal.Update(msg)
			t.terminal = model.(*bubbleterm.Model)
			return t, cmd
		}
		return t, nil
	}

	// Always update bubbleterm first to check process status
	if t.terminal != nil {
		model, cmd := t.terminal.Update(msg)
		t.terminal = model.(*bubbleterm.Model)

		// Track if user has typed (for key routing logic)
		if t.showPanels {
			switch msg := msg.(type) {
			case tea.KeyMsg:
				// Mark that user has typed (for Enter key routing)
				if msg.String() != "enter" {
					t.hasTyped = true
				} else {
					// Reset on Enter (command executed)
					t.hasTyped = false
				}
			}
		}

		return t, cmd
	}

	return t, nil
}

// View renders the terminal
func (t *Terminal) View() (string, *tea.Cursor) {
	if t.terminal == nil {
		cur := tea.NewCursor(0, 0)
		cur.Blink = true
		return "Terminal not initialized", cur
	}

	if t.showPanels {
		view, cur := t.renderTwoLineViewWithCursor()
		return view, cur
	}

	// bubbleterm now implements CursorModel and returns cursor directly
	terminalView, cursor := t.terminal.View()
	return terminalView, cursor
}

// renderTwoLineViewWithCursor renders the 2-line view and returns cursor
func (t *Terminal) renderTwoLineViewWithCursor() (string, *tea.Cursor) {
	if t.terminal == nil {
		fallback := lipgloss.NewStyle().
			Width(t.width).
			Height(2).
			Render("Terminal not initialized")
		return fallback, nil
	}

	terminalView, termCur := t.terminal.View()
	return renderTwoLineFrom(terminalView, termCur, t.width)
}

// renderTwoLineView renders the 2-line view when panels are visible
func (t *Terminal) renderTwoLineView() string {
	if t.terminal == nil {
		return lipgloss.NewStyle().
			Width(t.width).
			Height(2).
			Render("Terminal not initialized")
	}

	terminalView, _ := t.terminal.View()
	terminalLines := strings.Split(terminalView, "\n")

	// Take the last 2 lines to show the cursor and previous line
	var lines []string
	if len(terminalLines) >= 2 {
		lines = terminalLines[len(terminalLines)-2:]
	} else {
		lines = terminalLines
	}

	// Ensure we have exactly 2 lines
	for len(lines) < 2 {
		lines = append(lines, "")
	}

	// Join lines
	content := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Width(t.width).
		Height(2).
		Render(content)
}

// renderTwoLineFrom produces a two-line view from a full terminal view and optional cursor.
// It clamps the cursor position to existing lines and always returns exactly 2 lines rendered
// to the specified width. When the cursor is present, it is repositioned to Y=1.
func renderTwoLineFrom(terminalView string, termCur *tea.Cursor, width int) (string, *tea.Cursor) {
	terminalLines := strings.Split(terminalView, "\n")

	if termCur == nil {
		// No cursor available, show last 2 lines of terminal output
		var lines []string
		if len(terminalLines) >= 2 {
			lines = terminalLines[len(terminalLines)-2:]
		} else {
			lines = terminalLines
		}
		// Ensure we have exactly 2 lines
		for len(lines) < 2 {
			lines = append(lines, "")
		}
		content := strings.Join(lines, "\n")
		view := lipgloss.NewStyle().
			Width(width).
			Height(2).
			Render(content)
		return view, nil
	}

	// Show the cursor line and the line before it
	var lines []string
	if len(terminalLines) == 0 {
		lines = []string{"", ""}
		view := lipgloss.NewStyle().
			Width(width).
			Height(2).
			Render(strings.Join(lines, "\n"))
		return view, nil
	}
	y := termCur.Y
	if y < 0 {
		y = 0
	}
	if y >= len(terminalLines) {
		y = len(terminalLines) - 1
	}
	cur := *termCur
	cur.Y = 1
	cur.Blink = true
	if y > 0 {
		lines = []string{terminalLines[y-1], terminalLines[y]}
	} else {
		lines = []string{"", terminalLines[y]}
	}
	view := lipgloss.NewStyle().
		Width(width).
		Height(2).
		Render(strings.Join(lines, "\n"))
	return view, &cur
}

// SetShowPanels sets whether panels are visible
func (t *Terminal) SetShowPanels(show bool) {
	t.showPanels = show
	// Ensure proper focus when switching modes
	if !show {
		// When going to fullscreen mode, focus the terminal
		t.Focus()
	}
	// Terminal dimensions are managed by the app through WindowSizeMsg forwarding
}

// Focus sets focus on the terminal
func (t *Terminal) Focus() {
	if t.terminal != nil {
		t.terminal.Focus()
		// Also ensure the terminal is focused by calling it again
		// This helps with some focus issues
		t.terminal.Focus()
	}
}

// Blur removes focus from the terminal
func (t *Terminal) Blur() {
	if t.terminal != nil {
		t.terminal.Blur()
	}
}

// Focused returns whether the terminal is focused
func (t *Terminal) Focused() bool {
	if t.terminal != nil {
		return t.terminal.Focused()
	}
	return false
}

// SendInput sends input to the terminal
func (t *Terminal) SendInput(input string) {
	if t.terminal != nil {
		t.terminal.SendInput(input)
	}
}

// IsProcessExited returns whether the terminal process has exited
func (t *Terminal) IsProcessExited() bool {
	return t.shellExited
}

// HasInput returns whether the terminal has non-empty input
func (t *Terminal) HasInput() bool {
	return t.hasTyped
}

// ClearTyped resets the typed flag (used to return focus to panels).
func (t *Terminal) ClearTyped() { t.hasTyped = false }
