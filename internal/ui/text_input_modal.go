package ui

import (
	"strings"

	textinput "github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sttts/kc/internal/overlay"
	uistyles "github.com/sttts/kc/internal/ui/styles"
)

// TextInputModalResult describes the outcome of a text input modal interaction.
type TextInputModalResult struct {
	Value   string
	Confirm bool
	Close   bool
}

// TextInputModalResultMsg is emitted when no custom result builder is provided.
type TextInputModalResultMsg TextInputModalResult

// TextInputModalButtonStyles customizes confirm/cancel button appearance.
type TextInputModalButtonStyles struct {
	Confirm        *lipgloss.Style
	ConfirmFocused *lipgloss.Style
	Cancel         *lipgloss.Style
	CancelFocused  *lipgloss.Style
}

// TextInputModalConfig controls the appearance and behaviour of TextInputModal.
type TextInputModalConfig struct {
	Title        string
	Label        string
	Description  string
	ConfirmLabel string
	CancelLabel  string
	Placeholder  string
	InitialValue string
	MinWidth     int
	MaxWidth     int
	MinHeight    int
	MaxHeight    int
	Validate     func(string) error
	ResultMsg    func(TextInputModalResult) tea.Msg
	ButtonStyles TextInputModalButtonStyles
	HideTitle    bool
}

type textInputModalFocus int

const (
	textInputFocusField textInputModalFocus = iota
	textInputFocusConfirm
	textInputFocusCancel
)

// TextInputModal renders a labelled text input with confirm/cancel buttons.
type TextInputModal struct {
	cfg           TextInputModalConfig
	input         textinput.Model
	focus         textInputModalFocus
	errMsg        string
	width, height int
	buttonBounds  []buttonRect
}

// NewTextInputModal constructs a TextInputModal with the given configuration.
func NewTextInputModal(cfg TextInputModalConfig) *TextInputModal {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = cfg.Placeholder
	ti.CharLimit = 0
	ti.VirtualCursor = false
	ti.Styles = modalTextInputStyles()
	ti.SetValue(cfg.InitialValue)
	ti.CursorEnd()
	_ = ti.Focus()
	return &TextInputModal{
		cfg:   cfg,
		input: ti,
		focus: textInputFocusField,
	}
}

func modalTextInputStyles() textinput.Styles {
	styles := textinput.DefaultLightStyles()
	focused := lipgloss.NewStyle().
		Foreground(lipgloss.Color(uistyles.ColorWhite)).
		Background(lipgloss.Color(uistyles.ColorDarkGrey)).
		Bold(true)
	blurred := lipgloss.NewStyle().
		Foreground(lipgloss.Color(uistyles.ColorGrey)).
		Background(lipgloss.Color(uistyles.ColorDarkGrey))
	styles.Focused.Text = focused
	styles.Blurred.Text = blurred
	styles.Focused.Placeholder = focused.Copy().Foreground(lipgloss.Color(uistyles.ColorModalFg))
	styles.Blurred.Placeholder = blurred.Copy().Foreground(lipgloss.Color(uistyles.ColorModalFg))
	styles.Focused.Prompt = focused
	styles.Blurred.Prompt = blurred
	styles.Cursor.Color = lipgloss.Color(uistyles.ColorModalCursor)
	return styles
}

// Init implements tea.Model.
func (m *TextInputModal) Init() tea.Cmd { return nil }

// SetDimensions records the available content dimensions.
func (m *TextInputModal) SetDimensions(w, h int) {
	m.width = w
	m.height = h
}

// PreferredSize implements ModalSizer.
func (m *TextInputModal) PreferredSize(maxContentWidth, maxContentHeight int) (int, int) {
	width := m.clampWidth(m.desiredWidth())
	if maxContentWidth > 0 && width > maxContentWidth {
		width = maxContentWidth
	}
	content, _ := m.render(width)
	height := lipgloss.Height(content)
	height = m.clampHeight(height)
	if maxContentHeight > 0 && height > maxContentHeight {
		height = maxContentHeight
	}
	return width, height
}

// View renders the modal content and cursor.
func (m *TextInputModal) View() (string, *tea.Cursor) {
	width := m.clampWidth(m.width)
	if width <= 0 {
		width = m.clampWidth(m.desiredWidth())
	}
	if width <= 0 {
		width = 40
	}
	return m.render(width)
}

