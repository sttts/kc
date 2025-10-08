package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sttts/kc/internal/overlay"
)

// Modal represents a modal dialog
type Modal struct {
	title            string
	content          tea.Model
	width            int
	height           int
	visible          bool
	onClose          func() tea.Cmd
	escPressed       bool
	closeOnSingleEsc bool
	// Windowed overlay support
	windowed       bool
	winWidth       int
	winHeight      int
	background     string        // full-screen base to overlay on when windowed
	backgroundFunc func() string // dynamic background provider
	contentOffsetX int
	contentOffsetY int
}

// RedrawTickMsg is emitted periodically to force a re-render while a
// windowed modal with a dynamic background is visible (e.g., for live
// preview beneath the dialog).
type RedrawTickMsg struct{}

// ModalFooterHints allows content to contribute footer key hints
// rendered next to the default "Esc Close".
type ModalFooterHints interface {
	// FooterHints returns a list of key,label pairs to render in the footer.
	FooterHints() [][2]string
}

// Init initializes the modal
func (m *Modal) Init() tea.Cmd {
	return m.content.Init()
}

// NewModal creates a new modal dialog
func NewModal(title string, content tea.Model) *Modal {
	return &Modal{
		title:            title,
		content:          content,
		visible:          false,
		closeOnSingleEsc: true,
	}
}

// SetContent replaces the content model inside the modal.
func (m *Modal) SetContent(content tea.Model) { m.content = content }

// Show shows the modal
func (m *Modal) Show() {
	m.visible = true
}

// Hide hides the modal
func (m *Modal) Hide() {
	m.visible = false
}

// IsVisible returns true if the modal is visible
func (m *Modal) IsVisible() bool {
	return m.visible
}

// SetDimensions sets the modal dimensions
func (m *Modal) SetDimensions(width, height int) {
	m.width = width
	m.height = height
}

// SetOnClose sets the callback for when the modal is closed
func (m *Modal) SetOnClose(callback func() tea.Cmd) {
	m.onClose = callback
}

// SetCloseOnSingleEsc controls whether a lone Esc closes the modal. Esc+digit
// handling remains active regardless.
func (m *Modal) SetCloseOnSingleEsc(v bool) { m.closeOnSingleEsc = v }

// SetWindowed configures the modal to render as a centered window of the given
// size over the provided background (full-screen base). If bg is empty, the
// window is centered over a blank backdrop.
func (m *Modal) SetWindowed(winW, winH int, bg string) {
	m.windowed = true
	m.winWidth, m.winHeight = winW, winH
	m.background = bg
	m.contentOffsetX = 0
	m.contentOffsetY = 0
}

// SetWindowedBackgroundProvider sets a function to produce the background
// view dynamically each render (e.g., for live preview under dialogs).
func (m *Modal) SetWindowedBackgroundProvider(f func() string) { m.backgroundFunc = f }

// Update handles messages and updates the modal state
func (m *Modal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	var cmd tea.Cmd
	var cmds []tea.Cmd

	// Handle modal-specific keys
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Double-ESC always closes, regardless of closeOnSingleEsc.
			if m.escPressed {
				m.escPressed = false
				m.Hide()
				if m.onClose != nil {
					cmd = m.onClose()
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}
			// Single ESC: close immediately only when enabled
			if m.closeOnSingleEsc {
				m.Hide()
				if m.onClose != nil {
					cmd = m.onClose()
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}
			// Otherwise arm ESC sequence to allow ESC ESC close
			m.escPressed = true
			return m, tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg { return EscTimeoutMsg{} })
		case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0":
			if m.escPressed {
				switch msg.String() {
				case "2":
					// ESC+2 → Theme (if supported by content)
					if themable, ok := m.content.(interface{ RequestTheme() tea.Cmd }); ok {
						m.escPressed = false
						return m, themable.RequestTheme()
					}
				case "0":
					// ESC+0 → Close (F10)
					m.escPressed = false
					m.Hide()
					if m.onClose != nil {
						cmd = m.onClose()
						cmds = append(cmds, cmd)
					}
					return m, tea.Batch(cmds...)
				}
				// Unhandled number: cancel esc sequence
				m.escPressed = false
				return m, nil
			}
		}
	case EscTimeoutMsg:
		if m.escPressed {
			// Timeout expired without second ESC; cancel sequence.
			m.escPressed = false
			// For closeOnSingleEsc=false, do nothing (wait for explicit action)
			// For true, single ESC already closed immediately above.
			return m, nil
		}
	}

	// Update the content
	updateMsg := msg
	if mm, ok := msg.(tea.MouseMsg); ok {
		updateMsg = shiftMouseMsg(mm, m.contentOffsetX, m.contentOffsetY)
	}
	model, cmd := m.content.Update(updateMsg)
	m.content = model
	cmds = append(cmds, cmd)

	// If we have a dynamic background provider, schedule a redraw tick so
	// the composed background stays fresh even when only the selection moves.
	if m.windowed && m.backgroundFunc != nil {
		cmds = append(cmds, tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return RedrawTickMsg{} }))
	}

	return m, tea.Batch(cmds...)
}

