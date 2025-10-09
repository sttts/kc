package ui

import (
	"strconv"
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
	windowOffsetX  int
	windowOffsetY  int
	windowHasPos   bool
	contentOffsetX int
	contentOffsetY int
	footerHotspots []footerHotspot
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
	m.windowHasPos = false
}

// SetWindowedBackgroundProvider sets a function to produce the background
// view dynamically each render (e.g., for live preview under dialogs).
func (m *Modal) SetWindowedBackgroundProvider(f func() string) { m.backgroundFunc = f }

// SetWindowOffset positions the window relative to the main view's origin.
// Only applies when windowed=true.
func (m *Modal) SetWindowOffset(offsetX, offsetY int) {
	m.windowOffsetX = offsetX
	m.windowOffsetY = offsetY
	m.windowHasPos = true
}

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
	case tea.MouseMsg:
		if model, cmd, handled := m.handleFooterMouse(msg); handled {
			return model, cmd
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
	m.footerHotspots = m.footerHotspots[:0]
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
		bgWidth, bgHeight := lipgloss.Size(base)
		if bgWidth == 0 {
			bgWidth = m.width
		}
		if bgHeight == 0 {
			bgHeight = m.height
		}
		availableHeight := max(0, bgHeight-1) // leave room for footer outside
		winW := min(m.winWidth, bgWidth)
		if winW <= 0 {
			winW = bgWidth
		}
		winH := min(m.winHeight, max(1, availableHeight))
		if winH <= 0 {
			winH = 1
		}
		innerW := max(1, winW-2)
		innerH := max(1, winH-2)
		windowOffsetX := 0
		if bgWidth > winW {
			windowOffsetX = (bgWidth - winW) / 2
		}
		windowOffsetY := 0
		if availableHeight > winH {
			windowOffsetY = (availableHeight - winH) / 2
		}
		if m.windowHasPos {
			windowOffsetX = m.windowOffsetX
			windowOffsetY = m.windowOffsetY
		}
		windowOffsetX = clampInt(windowOffsetX, 0, max(0, bgWidth-winW))
		windowOffsetY = clampInt(windowOffsetY, 0, max(0, availableHeight-winH))
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

		frameBg := lipgloss.Color(ColorModalBg)
		frameFg := lipgloss.Color(ColorModalFg)
		boxStyle := lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(frameFg).
			BorderBackground(frameBg).
			Background(frameBg).
			Width(winW).
			Height(winH)

		labelStyle := lipgloss.NewStyle().
			Foreground(frameFg).
			Background(frameBg).
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
		// Render inner content using modal palette.
		inner = lipgloss.NewStyle().
			Background(frameBg).
			Foreground(frameFg).
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
			overlay.Left, overlay.Top,
			windowOffsetX, windowOffsetY,
		)
		bgLines := strings.Split(composed, "\n")
		// Footer line
		footer := m.buildFooter(false)
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
	modalBg := lipgloss.Color(ColorModalBg)
	modalFg := lipgloss.Color(ColorModalFg)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(modalFg).
		BorderBackground(modalBg).
		Background(modalBg)

		// Focused panel title chip style
	labelStyle := lipgloss.NewStyle().
		Foreground(modalFg).
		Background(lipgloss.Color(ColorModalSelBg)).
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
	footer := m.buildFooter(true)
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

type footerHotspot struct {
	key   string
	start int
	end   int
}

func (m *Modal) handleFooterMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return nil, nil, false
	}
	if mouse.Y != m.height-1 {
		return nil, nil, false
	}
	// Swallow clicks on footer to prevent fallthrough to content.
	if _, ok := msg.(tea.MouseClickMsg); ok {
		return m, nil, true
	}
	if _, ok := msg.(tea.MouseReleaseMsg); !ok {
		return m, nil, true
	}
	key := m.keyAtFooterColumn(mouse.X)
	if key == "" {
		return m, nil, true
	}
	if km, ok := keyMsgForLabel(key); ok {
		model, cmd := m.Update(km)
		return model.(*Modal), cmd, true
	}
	return m, nil, true
}

