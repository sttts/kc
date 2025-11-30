package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/go-logr/logr"
	bubbleterm "github.com/taigrr/bubbleterm"
)

// ShellExitedMsg is sent when the shell process exits
type ShellExitedMsg struct {
	ExitCode string
}

type terminalPollMsg struct {
	token uint64
}

const terminalPollInterval = 100 * time.Millisecond

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
	// Whether the PTY app has enabled mouse tracking (ESC[?1000h/1002h/1003h/1006h/1015h)
	ptyWantsMouse bool
	log           logr.Logger
	env           []string
	lastPoll      time.Time
	pollSeq       uint64
	pollToken     uint64
	pollScheduled bool
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
	if t.log.GetSink() != nil {
		t.log.Info("starting terminal shell", "command", shell)
	}

	// Create shell command
	cmd := exec.Command(shell)
	env := t.env
	if len(env) == 0 {
		env = os.Environ()
	}
	cmd.Env = env

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
	t.terminal.SetQuietExit(true)
	t.terminal.WithCtrlCSignal(true)
	t.terminal.WithCtrlZSignal(true)

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
	t.terminal.SetAutoPoll(false)
	return tea.Batch(t.terminal.Init(), t.terminal.StartCommand(cmd), t.immediatePollCmd())
}

// SetEnv overrides the environment used when spawning the PTY shell.
func (t *Terminal) SetEnv(env []string) { t.env = append([]string(nil), env...) }

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
			return t, tea.Batch(cmd, t.immediatePollCmd())
		}
		return t, nil
	case terminalPollMsg:
		if msg.token != t.pollToken {
			return t, nil
		}
		t.pollScheduled = false
		return t, t.immediatePollCmd()
	}

	// Always update bubbleterm first to check process status
	if t.terminal != nil {
		// Swallow mouse events while panels are visible to avoid sending
		// mouse escape sequences into the PTY (which shows up as garbage
		// in the 2-line terminal view). In fullscreen terminal mode, let
		// mouse events pass through to bubbleterm only if the app requested it.
		if mm, ok := msg.(tea.MouseMsg); ok {
			if t.showPanels {
				// Panels visible → ignore mouse for PTY; panels handle mouse themselves
				_ = mm
				return t, nil
			}
			// Fullscreen terminal: forward Bubble Tea mouse events to bubbleterm;
			// bubbleterm and the underlying app decide whether to act on them.
		}
		model, cmd := t.terminal.Update(translateKeyForTerminal(msg))
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

		return t, tea.Batch(cmd, t.immediatePollCmd())
	}

	return t, nil
}

// SetPTYWantsMouse allows wiring a detector that toggles whether the PTY app
// requested mouse events (e.g., by parsing ESC[?1000h/l sequences from the
// app's stdout). TODO: hook into bubbleterm emulator output to call this.
func (t *Terminal) SetPTYWantsMouse(on bool) { t.ptyWantsMouse = on }

// View renders the terminal
func (t *Terminal) View() tea.View {
	if t.terminal == nil {
		cur := tea.NewCursor(0, 0)
		cur.Blink = true
		return viewWithCursor("Terminal not initialized", cur)
	}

	if t.showPanels {
		view, cur := t.renderTwoLineViewWithCursor()
		return viewWithCursor(view, cur)
	}

	terminalView, cursor := t.terminalView()
	return viewWithCursor(terminalView, cursor)
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

	terminalView, termCur := t.terminalView()
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

	terminalView, _ := t.terminalView()
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

func (t *Terminal) terminalView() (string, *tea.Cursor) {
	if t.terminal == nil {
		return "", nil
	}
	switch term := any(t.terminal).(type) {
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

func (t *Terminal) immediatePollCmd() tea.Cmd {
	if t == nil || t.terminal == nil {
		return nil
	}
	now := time.Now()
	if !t.lastPoll.IsZero() {
		elapsed := now.Sub(t.lastPoll)
		if elapsed < terminalPollInterval {
			if t.pollScheduled {
				return nil
			}
			delay := terminalPollInterval - elapsed
			return t.schedulePoll(delay)
		}
	}
	t.lastPoll = now
	t.pollScheduled = false
	poll := t.terminal.UpdateTerminal()
	timer := t.schedulePoll(terminalPollInterval)
	if poll == nil {
		return timer
	}
	if timer == nil {
		return poll
	}
	return tea.Batch(poll, timer)
}

func (t *Terminal) schedulePoll(delay time.Duration) tea.Cmd {
	if t == nil || t.terminal == nil {
		return nil
	}
	if delay <= 0 {
		delay = terminalPollInterval
	}
	t.pollSeq++
	token := t.pollSeq
	t.pollToken = token
	t.pollScheduled = true
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return terminalPollMsg{token: token}
	})
}

type legacyKeyPressMsg struct {
	key  tea.Key
	text string
}

func (m legacyKeyPressMsg) String() string { return m.text }

func (m legacyKeyPressMsg) Key() tea.Key { return m.key }

func (m legacyKeyPressMsg) Keystroke() string {
	if len(m.text) == 1 {
		return m.text
	}
	return m.key.Keystroke()
}

type legacyKeyReleaseMsg struct {
	key  tea.Key
	text string
}

func (m legacyKeyReleaseMsg) String() string { return m.text }

func (m legacyKeyReleaseMsg) Key() tea.Key { return m.key }

func (m legacyKeyReleaseMsg) Keystroke() string {
	if len(m.text) == 1 {
		return m.text
	}
	return m.key.Keystroke()
}

func translateKeyForTerminal(msg tea.Msg) tea.Msg {
	switch msg.(type) {
	case tea.KeyMsg, tea.KeyPressMsg, tea.KeyReleaseMsg:
		return msg
	default:
		return msg
	}
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

// SetLogger installs a logger for terminal lifecycle events.
func (t *Terminal) SetLogger(log logr.Logger) { t.log = log }

// Close shuts down the underlying emulator/PTY.
func (t *Terminal) Close() error {
	if t == nil || t.terminal == nil {
		return nil
	}
	return t.terminal.Close()
}
