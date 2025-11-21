package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	uistyles "github.com/sttts/kc/internal/ui/styles"
)

var helpLineStyle = lipgloss.NewStyle().
	Background(lipgloss.Color(uistyles.ColorModalBg)).
	Foreground(lipgloss.Color(uistyles.ColorModalFg))

// MarkdownHelpViewer renders embedded markdown content inside a windowed modal.
type MarkdownHelpViewer struct {
	markdown string
	rendered []string
	width    int
	height   int
	scroll   int
}

// NewMarkdownHelpViewer constructs a viewer seeded with the provided markdown.
func NewMarkdownHelpViewer(md string) *MarkdownHelpViewer {
	v := &MarkdownHelpViewer{}
	v.SetMarkdown(md)
	return v
}

// Init implements tea.Model.
func (v *MarkdownHelpViewer) Init() tea.Cmd { return nil }

// SetDimensions applies the viewport dimensions (characters).
func (v *MarkdownHelpViewer) SetDimensions(w, h int) {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	if w != v.width {
		v.width = w
		v.render()
	} else {
		v.width = w
	}
	v.height = h
	v.clampScroll()
}

// SetMarkdown replaces the markdown source and resets scroll.
func (v *MarkdownHelpViewer) SetMarkdown(md string) {
	v.markdown = strings.TrimSpace(md)
	v.scroll = 0
	v.render()
}

// ScrollTop resets the viewport to the first line.
func (v *MarkdownHelpViewer) ScrollTop() { v.scroll = 0 }

func (v *MarkdownHelpViewer) render() {
	if v.markdown == "" {
		v.rendered = []string{""}
		return
	}
	wrap := v.width
	if wrap <= 0 {
		wrap = 80
	}
	// Leave a small gutter inside the modal border.
	wrap = max(20, wrap-2)
	style := glamour.LightStyleConfig
	bg := uistyles.ColorModalBg
	fg := uistyles.ColorModalFg
	style.Document.StylePrimitive.BackgroundColor = stringPtr(bg)
	style.Document.StylePrimitive.Color = stringPtr(fg)
	style.Paragraph.StylePrimitive.BackgroundColor = stringPtr(bg)
	style.Paragraph.StylePrimitive.Color = stringPtr(fg)
	style.List.StyleBlock.StylePrimitive.BackgroundColor = stringPtr(bg)
	style.List.StyleBlock.StylePrimitive.Color = stringPtr(fg)
	style.BlockQuote.StylePrimitive.BackgroundColor = stringPtr(bg)
	style.BlockQuote.StylePrimitive.Color = stringPtr(fg)
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(wrap),
	)
	result := v.markdown
	if err == nil {
		if out, rErr := renderer.Render(v.markdown); rErr == nil {
			result = out
		}
	}
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	v.rendered = lines
	v.clampScroll()
}

func (v *MarkdownHelpViewer) clampScroll() {
	if v.height <= 0 {
		v.scroll = 0
		return
	}
	maxScroll := len(v.rendered) - v.height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if v.scroll > maxScroll {
		v.scroll = maxScroll
	}
	if v.scroll < 0 {
		v.scroll = 0
	}
}

// Update processes key/mouse input for scrolling.
func (v *MarkdownHelpViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch m.String() {
		case "up", "k":
			v.scrollBy(-1)
		case "down", "j":
			v.scrollBy(1)
		case "pgup", "ctrl+b":
			v.scrollBy(-v.pageSize())
		case "pgdown", "ctrl+f":
			v.scrollBy(v.pageSize())
		case "home", "g":
			v.scroll = 0
		case "end", "G":
			v.scroll = len(v.rendered) - v.height
			v.clampScroll()
		}
	case tea.MouseWheelMsg:
		mouse := tea.Mouse(m)
		switch mouse.Button {
		case tea.MouseWheelUp:
			v.scrollBy(-1)
		case tea.MouseWheelDown:
			v.scrollBy(1)
		}
	}
	return v, nil
}

func (v *MarkdownHelpViewer) pageSize() int {
	if v.height <= 0 {
		return 1
	}
	return max(1, v.height-1)
}

func (v *MarkdownHelpViewer) scrollBy(delta int) {
	if delta == 0 {
		return
	}
	v.scroll += delta
	v.clampScroll()
}

// View renders the currently visible markdown slice.
func (v *MarkdownHelpViewer) View() tea.View {
	if v.width <= 0 || v.height <= 0 {
		return tea.NewView("")
	}
	total := len(v.rendered)
	if total == 0 {
		v.rendered = []string{""}
		total = 1
	}
	start := clampInt(v.scroll, 0, max(0, total-1))
	end := min(total, start+v.height)
	lines := v.rendered[start:end]
	buf := make([]string, v.height)
	for i := 0; i < v.height; i++ {
		if i < len(lines) {
			buf[i] = padANSI(lines[i], v.width)
		} else {
			buf[i] = strings.Repeat(" ", max(0, v.width))
		}
	}
	return tea.NewView(strings.Join(buf, "\n"))
}

// FooterHints surfaces keyboard shortcuts for modal footer.
func (v *MarkdownHelpViewer) FooterHints() []FooterHint {
	return []FooterHint{
		{Key: "↑/↓", Label: "Scroll", Enabled: true},
		{Key: "PgUp/PgDn", Label: "Page", Enabled: true},
		{Key: "Esc", Label: "Close", Enabled: true},
	}
}

func padANSI(line string, width int) string {
	if width <= 0 {
		return ""
	}
	lineWidth := lipgloss.Width(line)
	if lineWidth > width {
		line = sliceANSIColsRaw(line, 0, width)
		lineWidth = width
	}
	if lineWidth < width {
		line += strings.Repeat(" ", width-lineWidth)
	}
	return wrapWithModalColors(line)
}

func stringPtr(val string) *string {
	return &val
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func wrapWithModalColors(s string) string {
	bgSeq := ansiBackgroundSeq(uistyles.ColorModalBg)
	fgSeq := ansiForegroundSeq(uistyles.ColorModalFg)
	prefix := bgSeq + fgSeq
	var b strings.Builder
	b.WriteString(prefix)
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				j++
				seq := s[i:j]
				b.WriteString(seq)
				if seq == "\x1b[0m" {
					b.WriteString(prefix)
				}
				i = j
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

func ansiBackgroundSeq(color string) string {
	if strings.HasPrefix(color, "#") {
		if rgb, ok := parseHexColor(color); ok {
			return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", rgb[0], rgb[1], rgb[2])
		}
	}
	return fmt.Sprintf("\x1b[48;5;%sm", color)
}

func ansiForegroundSeq(color string) string {
	if strings.HasPrefix(color, "#") {
		if rgb, ok := parseHexColor(color); ok {
			return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", rgb[0], rgb[1], rgb[2])
		}
	}
	return fmt.Sprintf("\x1b[38;5;%sm", color)
}

func parseHexColor(hex string) ([3]int, bool) {
	var rgb [3]int
	if len(hex) != 7 {
		return rgb, false
	}
	for i := 0; i < 3; i++ {
		val, err := strconv.ParseInt(hex[1+i*2:3+i*2], 16, 0)
		if err != nil {
			return rgb, false
		}
		rgb[i] = int(val)
	}
	return rgb, true
}
