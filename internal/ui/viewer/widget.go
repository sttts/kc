package viewer

import (
	"fmt"
	"strings"

	chroma "github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

// Metadata describes the highlighted content.
type Metadata struct {
	Title    string
	Language string
	MimeType string
	Filename string
}

// Frame describes the drawing area granted to the viewer.
type Frame struct {
	Width   int
	Height  int
	Focused bool
}

// Widget renders highlighted text with scrolling support.
type Widget struct {
	metadata Metadata

	width  int
	height int

	raw      string
	rawLines []string
	lines    []string
	theme    string

	offset  int
	hOffset int

	onEdit  func() tea.Cmd
	onTheme func() tea.Cmd
	onClose func() tea.Cmd
}

// New constructs a viewer widget with an optional theme name.
func New(theme string) *Widget {
	if theme == "" {
		theme = "dracula"
	}
	return &Widget{theme: theme}
}

// SetContent replaces the text body and metadata, re-running syntax highlighting.
func (w *Widget) SetContent(text string, meta Metadata) {
	w.raw = text
	w.metadata = meta
	w.rawLines = strings.Split(text, "\n")
	w.highlight()
	w.offset = 0
	w.hOffset = 0
}

// SetTheme updates the chroma theme name and re-highlights the content.
func (w *Widget) SetTheme(theme string) {
	if theme == "" || theme == w.theme {
		return
	}
	w.theme = theme
	w.highlight()
}

// SetCallbacks configures optional edit/theme/close handlers.
func (w *Widget) SetCallbacks(onEdit, onTheme, onClose func() tea.Cmd) {
	w.onEdit = onEdit
	w.onTheme = onTheme
	w.onClose = onClose
}

// Resize updates the viewport size.
func (w *Widget) Resize(width, height int) {
	w.width = width
	w.height = height
}

// Update processes key/mouse input. Returns a command when an action is triggered.
func (w *Widget) Update(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		if cmd, handled := w.handleKey(m.String()); handled {
			return cmd, true
		}
	case tea.MouseWheelMsg:
		mouse := tea.Mouse(m)
		switch mouse.Button {
		case tea.MouseWheelUp:
			w.scrollBy(-1)
			return nil, true
		case tea.MouseWheelDown:
			w.scrollBy(1)
			return nil, true
		}
	}
	return nil, false
}

// View renders the highlighted text inside the frame.
func (w *Widget) View(frame Frame) string {
	w.Resize(frame.Width, frame.Height)
	lines := w.visibleLines()
	lineStyle := lipgloss.NewStyle().Width(max(1, frame.Width))
	if frame.Focused {
		lineStyle = lineStyle.Bold(true)
	}
	rendered := make([]string, len(lines))
	for i, line := range lines {
		rendered[i] = lineStyle.Render(line)
	}
	if len(rendered) == 0 {
		return lineStyle.Render("")
	}
	return lipgloss.JoinVertical(lipgloss.Left, rendered...)
}

// Footer returns viewer status (title/position) rendered into a footer row.
func (w *Widget) Footer(width int) string {
	info := w.metadata.Title
	if info == "" {
		info = w.metadata.Filename
	}
	if info == "" {
		info = "Manifest"
	}
	status := fmt.Sprintf("%s • %d/%d", info, min(len(w.lines), w.offset+w.page()), len(w.lines))
	return lipgloss.NewStyle().Width(max(1, width)).Render(status)
}

// Metadata returns the current content metadata.
func (w *Widget) Metadata() Metadata { return w.metadata }

func (w *Widget) highlight() {
	if w.raw == "" {
		w.lines = []string{""}
		return
	}
	ensureCustomStyles()
	lexer := w.selectLexer()
	iterator, err := lexer.Tokenise(nil, w.raw)
	if err != nil {
		w.lines = strings.Split(w.raw, "\n")
		return
	}
	var rendered string
	if w.theme == "turbo-pascal" {
		rendered = formatTurboPascalANSI(iterator)
	} else {
		style := styles.Get(w.theme)
		if style == nil {
			style = styles.Monokai
		}
		rendered = formatTTY16mWithPanelBG(style, iterator)
	}
	rendered = strings.TrimRight(rendered, "\n")
	w.lines = strings.Split(rendered, "\n")
	if len(w.lines) == 0 {
		w.lines = []string{""}
	}
}