// Update handles Tea messages.
func (m *TextInputModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		switch key.String() {
		case "tab":
			return m, m.moveFocus(1)
		case "shift+tab":
			return m, m.moveFocus(-1)
		case "esc", "ctrl+c", "ctrl+g":
			return m, m.emitResult(false, true)
		}
		if m.focus == textInputFocusField {
			before := m.input.Value()
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(key)
			if m.errMsg != "" && m.input.Value() != before {
				m.errMsg = ""
			}
			// Let Enter submit from the input field.
			if key.Key().Code == tea.KeyEnter {
				if submit := m.validateAndSubmit(); submit != nil {
					return m, submit
				}
			}
			return m, cmd
		}
		switch key.Key().Code {
		case tea.KeyEnter:
			if m.focus == textInputFocusCancel {
				return m, m.emitResult(false, true)
			}
			return m, m.validateAndSubmit()
		case tea.KeyLeft:
			if m.focus != textInputFocusField {
				return m, m.moveFocus(-1)
			}
		case tea.KeyRight:
			if m.focus != textInputFocusField {
				return m, m.moveFocus(1)
			}
		}
	case tea.MouseMsg:
		return m, m.handleMouse(key)
	}
	return m, nil
}

// FooterHints implements ModalFooterHints.
func (m *TextInputModal) FooterHints() []FooterHint {
	return []FooterHint{
		{Key: "Enter", Label: m.confirmLabel(), Enabled: true},
		{Key: "Esc", Label: m.cancelLabel(), Enabled: true},
	}
}

// FocusInput focuses the text field.
func (m *TextInputModal) FocusInput() tea.Cmd {
	m.focus = textInputFocusField
	return m.input.Focus()
}

// BlurInput blurs the text field.
func (m *TextInputModal) BlurInput() {
	m.input.Blur()
	if m.focus == textInputFocusField {
		m.focus = textInputFocusConfirm
	}
}

// SetTitle updates the modal title.
func (m *TextInputModal) SetTitle(title string) { m.cfg.Title = title }

// SetLabel updates the field label.
func (m *TextInputModal) SetLabel(label string) { m.cfg.Label = label }

// SetDescription updates the supporting description/hint text.
func (m *TextInputModal) SetDescription(desc string) { m.cfg.Description = desc }

// SetConfirmLabel updates the confirm button label.
func (m *TextInputModal) SetConfirmLabel(label string) { m.cfg.ConfirmLabel = label }

// SetCancelLabel updates the cancel button label.
func (m *TextInputModal) SetCancelLabel(label string) { m.cfg.CancelLabel = label }

// SetPlaceholder changes the placeholder text.
func (m *TextInputModal) SetPlaceholder(placeholder string) { m.input.Placeholder = placeholder }

// SetButtonStyles overrides the confirm/cancel button styling.
func (m *TextInputModal) SetButtonStyles(styles TextInputModalButtonStyles) {
	m.cfg.ButtonStyles = styles
}

// SetValue replaces the current text input value.
func (m *TextInputModal) SetValue(val string) {
	m.input.SetValue(val)
	m.input.CursorEnd()
}

// Value returns the trimmed text input value.
func (m *TextInputModal) Value() string { return strings.TrimSpace(m.input.Value()) }

// SetValidate overrides the validation function.
func (m *TextInputModal) SetValidate(fn func(string) error) { m.cfg.Validate = fn }

// SetResultBuilder overrides the result message factory.
func (m *TextInputModal) SetResultBuilder(fn func(TextInputModalResult) tea.Msg) {
	m.cfg.ResultMsg = fn
}

// ClearError resets any displayed validation error.
func (m *TextInputModal) ClearError() { m.errMsg = "" }

// Error returns the last validation error, if any.
func (m *TextInputModal) Error() string { return m.errMsg }

func (m *TextInputModal) handleMouse(msg tea.MouseMsg) tea.Cmd {
	if len(m.buttonBounds) == 0 {
		return nil
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return nil
	}
	for idx, rect := range m.buttonBounds {
		if !rect.contains(mouse.X, mouse.Y) {
			continue
		}
		switch msg.(type) {
		case tea.MouseClickMsg:
			if idx == 0 {
				m.focus = textInputFocusConfirm
			} else {
				m.focus = textInputFocusCancel
			}
			return nil
		case tea.MouseReleaseMsg:
			if idx == 0 {
				m.focus = textInputFocusConfirm
				return m.validateAndSubmit()
			}
			m.focus = textInputFocusCancel
			return m.emitResult(false, true)
		}
		break
	}
	return nil
}

