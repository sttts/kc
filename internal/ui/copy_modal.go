package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	textinput "github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sttts/kc/internal/overlay"
	uistyles "github.com/sttts/kc/internal/ui/styles"
	bl "github.com/sttts/kc/third_party/bubblelayout"
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

const (
	copyMinWidth  = 24
	copyMaxWidth  = 80
	copyMinHeight = 6
)

// CopyToLocalModel renders the "Copy to local" dialog with a single path input.
type CopyToLocalModel struct {
	width, height int
	subject       string
	pathInput     textinput.Model
	focus         copyFieldFocus
	err           string
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
	width := m.clampWidth(m.width)
	return m.buildLayout(width)
}

func (m *CopyToLocalModel) renderTitle(width int) string {
	text := fmt.Sprintf("Copy %s to local file", strings.TrimSpace(m.subject))
	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Bold(true).
		Render(text)
}

func (m *CopyToLocalModel) renderPathField(width int) (string, *tea.Cursor) {
	label := lipgloss.NewStyle().
		Width(width).
		Padding(0, 2).
		Bold(true).
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Render("File path")

	fieldWidth := max(10, width-4)
	m.pathInput.SetWidth(fieldWidth)
	inputBox := lipgloss.NewStyle().
		Width(width).
		Padding(0, 2).
		Background(lipgloss.Color(uistyles.ColorDarkGrey)).
		Foreground(lipgloss.Color(uistyles.ColorWhite)).
		Render(m.pathInput.View())

	rows := []string{
		label,
		lipgloss.NewStyle().
			Width(width).
			Background(lipgloss.Color(uistyles.ColorModalBg)).
			Render(inputBox),
	}
	view := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return view, m.pathInput.Cursor()
}

func (m *CopyToLocalModel) renderHint(width int) string {
	text := m.hintText()
	style := lipgloss.NewStyle().
		Width(width).
		Padding(0, 2).
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Faint(true)
	if strings.TrimSpace(m.err) != "" {
		style = style.
			Faint(false).
			Foreground(lipgloss.Color("196"))
		text = m.err
	}
	return style.Render(text)
}

func (m *CopyToLocalModel) renderButtonsRow(width int) string {
	wrap := func(content string) string {
		return lipgloss.NewStyle().
			Background(lipgloss.Color(uistyles.ColorModalBg)).
			Padding(0, 1).
			Render(content)
	}
	copyBtn := wrap(m.buttonView("Copy", m.focus == copyFocusConfirm))
	cancelBtn := wrap(m.buttonView("Cancel", m.focus == copyFocusCancel))
	row := lipgloss.JoinHorizontal(lipgloss.Left, copyBtn, cancelBtn)
	return uistyles.AlignCenter(width, row, lipgloss.NewStyle().Background(lipgloss.Color(uistyles.ColorModalBg)))
}

func (m *CopyToLocalModel) buttonView(label string, focused bool) string {
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

func (m *CopyToLocalModel) hintText() string {
	if strings.TrimSpace(m.err) != "" {
		return m.err
	}
	return "Enter an absolute file path. Existing files will be overwritten."
}

func (m *CopyToLocalModel) clampWidth(w int) int {
	if w <= 0 {
		return copyMinWidth
	}
	if w < copyMinWidth {
		return copyMinWidth
	}
	if w > copyMaxWidth {
		return copyMaxWidth
	}
	return w
}

func (m *CopyToLocalModel) desiredWidth() int {
	title := fmt.Sprintf("Copy %s to local file", strings.TrimSpace(m.subject))
	hint := m.hintText()
	copyBtn := m.buttonView("Copy", false)
	cancelBtn := m.buttonView("Cancel", false)
	buttons := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().Background(lipgloss.Color(uistyles.ColorModalBg)).Padding(0, 1).Render(copyBtn),
		lipgloss.NewStyle().Background(lipgloss.Color(uistyles.ColorModalBg)).Padding(0, 1).Render(cancelBtn),
	)
	maxWidth := lipgloss.Width(title)
	maxWidth = max(maxWidth, lipgloss.Width(hint))
	maxWidth = max(maxWidth, lipgloss.Width(buttons))
	labelWidth := lipgloss.Width("File path") + 4
	fieldWidth := lipgloss.Width(m.pathInput.Value()) + 4
	maxWidth = max(maxWidth, labelWidth)
	maxWidth = max(maxWidth, fieldWidth)
	return maxWidth
}

func (m *CopyToLocalModel) buildLayout(width int) (string, *tea.Cursor) {
	if width <= 0 {
		width = copyMinWidth
	}
	title := m.renderTitle(width)
	fieldView, fieldCursor := m.renderPathField(width)
	hint := m.renderHint(width)
	buttons := m.renderButtonsRow(width)

	layout := bl.New()
	addRow := func(view string) bl.ID {
		h := max(1, lipgloss.Height(view))
		id := layout.Add(fmt.Sprintf("height %d!", h))
		layout.Wrap()
		return id
	}
	titleID := addRow(title)
	fieldID := addRow(fieldView)
	hintID := addRow(hint)
	buttonID := addRow(buttons)

	totalHeight := lipgloss.Height(title) + lipgloss.Height(fieldView) + lipgloss.Height(hint) + lipgloss.Height(buttons)
	if totalHeight < copyMinHeight {
		totalHeight = copyMinHeight
	}
	msg := layout.Resize(width, totalHeight)
	canvas := lipgloss.NewStyle().
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Foreground(lipgloss.Color(uistyles.ColorModalFg)).
		Width(width).
		Height(totalHeight).
		Render("")
	place := func(content string, id bl.ID) {
		if size, err := msg.Size(id); err == nil {
			canvas = overlay.Composite(content, canvas, overlay.Left, overlay.Top, size.X, size.Y)
		}
	}
	place(title, titleID)
	place(fieldView, fieldID)
	place(hint, hintID)
	place(buttons, buttonID)

	var cursor *tea.Cursor
	if fieldCursor != nil {
		fieldCursor.Position.Y += 1
		fieldCursor.Position.X += 2
		if c, err := msg.OffsetCursor(fieldID, fieldCursor); err == nil {
			cursor = c
		}
	}
	return canvas, cursor
}

// PreferredSize implements ModalSizer.
func (m *CopyToLocalModel) PreferredSize(maxContentWidth, maxContentHeight int) (int, int) {
	width := m.desiredWidth()
	if width < copyMinWidth {
		width = copyMinWidth
	}
	if width > copyMaxWidth {
		width = copyMaxWidth
	}
	if maxContentWidth > 0 && width > maxContentWidth {
		width = maxContentWidth
	}
	canvas, _ := m.buildLayout(width)
	height := lipgloss.Height(canvas)
	if height < copyMinHeight {
		height = copyMinHeight
	}
	if maxContentHeight > 0 && height > maxContentHeight {
		height = maxContentHeight
	}
	return width, height
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