func (w *Widget) selectLexer() chroma.Lexer {
	if w.metadata.Language != "" {
		if lex := lexers.Get(w.metadata.Language); lex != nil {
			return lex
		}
	}
	if w.metadata.MimeType != "" {
		if lex := lexers.MatchMimeType(w.metadata.MimeType); lex != nil {
			return lex
		}
	}
	if w.metadata.Filename != "" {
		if lex := lexers.Match(w.metadata.Filename); lex != nil {
			return lex
		}
	}
	if analyser := lexers.Analyse(w.raw); analyser != nil {
		return analyser
	}
	return lexers.Fallback
}

func (w *Widget) handleKey(key string) (tea.Cmd, bool) {
	switch key {
	case "up", "k":
		w.scrollBy(-1)
	case "down", "j":
		w.scrollBy(1)
	case "left", "h":
		if w.hOffset > 0 {
			w.hOffset--
		}
	case "right", "l":
		w.hOffset++
	case "pgup", "ctrl+b":
		w.scrollBy(-w.page())
	case "pgdown", "ctrl+f":
		w.scrollBy(w.page())
	case "home", "g":
		w.offset = 0
	case "end", "G":
		w.offset = max(0, len(w.lines)-w.page())
	case "ctrl+a":
		w.hOffset = 0
	case "ctrl+e":
		w.alignToEnd()
	case "f4":
		if w.onEdit != nil {
			return w.onEdit(), true
		}
	case "f2":
		if w.onTheme != nil {
			return w.onTheme(), true
		}
	case "f10":
		if w.onClose != nil {
			return w.onClose(), true
		}
	default:
		return nil, false
	}
	return nil, true
}

func (w *Widget) scrollBy(delta int) {
	w.offset += delta
	if w.offset < 0 {
		w.offset = 0
	}
	maxScroll := max(0, len(w.lines)-w.page())
	if w.offset > maxScroll {
		w.offset = maxScroll
	}
}

func (w *Widget) alignToEnd() {
	start := w.offset
	end := min(len(w.rawLines), w.offset+w.page())
	maxLen := 0
	for i := start; i < end; i++ {
		if length := runeWidth(w.rawLines[i]); length > maxLen {
			maxLen = length
		}
	}
	if maxLen > w.width {
		w.hOffset = maxLen - w.width
	} else {
		w.hOffset = 0
	}
}

func (w *Widget) visibleLines() []string {
	if len(w.lines) == 0 {
		return []string{""}
	}
	start := w.offset
	end := min(len(w.lines), start+w.page())
	segment := w.lines[start:end]
	result := make([]string, w.page())
	for i := range result {
		if i < len(segment) {
			result[i] = trimANSI(segment[i], w.hOffset, w.width)
		} else {
			result[i] = ""
		}
	}
	return result
}

func (w *Widget) page() int {
	if w.height <= 0 {
		return 1
	}
	return w.height
}

func trimANSI(s string, start, width int) string {
	if width <= 0 {
		return ""
	}
	return sliceANSI(s, start, width)
}

// ANSI helpers --------------------------------------------------------------

func sliceANSI(s string, start, width int) string {
	raw := sliceANSIColsRaw(s, start, width)
	if !strings.HasSuffix(raw, "\x1b[0m") {
		raw += "\x1b[0m"
	}
	return raw
}

func sliceANSIColsRaw(s string, start, width int) string {
	if start < 0 {
		start = 0
	}
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	col := 0
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				b.WriteString(s[i : j+1])
				i = j + 1
				continue
			}
		}
		if col >= start && col < start+width {
			b.WriteByte(s[i])
		}
		col++
		if col >= start+width {
			break
		}
		i++
	}
	return b.String()
}

func runeWidth(s string) int {
	return lipgloss.Width(s)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