// View renders the modal
func (m *Modal) View() string {
	if !m.visible {
		return ""
	}
	// Fullscreen modal styled like panel
	// Reserve 1 line for overlay header and 1 terminal line for the function key bar outside the frame.
	// The framed box below has total height (m.height-2); its interior height is (m.height-2) - bottom border (1) = m.height-3.
	contentW := max(1, m.width-2)
	contentH := max(1, m.height-3)
	// For YAML viewers, drop left/right borders for easier copy/paste and allow full width content.
	// Full-width content for viewers: detect via RequestTheme capability (viewers implement it)
	_, isViewer := m.content.(interface{ RequestTheme() tea.Cmd })
	if isViewer {
		contentW = m.width
		// Viewers do not draw a bottom border; give them one extra row of
		// interior height so content reaches the footer line.
		contentH = max(1, m.height-2)
	}
	m.contentOffsetX = 0
	m.contentOffsetY = 0

	if m.windowed {
		// Render background (use provided base or blank)
		base := m.background
		if m.backgroundFunc != nil {
			if s := m.backgroundFunc(); s != "" {
				base = s
			}
		}
		if base == "" {
			base = lipgloss.NewStyle().Width(m.width).Height(m.height).Render("")
		}

		// Compute inner dimensions and render content
		winW := min(m.winWidth, m.width)
		winH := min(m.winHeight, m.height-1) // leave room for footer outside
		innerW := max(1, winW-2)
		innerH := max(1, winH-2)
		windowOffsetX := (m.width - winW) / 2
		if windowOffsetX < 0 {
			windowOffsetX = 0
		}
		maxOffsetX := m.width - winW
		if maxOffsetX < 0 {
			maxOffsetX = 0
		}
		if windowOffsetX > maxOffsetX {
			windowOffsetX = maxOffsetX
		}
		windowOffsetY := (m.height - winH) / 2
		windowOffsetY-- // lift window to leave footer line for function keys
		if windowOffsetY < 0 {
			windowOffsetY = 0
		}
		maxOffsetY := m.height - winH
		if maxOffsetY < 0 {
			maxOffsetY = 0
		}
		if windowOffsetY > maxOffsetY {
			windowOffsetY = maxOffsetY
		}
		m.contentOffsetX = windowOffsetX + 1
		m.contentOffsetY = windowOffsetY + 1
		if setter, ok := m.content.(interface{ SetDimensions(int, int) }); ok {
			setter.SetDimensions(innerW, innerH)
		}
		inner := ""
		if m.content != nil {
			if viewable, ok := m.content.(interface{ View() string }); ok {
				inner = viewable.View()
			}
		}

		// Build window frame with requested dialog styling for settings dialogs:
		// - Background: light grey
		// - Foreground: black
		// - Border: double, black
		boxStyle := lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Black).
			BorderBackground(lipgloss.Color("250")).
			Background(lipgloss.Color("250")).
			Width(winW).
			Height(winH)

		labelStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Black).
			Background(lipgloss.Color("250")).
			Padding(0, 1)
		label := labelStyle.Render(m.title)
		border := boxStyle.GetBorderStyle()
		topBorderStyler := lipgloss.NewStyle().
			Foreground(boxStyle.GetBorderTopForeground()).
			Background(boxStyle.GetBorderTopBackground()).
			Render
		// In windowed settings dialogs we keep corners; viewers don't use windowed mode.
		topLeft := topBorderStyler(border.TopLeft)
		topRight := topBorderStyler(border.TopRight)
		available := winW - lipgloss.Width(topLeft+topRight)
		lw := lipgloss.Width(label)
		var top string
		if lw >= available {
			gap := strings.Repeat(border.Top, max(0, available-lw))
			top = topLeft + label + topBorderStyler(gap) + topRight
		} else {
			total := available - lw
			left := total / 2
			right := total - left
			top = topLeft + topBorderStyler(strings.Repeat(border.Top, left)) + label + topBorderStyler(strings.Repeat(border.Top, right)) + topRight
		}
		// Render inner content with dialog colors (white on dark cyan)
		inner = lipgloss.NewStyle().
			Background(lipgloss.Color("250")).
			Foreground(lipgloss.Black).
			Width(innerW).
			Height(innerH).
			Render(inner)

		winBottom := boxStyle.Copy().
			BorderTop(false).
			Width(winW).
			Height(winH - 1).
			Render(inner)
		winFrame := top + "\n" + winBottom

		// Compose window over background (centered)
		composed := overlay.Composite(
			winFrame,
			base,
			overlay.Center, overlay.Center,
			0, -1, // lift by 1 to keep footer free
		)
		bgLines := strings.Split(composed, "\n")
		// Footer line
		footer := ""
		if provider, ok := m.content.(ModalFooterHints); ok {
			for i, kv := range provider.FooterHints() {
				if i > 0 {
					footer += " "
				}
				footer += FunctionKeyStyle.Render(kv[0]) + FunctionKeyDescriptionStyle.Render(kv[1])
			}
		}
		if m.height > 0 {
			if len(bgLines) < m.height {
				for len(bgLines) < m.height {
					bgLines = append(bgLines, "")
				}
			}
			bgLines[m.height-1] = FunctionKeyBarStyle.Width(m.width).Render(footer)
		}
		composed = strings.Join(bgLines, "\n")
		// Return composed screen without forcing a global background color so
		// the 2-line terminal retains its original styling.
		return lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Render(composed)
	}
	var inner string
	if setter, ok := m.content.(interface{ SetDimensions(int, int) }); ok {
		setter.SetDimensions(contentW, contentH)
	}
	if m.content != nil {
		if viewable, ok := m.content.(interface{ View() string }); ok {
			inner = viewable.View()
		}
	}
	// Build frame with overlay title (match focused panel style)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.White).
		BorderBackground(lipgloss.Blue).
		Background(lipgloss.Blue)

		// Focused panel title chip style
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Black).
		Background(lipgloss.White).
		Padding(0, 1)
	label := labelStyle.Render(m.title)
	border := boxStyle.GetBorderStyle()
	topBorderStyler := lipgloss.NewStyle().
		Foreground(boxStyle.GetBorderTopForeground()).
		Background(boxStyle.GetBorderTopBackground()).
		Render

	// For viewers, do not show corner glyphs; use '-' for the entire top line.
	// For viewers, use the horizontal border rune for the entire top line,
	// including the ends, instead of corner glyphs.
	topRune := border.Top
	tl := border.TopLeft
	tr := border.TopRight
	if isViewer {
		tl, tr = topRune, topRune
	}
	topLeft := topBorderStyler(tl)
	topRight := topBorderStyler(tr)
	available := m.width - lipgloss.Width(topLeft+topRight)
	lw := lipgloss.Width(label)
	var top string
	if lw >= available {
		gap := strings.Repeat(string(topRune), max(0, available-lw))
		top = topLeft + label + topBorderStyler(gap) + topRight
	} else {
		total := available - lw
		left := total / 2
		right := total - left
		top = topLeft + topBorderStyler(strings.Repeat(string(topRune), left)) + label + topBorderStyler(strings.Repeat(string(topRune), right)) + topRight
	}

	// Box content under header (no footer inside the frame)
	bottomSty := boxStyle.Copy().
		BorderTop(false).
		Width(m.width).
		// Reserve one terminal line for the footer outside the frame
		Height(m.height - 2)
	if isViewer {
		// Remove vertical borders and bottom border for viewers.
		bottomSty = bottomSty.BorderLeft(false).BorderRight(false).BorderBottom(false)
	}
	bottom := bottomSty.Render(inner)
	if isViewer {
		m.contentOffsetX = 0
	} else {
		m.contentOffsetX = 1
	}
	m.contentOffsetY = 1

	// Replace bottom corners to T junction at the top border of bottom
	lines := strings.Split(bottom, "\n")
	frame := top + "\n" + strings.Join(lines, "\n")
	// Function key bar outside the frame
	footer := ""
	if m.closeOnSingleEsc {
		footer = FunctionKeyStyle.Render("Esc") + FunctionKeyDescriptionStyle.Render("Close")
	}
	if provider, ok := m.content.(ModalFooterHints); ok {
		for _, kv := range provider.FooterHints() {
			key, label := kv[0], kv[1]
			if footer != "" {
				footer += " "
			}
			footer += FunctionKeyStyle.Render(key) + FunctionKeyDescriptionStyle.Render(label)
		}
	}
	footerLine := FunctionKeyBarStyle.Width(m.width).Render(footer)
	return lipgloss.JoinVertical(lipgloss.Left, frame, footerLine)
}

