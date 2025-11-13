package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sttts/kc/internal/overlay"
	uistyles "github.com/sttts/kc/internal/ui/styles"
	bl "github.com/sttts/kc/third_party/bubblelayout"
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
	focus         namespaceFocus
}

type namespaceFocus int

const (
	namespaceFocusInput namespaceFocus = iota
	namespaceFocusCreate
	namespaceFocusCancel
)

const (
	namespaceMinWidth  = 24
	namespaceMaxWidth  = 60
	namespaceMinHeight = 5
)

// NewNamespaceCreateModel constructs an empty namespace creation dialog model.
func NewNamespaceCreateModel() *NamespaceCreateModel {
	return &NamespaceCreateModel{focus: namespaceFocusInput}
}

func (m *NamespaceCreateModel) Init() tea.Cmd          { return nil }
func (m *NamespaceCreateModel) SetDimensions(w, h int) { m.width, m.height = w, h }

// Reset clears the input state.
func (m *NamespaceCreateModel) Reset() {
	m.runes = m.runes[:0]
	m.cursor = 0
	m.err = ""
	m.buttons = nil
	m.focus = namespaceFocusInput
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

func (m *NamespaceCreateModel) moveFocus(delta int) tea.Cmd {
	total := 3
	next := (int(m.focus) + delta) % total
	if next < 0 {
		next += total
	}
	m.focus = namespaceFocus(next)
	return nil
}

func (m *NamespaceCreateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		switch key.String() {
		case "tab":
			return m, m.moveFocus(1)
		case "shift+tab":
			return m, m.moveFocus(-1)
		case "ctrl+c", "ctrl+g", "esc":
			return m, func() tea.Msg { return NamespaceCreateResultMsg{Close: true} }
		case "ctrl+h":
			if m.focus == namespaceFocusInput {
				m.deleteBackward()
				return m, nil
			}
		}
		if m.focus != namespaceFocusInput {
			switch key.String() {
			case "left":
				if m.focus == namespaceFocusCancel {
					m.focus = namespaceFocusCreate
				}
				return m, nil
			case "right":
				if m.focus == namespaceFocusCreate {
					m.focus = namespaceFocusCancel
				}
				return m, nil
			case "enter", " ":
				idx := 0
				if m.focus == namespaceFocusCancel {
					idx = 1
				}
				return m, m.executeButton(idx)
			}
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
			if _, ok := msg.(tea.MouseReleaseMsg); ok {
				if idx == 0 {
					m.focus = namespaceFocusCreate
				} else {
					m.focus = namespaceFocusCancel
				}
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

func (m *NamespaceCreateModel) View() (string, *tea.Cursor) {
	width := m.clampWidth(m.width)
	return m.buildLayout(width)
}

func (m *NamespaceCreateModel) renderHeader(width int) string {
	return lipgloss.NewStyle().
		Width(width).
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Bold(true).
		Align(lipgloss.Center).
		Render("Enter new namespace name")
}

func (m *NamespaceCreateModel) renderInputBlock(width int) (string, *tea.Cursor) {
	fieldWidth := max(24, width-6)
	renderedInput, cursorPos := m.renderInput(fieldWidth)
	inputLine := lipgloss.NewStyle().
		Width(width).
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Align(lipgloss.Center).
		Render(renderedInput)
	cursorX := max(0, (width-lipgloss.Width(renderedInput))/2+cursorPos)
	return inputLine, tea.NewCursor(cursorX, 0)
}

func (m *NamespaceCreateModel) renderStatus(width int) string {
	if strings.TrimSpace(m.err) == "" {
		return lipgloss.NewStyle().
			Width(width).
			Background(lipgloss.Color(uistyles.ColorModalBg)).
			Render("")
	}
	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Foreground(lipgloss.Color(uistyles.ColorModalSelBg)).
		Render(m.err)
}

func (m *NamespaceCreateModel) renderButtonsRow(width int) (string, []buttonRect) {
	wrap := func(content string) string {
		return lipgloss.NewStyle().
			Background(lipgloss.Color(uistyles.ColorModalBg)).
			Padding(0, 1).
			Render(content)
	}
	options := []string{
		wrap(m.renderButton("Create", m.focus == namespaceFocusCreate)),
		wrap(m.renderButton("Cancel", m.focus == namespaceFocusCancel)),
	}
	row := lipgloss.JoinHorizontal(lipgloss.Left, options[0], options[1])
	rowWidth := lipgloss.Width(options[0])
	line := uistyles.AlignCenter(width, row, lipgloss.NewStyle().Background(lipgloss.Color(uistyles.ColorModalBg)))
	contentWidth := lipgloss.Width(row)
	leftPad := max(0, (width-contentWidth)/2)
	rects := []buttonRect{
		{x: leftPad, y: 0, w: rowWidth, h: 1},
		{x: leftPad + rowWidth, y: 0, w: lipgloss.Width(options[1]), h: 1},
	}
	return line, rects
}

func (m *NamespaceCreateModel) renderButton(label string, focused bool) string {
	style := lipgloss.NewStyle().
		Padding(0, 3).
		Background(lipgloss.Color(uistyles.ColorModalButtonBg)).
		Foreground(lipgloss.Color(uistyles.ColorModalButtonFg))
	if focused {
		style = style.
			Background(lipgloss.Color(uistyles.ColorModalButtonSelBg)).
			Foreground(lipgloss.Color(uistyles.ColorModalButtonFg)).
			Bold(true)
	}
	return style.Render(label)
}

func (m *NamespaceCreateModel) renderInput(fieldWidth int) (string, int) {
	if fieldWidth <= 0 {
		fieldWidth = 1
	}
	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(uistyles.ColorWhite)).
		Background(lipgloss.Color(uistyles.ColorDarkGrey))
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.runes) {
		m.cursor = len(m.runes)
	}

	display := m.runes
	cursor := m.cursor
	if len(display) > fieldWidth {
		start := 0
		if cursor > fieldWidth {
			start = cursor - fieldWidth
			if start > len(display)-fieldWidth {
				start = len(display) - fieldWidth
			}
		}
		display = display[start : start+fieldWidth]
		cursor -= start
		if cursor < 0 {
			cursor = 0
		}
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
		b.WriteString(textStyle.Render(ch))
	}
	return b.String(), cursor
}

// FooterHints wires the modal footer hints.
func (m *NamespaceCreateModel) FooterHints() []FooterHint {
	return []FooterHint{
		{Key: "Enter", Label: "Create", Enabled: true},
		{Key: "Esc", Label: "Cancel", Enabled: true},
	}
}

func (m *NamespaceCreateModel) clampWidth(w int) int {
	if w <= 0 {
		return namespaceMinWidth
	}
	if w < namespaceMinWidth {
		return namespaceMinWidth
	}
	if w > namespaceMaxWidth {
		return namespaceMaxWidth
	}
	return w
}

func (m *NamespaceCreateModel) desiredWidth() int {
	base := lipgloss.Width("Enter new namespace name")
	input := lipgloss.Width(m.value()) + 4
	if input < namespaceMinWidth {
		input = namespaceMinWidth
	}
	buttons := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().Background(lipgloss.Color(uistyles.ColorModalBg)).Padding(0, 1).Render(m.renderButton("Create", false)),
		lipgloss.NewStyle().Background(lipgloss.Color(uistyles.ColorModalBg)).Padding(0, 1).Render(m.renderButton("Cancel", false)),
	)
	buttonWidth := lipgloss.Width(buttons)
	maxWidth := base
	maxWidth = max(maxWidth, input)
	maxWidth = max(maxWidth, buttonWidth)
	return maxWidth
}

func (m *NamespaceCreateModel) buildLayout(width int) (string, *tea.Cursor) {
	if width <= 0 {
		width = namespaceMinWidth
	}
	header := m.renderHeader(width)
	inputView, inputCursor := m.renderInputBlock(width)
	status := m.renderStatus(width)
	buttonRow, buttonRects := m.renderButtonsRow(width)

	layout := bl.New()
	addRow := func(view string) bl.ID {
		h := max(1, lipgloss.Height(view))
		id := layout.Add(fmt.Sprintf("height %d!", h))
		layout.Wrap()
		return id
	}
	headerID := addRow(header)
	inputID := addRow(inputView)
	statusID := addRow(status)
	buttonID := addRow(buttonRow)

	totalHeight := lipgloss.Height(header) + lipgloss.Height(inputView) + lipgloss.Height(status) + lipgloss.Height(buttonRow)
	if totalHeight < namespaceMinHeight {
		totalHeight = namespaceMinHeight
	}
	msg := layout.Resize(width, totalHeight)
	canvas := lipgloss.NewStyle().
		Width(width).
		Height(totalHeight).
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Foreground(lipgloss.Color(uistyles.ColorModalFg)).
		Render("")

	place := func(content string, id bl.ID) {
		if size, err := msg.Size(id); err == nil {
			canvas = overlay.Composite(content, canvas, overlay.Left, overlay.Top, size.X, size.Y)
		}
	}
	place(header, headerID)
	place(inputView, inputID)
	place(status, statusID)
	place(buttonRow, buttonID)

	if size, err := msg.Size(buttonID); err == nil {
		m.buttons = make([]buttonRect, len(buttonRects))
		for i, r := range buttonRects {
			m.buttons[i] = buttonRect{
				x: size.X + r.x,
				y: size.Y + r.y,
				w: r.w,
				h: r.h,
			}
		}
	} else {
		m.buttons = nil
	}

	var cursor *tea.Cursor
	if inputCursor != nil {
		if c, err := msg.OffsetCursor(inputID, inputCursor); err == nil {
			cursor = c
		}
	}
	return canvas, cursor
}

// PreferredSize implements ModalSizer.
func (m *NamespaceCreateModel) PreferredSize(maxContentWidth, maxContentHeight int) (int, int) {
	width := m.desiredWidth()
	if width < namespaceMinWidth {
		width = namespaceMinWidth
	}
	if width > namespaceMaxWidth {
		width = namespaceMaxWidth
	}
	if maxContentWidth > 0 && width > maxContentWidth {
		width = maxContentWidth
	}
	canvas, _ := m.buildLayout(width)
	height := lipgloss.Height(canvas)
	if height < namespaceMinHeight {
		height = namespaceMinHeight
	}
	if maxContentHeight > 0 && height > maxContentHeight {
		height = maxContentHeight
	}
	return width, height
}