func (m *TextInputModal) render(width int) (string, *tea.Cursor) {
	var sections []string
	totalHeight := 0
	add := func(s string) {
		sections = append(sections, s)
		totalHeight += lipgloss.Height(s)
	}

	if !m.cfg.HideTitle && strings.TrimSpace(m.cfg.Title) != "" {
		title := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Background(lipgloss.Color(uistyles.ColorModalBg)).
			Foreground(lipgloss.Color(uistyles.ColorModalFg)).
			Bold(true).
			Render(m.cfg.Title)
		add(title)
	}

	label := lipgloss.NewStyle().
		Width(width).
		Padding(0, 2).
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Foreground(lipgloss.Color(uistyles.ColorModalFg)).
		Bold(true).
		Render(m.cfg.Label)
	add(label)

	field, fieldCursor := m.renderField(width)
	fieldOffset := totalHeight
	desc := m.renderDescription(width)

	buttonRow := ""
	var buttonRects []buttonRect
	if m.confirmLabel() != "" || m.cancelLabel() != "" {
		buttonRow, buttonRects = m.renderButtonsRow(width)
	}

	add(field)
	add(desc)
	preButtonsHeight := totalHeight
	if buttonRow != "" {
		add(buttonRow)
	}

	view := lipgloss.JoinVertical(lipgloss.Left, sections...)

	m.buttonBounds = m.buttonBounds[:0]
	if len(buttonRects) > 0 {
		m.buttonBounds = make([]buttonRect, len(buttonRects))
		for i, r := range buttonRects {
			m.buttonBounds[i] = buttonRect{
				x: r.x,
				y: preButtonsHeight + r.y,
				w: r.w,
				h: r.h,
			}
		}
	}

	root := lipgloss.NewStyle().
		Width(width).
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Render(view)

	if fieldCursor != nil {
		fieldCursor.Position.Y += fieldOffset
	}
	return root, fieldCursor
}

func (m *TextInputModal) renderField(width int) (string, *tea.Cursor) {
	const sidePad = 2
	fieldWidth := max(10, width-(sidePad*2))
	m.input.SetWidth(fieldWidth)
	base := lipgloss.NewStyle().
		Width(width).
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Render(strings.Repeat(" ", width))
	inputBg := lipgloss.NewStyle().
		Width(fieldWidth).
		Background(lipgloss.Color(uistyles.ColorDarkGrey)).
		Render(strings.Repeat(" ", fieldWidth))
	row := overlay.Composite(inputBg, base, overlay.Left, overlay.Top, sidePad, 0)
	row = overlay.Composite(m.input.View(), row, overlay.Left, overlay.Top, sidePad, 0)
	cursor := m.input.Cursor()
	if cursor != nil {
		cursor.Position.X += sidePad
	}
	return row, cursor
}

func (m *TextInputModal) renderDescription(width int) string {
	desc := m.cfg.Description
	style := lipgloss.NewStyle().
		Width(width).
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Foreground(lipgloss.Color(uistyles.ColorModalFg)).
		Faint(true).
		Padding(0, 2)
	if strings.TrimSpace(desc) == "" {
		desc = " "
	}
	if strings.TrimSpace(m.errMsg) != "" {
		desc = m.errMsg
		style = style.Foreground(lipgloss.Color("196")).Faint(false)
	}
	return style.Render(desc)
}

func (m *TextInputModal) renderButtonsRow(width int) (string, []buttonRect) {
	wrap := func(content string) string {
		return lipgloss.NewStyle().
			Background(lipgloss.Color(uistyles.ColorModalBg)).
			Padding(0, 1).
			Render(content)
	}
	confirmButton := wrap(m.renderButton(m.confirmLabel(), m.focus == textInputFocusConfirm, true))
	cancelButton := wrap(m.renderButton(m.cancelLabel(), m.focus == textInputFocusCancel, false))
	content := lipgloss.JoinHorizontal(lipgloss.Left, confirmButton, cancelButton)
	row := uistyles.AlignCenter(width, content, lipgloss.NewStyle().Background(lipgloss.Color(uistyles.ColorModalBg)))
	contentWidth := lipgloss.Width(content)
	leftPad := max(0, (width-contentWidth)/2)
	rects := []buttonRect{
		{x: leftPad, y: 0, w: lipgloss.Width(confirmButton), h: lipgloss.Height(confirmButton)},
		{x: leftPad + lipgloss.Width(confirmButton), y: 0, w: lipgloss.Width(cancelButton), h: lipgloss.Height(cancelButton)},
	}
	return row, rects
}