func shiftMouseMsg(msg tea.MouseMsg, dx, dy int) tea.MouseMsg {
	if dx == 0 && dy == 0 {
		return msg
	}
	mouse := msg.Mouse()
	mouse.X -= dx
	mouse.Y -= dy
	switch msg.(type) {
	case tea.MouseClickMsg:
		return tea.MouseClickMsg(mouse)
	case tea.MouseReleaseMsg:
		return tea.MouseReleaseMsg(mouse)
	case tea.MouseWheelMsg:
		return tea.MouseWheelMsg(mouse)
	case tea.MouseMotionMsg:
		return tea.MouseMotionMsg(mouse)
	default:
		return msg
	}
}

// ModalManager manages multiple modals
type ModalManager struct {
	modals map[string]*Modal
	stack  []string // modal name stack; top-most is last
}

// Init initializes the modal manager
func (mm *ModalManager) Init() tea.Cmd {
	return nil
}

// NewModalManager creates a new modal manager
func NewModalManager() *ModalManager {
	return &ModalManager{
		modals: make(map[string]*Modal),
		stack:  []string{},
	}
}

// Register registers a modal
func (mm *ModalManager) Register(name string, modal *Modal) {
	mm.modals[name] = modal
}

// Show shows a modal by name
func (mm *ModalManager) Show(name string) {
	if modal, exists := mm.modals[name]; exists {
		// If already on top, just ensure visible
		if len(mm.stack) > 0 && mm.stack[len(mm.stack)-1] == name {
			modal.Show()
			return
		}
		modal.Show()
		mm.stack = append(mm.stack, name)
	}
}