func (m *Modal) keyAtFooterColumn(x int) string {
	for _, hs := range m.footerHotspots {
		if x >= hs.start && x < hs.end {
			return hs.key
		}
	}
	return ""
}

func (m *Modal) buildFooter(includeEsc bool) string {
	var builder strings.Builder
	currentWidth := 0
	appendHint := func(key, label string) {
		if builder.Len() > 0 {
			builder.WriteString(" ")
			currentWidth += lipgloss.Width(" ")
		}
		segment := FunctionKeyStyle.Render(key) + FunctionKeyDescriptionStyle.Render(label)
		segWidth := lipgloss.Width(segment)
		m.footerHotspots = append(m.footerHotspots, footerHotspot{
			key:   key,
			start: currentWidth,
			end:   currentWidth + segWidth,
		})
		builder.WriteString(segment)
		currentWidth += segWidth
	}
	if includeEsc && m.closeOnSingleEsc {
		appendHint("Esc", "Close")
	}
	if provider, ok := m.content.(ModalFooterHints); ok {
		for _, kv := range provider.FooterHints() {
			appendHint(kv[0], kv[1])
		}
	}
	return builder.String()
}

func clampInt(v, minVal, maxVal int) int {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

func keyMsgForLabel(label string) (tea.KeyMsg, bool) {
	if label == "" {
		return nil, false
	}
	lower := strings.ToLower(label)
	switch lower {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}, true
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}, true
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}, true
	}
	if strings.HasPrefix(lower, "f") && len(label) > 1 {
		if n, err := strconv.Atoi(label[1:]); err == nil {
			if key, ok := functionKeyConstant(n); ok {
				return tea.KeyPressMsg{Code: key}, true
			}
		}
	}
	if strings.Contains(lower, "+") {
		parts := strings.Split(label, "+")
		if len(parts) < 2 {
			return nil, false
		}
		mod := tea.KeyMod(0)
		for i := 0; i < len(parts)-1; i++ {
			switch strings.ToLower(parts[i]) {
			case "ctrl", "control":
				mod |= tea.ModCtrl
			case "alt":
				mod |= tea.ModAlt
			case "shift":
				mod |= tea.ModShift
			case "meta":
				mod |= tea.ModMeta
			case "super":
				mod |= tea.ModSuper
			case "hyper":
				mod |= tea.ModHyper
			}
		}
		finalPart := parts[len(parts)-1]
		finalLower := strings.ToLower(finalPart)
		switch finalLower {
		case "esc":
			return tea.KeyPressMsg{Code: tea.KeyEsc, Mod: mod}, true
		case "enter":
			return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: mod}, true
		}
		runes := []rune(finalLower)
		if len(runes) == 1 {
			return tea.KeyPressMsg{Code: runes[0], Text: string(runes[0]), Mod: mod}, true
		}
		return nil, false
	}
	return nil, false
}

func functionKeyConstant(n int) (rune, bool) {
	switch n {
	case 1:
		return tea.KeyF1, true
	case 2:
		return tea.KeyF2, true
	case 3:
		return tea.KeyF3, true
	case 4:
		return tea.KeyF4, true
	case 5:
		return tea.KeyF5, true
	case 6:
		return tea.KeyF6, true
	case 7:
		return tea.KeyF7, true
	case 8:
		return tea.KeyF8, true
	case 9:
		return tea.KeyF9, true
	case 10:
		return tea.KeyF10, true
	case 11:
		return tea.KeyF11, true
	case 12:
		return tea.KeyF12, true
	case 13:
		return tea.KeyF13, true
	case 14:
		return tea.KeyF14, true
	case 15:
		return tea.KeyF15, true
	case 16:
		return tea.KeyF16, true
	case 17:
		return tea.KeyF17, true
	case 18:
		return tea.KeyF18, true
	case 19:
		return tea.KeyF19, true
	case 20:
		return tea.KeyF20, true
	case 21:
		return tea.KeyF21, true
	case 22:
		return tea.KeyF22, true
	case 23:
		return tea.KeyF23, true
	case 24:
		return tea.KeyF24, true
	default:
		return 0, false
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
