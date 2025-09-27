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
	windowed   bool
	winWidth   int
	winHeight  int
    background string // full-screen base to overlay on when windowed
    backgroundFunc func() string // dynamic background provider
}

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
			// Support ESC+number sequences and double-ESC close.
			if m.escPressed {
				// Double ESC: always close, regardless of closeOnSingleEsc
				m.escPressed = false
				m.Hide()
				if m.onClose != nil {
					cmd = m.onClose()
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}
			m.escPressed = true
			return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return EscTimeoutMsg{} })
		case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0":
			if m.escPressed {
				switch msg.String() {
				case "9":
					// ESC+9 → Theme (if supported by content)
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
			// Standalone ESC: close only if enabled
			m.escPressed = false
			if m.closeOnSingleEsc {
				m.Hide()
				if m.onClose != nil {
					cmd = m.onClose()
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}
			return m, nil
		}
	}

	// Update the content
	model, cmd := m.content.Update(msg)
	m.content = model
	cmds = append(cmds, cmd)

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

	if m.windowed {
		// Render background (use provided base or blank)
        base := m.background
        if m.backgroundFunc != nil {
            if s := m.backgroundFunc(); s != "" { base = s }
        }
		if base == "" {
			base = lipgloss.NewStyle().Width(m.width).Height(m.height).Render("")
		}

		// Compute inner dimensions and render content
		winW := min(m.winWidth, m.width)
		winH := min(m.winHeight, m.height-1) // leave room for footer outside
		innerW := max(1, winW-2)
		innerH := max(1, winH-2)
		if setter, ok := m.content.(interface{ SetDimensions(int, int) }); ok {
			setter.SetDimensions(innerW, innerH)
		}
		inner := ""
		if m.content != nil {
			if viewable, ok := m.content.(interface{ View() string }); ok {
				inner = viewable.View()
			}
		}

		// Build window frame with requested dialog styling:
		// - Background: dark cyan (ANSI 46)
		// - Foreground: white
		// - Border: double, white
		boxStyle := lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Black).
			BorderBackground(lipgloss.Cyan).
			Background(lipgloss.Cyan).
			Width(winW).
			Height(winH)

		labelStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorBlack)).
			Background(lipgloss.Color(ColorWhite)).
			Padding(0, 1)
		label := labelStyle.Render(m.title)
		border := boxStyle.GetBorderStyle()
		topBorderStyler := lipgloss.NewStyle().
			Foreground(boxStyle.GetBorderTopForeground()).
			Background(boxStyle.GetBorderTopBackground()).
			Render
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
			Background(lipgloss.Cyan).
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
		// Ensure full-width/height with blue background to avoid artifacts.
		return lipgloss.NewStyle().
			Background(lipgloss.Cyan).
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
		BorderForeground(lipgloss.Color(ColorGrey)).
		BorderBackground(lipgloss.Color(ColorDarkerBlue)).
		Background(lipgloss.Color(ColorDarkerBlue))

	// Focused panel title chip style
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorBlack)).
		Background(lipgloss.Color(ColorGrey)).
		Padding(0, 1)
	label := labelStyle.Render(m.title)
	border := boxStyle.GetBorderStyle()
	topBorderStyler := lipgloss.NewStyle().
		Foreground(boxStyle.GetBorderTopForeground()).
		Background(boxStyle.GetBorderTopBackground()).
		Render

	topLeft := topBorderStyler(border.TopLeft)
	topRight := topBorderStyler(border.TopRight)
	available := m.width - lipgloss.Width(topLeft+topRight)
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

	// Box content under header (no footer inside the frame)
	bottom := boxStyle.Copy().
		BorderTop(false).
		Width(m.width).
		// Reserve one terminal line for the footer outside the frame
		Height(m.height - 2).
		Render(inner)

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