// Hide hides the active modal
func (mm *ModalManager) Hide() {
	if len(mm.stack) > 0 {
		top := mm.stack[len(mm.stack)-1]
		if modal, exists := mm.modals[top]; exists {
			modal.Hide()
		}
		mm.stack = mm.stack[:len(mm.stack)-1]
	}
}

// IsModalVisible returns true if any modal is visible
func (mm *ModalManager) IsModalVisible() bool {
	return len(mm.stack) > 0
}

// GetActiveModal returns the active modal
func (mm *ModalManager) GetActiveModal() *Modal {
	if len(mm.stack) > 0 {
		name := mm.stack[len(mm.stack)-1]
		return mm.modals[name]
	}
	return nil
}

// Update handles messages and updates the modal manager
func (mm *ModalManager) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(mm.stack) > 0 {
		name := mm.stack[len(mm.stack)-1]
		if modal, exists := mm.modals[name]; exists {
			model, cmd := modal.Update(msg)
			mm.modals[name] = model.(*Modal)
			// If the modal was closed externally, pop it safely
			if !modal.IsVisible() && len(mm.stack) > 0 && mm.stack[len(mm.stack)-1] == name {
				mm.stack = mm.stack[:len(mm.stack)-1]
			}
			return mm, cmd
		}
	}
	return mm, nil
}

// View renders the modal manager
func (mm *ModalManager) View() string {
	if len(mm.stack) > 0 {
		name := mm.stack[len(mm.stack)-1]
		if modal, exists := mm.modals[name]; exists {
			return modal.View()
		}
	}
	return ""
}

// sliceANSIByColumns returns a substring by visible columns, ignoring ANSI
// escape sequences for counting. It preserves escape sequences in the slice
// and terminates with a reset to avoid leaking attributes.
// sliceANSIColsRaw returns a raw ANSI slice by visible columns without
// appending a reset; useful for composing segments.
func sliceANSIColsRaw(s string, start, width int) string {
	if start < 0 {
		start = 0
	}
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	col := 0
	collecting := false
	for i := 0; i < len(s); {
		if i < len(s) && s[i] == 0x1b { // ESC
			// CSI ... 'm'
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && s[j] != 'm' {
					j++
				}
				if j < len(s) {
					j++
				}
			}
			if collecting {
				b.WriteString(s[i:j])
			}
			i = j
			continue
		}
		if col >= start && col < start+width {
			b.WriteByte(s[i])
			collecting = true
		}
		col++
		if col >= start+width {
			break
		}
		i++
	}
	return b.String()
}