func (m *TextInputModal) renderButton(label string, focused bool, confirm bool) string {
	if strings.TrimSpace(label) == "" {
		label = " "
	}
	style := lipgloss.NewStyle().
		Padding(0, 3).
		Background(lipgloss.Color(uistyles.ColorModalButtonBg)).
		Foreground(lipgloss.Color(uistyles.ColorModalButtonFg))
	focusedStyle := style.Copy().
		Background(lipgloss.Color(uistyles.ColorModalButtonSelBg)).
		Foreground(lipgloss.Color(uistyles.ColorModalButtonFg)).
		Bold(true)
	if confirm {
		if focused && m.cfg.ButtonStyles.ConfirmFocused != nil {
			return m.cfg.ButtonStyles.ConfirmFocused.Render(label)
		}
		if !focused && m.cfg.ButtonStyles.Confirm != nil {
			return m.cfg.ButtonStyles.Confirm.Render(label)
		}
		if focused {
			return focusedStyle.Render(label)
		}
		return style.Render(label)
	}
	if focused && m.cfg.ButtonStyles.CancelFocused != nil {
		return m.cfg.ButtonStyles.CancelFocused.Render(label)
	}
	if !focused && m.cfg.ButtonStyles.Cancel != nil {
		return m.cfg.ButtonStyles.Cancel.Render(label)
	}
	if focused {
		return focusedStyle.Render(label)
	}
	return style.Render(label)
}

func (m *TextInputModal) desiredWidth() int {
	width := 0
	if !m.cfg.HideTitle {
		width = lipgloss.Width(m.cfg.Title)
	}
	labelWidth := lipgloss.Width(m.cfg.Label) + 4
	descWidth := lipgloss.Width(m.cfg.Description) + 4
	inputWidth := lipgloss.Width(m.input.Value()) + 8
	confirm := m.renderButton(m.confirmLabel(), false, true)
	cancel := m.renderButton(m.cancelLabel(), false, false)
	buttonWidth := lipgloss.Width(confirm) + lipgloss.Width(cancel) + 4
	for _, candidate := range []int{labelWidth, descWidth, inputWidth, buttonWidth} {
		if candidate > width {
			width = candidate
		}
	}
	return m.clampWidth(width)
}

func (m *TextInputModal) clampWidth(w int) int {
	minWidth := m.cfg.MinWidth
	if minWidth <= 0 {
		minWidth = 24
	}
	if w < minWidth {
		w = minWidth
	}
	maxWidth := m.cfg.MaxWidth
	if maxWidth > 0 && w > maxWidth {
		w = maxWidth
	}
	return w
}

func (m *TextInputModal) clampHeight(h int) int {
	minHeight := m.cfg.MinHeight
	if minHeight <= 0 {
		minHeight = 5
	}
	if h < minHeight {
		h = minHeight
	}
	maxHeight := m.cfg.MaxHeight
	if maxHeight > 0 && h > maxHeight {
		h = maxHeight
	}
	return h
}

func (m *TextInputModal) moveFocus(delta int) tea.Cmd {
	next := (int(m.focus) + delta) % 3
	if next < 0 {
		next += 3
	}
	m.focus = textInputModalFocus(next)
	if m.focus == textInputFocusField {
		return m.input.Focus()
	}
	m.input.Blur()
	return nil
}

func (m *TextInputModal) validateAndSubmit() tea.Cmd {
	value := strings.TrimSpace(m.input.Value())
	if fn := m.cfg.Validate; fn != nil {
		if err := fn(value); err != nil {
			m.errMsg = err.Error()
			return nil
		}
	}
	m.errMsg = ""
	return m.emitResult(true, true)
}

func (m *TextInputModal) emitResult(confirm, close bool) tea.Cmd {
	result := TextInputModalResult{
		Value:   strings.TrimSpace(m.input.Value()),
		Confirm: confirm,
		Close:   close,
	}
	builder := m.cfg.ResultMsg
	if builder == nil {
		builder = func(res TextInputModalResult) tea.Msg { return TextInputModalResultMsg(res) }
	}
	return func() tea.Msg { return builder(result) }
}

func (m *TextInputModal) confirmLabel() string {
	if label := strings.TrimSpace(m.cfg.ConfirmLabel); label != "" {
		return label
	}
	return "OK"
}

func (m *TextInputModal) cancelLabel() string {
	if label := strings.TrimSpace(m.cfg.CancelLabel); label != "" {
		return label
	}
	return "Cancel"
}
