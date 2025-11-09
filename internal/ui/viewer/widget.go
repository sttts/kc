package viewer

import (
	"fmt"
	"strings"

	chroma "github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	textinput "github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	uistyles "github.com/sttts/kc/internal/ui/styles"
)

var searchInputStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#d7d7d7")).
	Foreground(lipgloss.Black)

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

	searchField      textinput.Model
	searchMode       bool
	searchQuery      string
	searchMatches    []int
	searchMatchIndex map[int]int
	activeMatch      int
}

// New constructs a viewer widget with an optional theme name.
func New(theme string) *Widget {
	if theme == "" {
		theme = "dracula"
	}
	ti := textinput.New()
	ti.Prompt = ""
	styles := textinput.DefaultLightStyles()
	styles.Focused.Text = searchInputStyle
	styles.Focused.Prompt = searchInputStyle
	styles.Focused.Placeholder = searchInputStyle.Copy().Foreground(lipgloss.Color("#808080"))
	styles.Blurred = styles.Focused
	styles.Cursor.Color = lipgloss.Color("#ff8c00")
	ti.Styles = styles
	ti.VirtualCursor = false
	ti.Blur()
	return &Widget{
		theme:            theme,
		searchField:      ti,
		searchMatchIndex: make(map[int]int),
		activeMatch:      -1,
	}
}

// SetContent replaces the text body and metadata, re-running syntax highlighting.
func (w *Widget) SetContent(text string, meta Metadata) {
	w.raw = text
	w.metadata = meta
	w.rawLines = strings.Split(text, "\n")
	w.highlight()
	w.offset = 0
	w.hOffset = 0
	w.clearSearchState()
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
		if w.searchMode {
			if cmd, handled := w.handleSearchInput(m); handled {
				return cmd, true
			}
		}
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
	if len(lines) == 0 {
		lines = []string{""}
	}
	body := strings.Join(lines, "\n")
	style := uistyles.PanelContentStyle.Width(max(1, frame.Width)).Height(max(1, frame.Height))
	if frame.Focused {
		style = style.Bold(true)
	}
	return style.Render(body)
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

// Position returns the one-based index of the last visible line and total lines.
func (w *Widget) Position() (current int, total int) {
	total = len(w.lines)
	if total == 0 {
		return 0, 0
	}
	end := w.offset + w.page()
	if end > total {
		end = total
	}
	if end <= 0 {
		end = 1
	}
	return end, total
}

// HasSearchMatches reports whether a committed search has any matches.
func (w *Widget) HasSearchMatches() bool {
	return len(w.searchMatches) > 0
}

// ScrollIndicators reports whether additional content exists above or below.
func (w *Widget) ScrollIndicators() (bool, bool) {
	if len(w.lines) == 0 {
		return false, false
	}
	return w.offset > 0, w.offset+w.page() < len(w.lines)
}

// SearchMode reports whether the viewer is currently editing a search query.
func (w *Widget) SearchMode() bool {
	return w.searchMode
}

// FooterStatusText describes the active search prompt or summary for footer display.
func (w *Widget) FooterStatusText(width int) string {
	if width <= 0 {
		return ""
	}
	if w.searchMode {
		w.searchField.SetWidth(width)
		view := w.searchField.View()
		return searchInputStyle.Copy().Width(width).Render(view)
	}
	summary := w.searchSummary()
	if summary == "" {
		return ""
	}
	trimmed := trimToWidth(summary, width)
	return searchInputStyle.Copy().Width(width).Render(trimmed)
}

// FooterCursor returns a real cursor relative to the footer status line.
func (w *Widget) FooterCursor(width int) *tea.Cursor {
	if !w.searchMode {
		return nil
	}
	w.searchField.SetWidth(width)
	cur := w.searchField.Cursor()
	if cur == nil {
		return nil
	}
	clone := tea.NewCursor(cur.X, cur.Y)
	clone.Blink = cur.Blink
	clone.Color = cur.Color
	clone.Shape = cur.Shape
	return clone
}

func (w *Widget) searchSummary() string {
	trimmed := strings.TrimSpace(w.searchQuery)
	if trimmed == "" {
		return ""
	}
	if len(w.searchMatches) == 0 {
		return fmt.Sprintf("/%s (no match)", trimmed)
	}
	idx := w.activeMatch + 1
	if idx <= 0 {
		idx = 1
	}
	return fmt.Sprintf("/%s (%d/%d)", trimmed, idx, len(w.searchMatches))
}

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
		rendered = highlightFallback(iterator)
	} else {
		style := styles.Get(w.theme)
		if style == nil {
			style = styles.Monokai
		}
		rendered = renderTokensWithStyle(style, iterator)
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
	case "pgdown":
		w.scrollBy(w.page())
	case "ctrl+f":
		if cmd := w.beginSearch(); cmd != nil {
			return cmd, true
		}
		return nil, true
	case "home", "g":
		w.offset = 0
	case "end", "G":
		w.offset = max(0, len(w.lines)-w.page())
	case "ctrl+a":
		w.hOffset = 0
	case "ctrl+e":
		w.alignToEnd()
	case "f7", "/":
		if cmd := w.beginSearch(); cmd != nil {
			return cmd, true
		}
		return nil, true
	case "f3":
		if w.advanceMatch() {
			return nil, true
		}
		return nil, false
	case "n":
		if w.advanceMatch() {
			return nil, true
		}
	case "N":
		if w.previousMatch() {
			return nil, true
		}
	case "f4":
		if w.onEdit != nil {
			return w.onEdit(), true
		}
		return nil, false
	case "f2":
		if w.onTheme != nil {
			return w.onTheme(), true
		}
		return nil, false
	case "f10":
		if w.onClose != nil {
			return w.onClose(), true
		}
		return nil, false
	default:
		return nil, false
	}
	return nil, true
}

func (w *Widget) handleSearchInput(key tea.KeyMsg) (tea.Cmd, bool) {
	switch key.String() {
	case "enter":
		w.commitSearch()
		return nil, true
	case "esc", "ctrl+c":
		w.cancelSearch()
		return nil, true
	default:
		var cmd tea.Cmd
		w.searchField, cmd = w.searchField.Update(key)
		return cmd, true
	}
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

func (w *Widget) beginSearch() tea.Cmd {
	w.searchMode = true
	w.searchField.SetValue(w.searchQuery)
	w.searchField.CursorEnd()
	return tea.Batch(w.searchField.Focus(), textinput.Blink)
}

func (w *Widget) commitSearch() {
	query := strings.TrimSpace(w.searchField.Value())
	w.searchMode = false
	w.searchField.Blur()
	w.searchQuery = query
	w.updateSearchMatches()
}

func (w *Widget) cancelSearch() {
	w.searchMode = false
	w.searchField.Blur()
}

func (w *Widget) clearSearchState() {
	w.searchMode = false
	w.searchField.SetValue("")
	w.searchQuery = ""
	w.searchMatches = nil
	w.activeMatch = -1
	if w.searchMatchIndex == nil {
		w.searchMatchIndex = make(map[int]int)
	}
	for k := range w.searchMatchIndex {
		delete(w.searchMatchIndex, k)
	}
}

func (w *Widget) hasSearchQuery() bool {
	return strings.TrimSpace(w.searchQuery) != ""
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

func (w *Widget) updateSearchMatches() {
	if w.searchMatchIndex == nil {
		w.searchMatchIndex = make(map[int]int)
	}
	for k := range w.searchMatchIndex {
		delete(w.searchMatchIndex, k)
	}
	if w.searchMatches != nil {
		w.searchMatches = w.searchMatches[:0]
	} else {
		w.searchMatches = make([]int, 0, 8)
	}
	w.activeMatch = -1
	query := strings.TrimSpace(w.searchQuery)
	if query == "" {
		return
	}
	lower := strings.ToLower(query)
	for idx, line := range w.rawLines {
		if strings.Contains(strings.ToLower(line), lower) {
			w.searchMatches = append(w.searchMatches, idx)
			w.searchMatchIndex[idx] = len(w.searchMatches) - 1
		}
	}
	if len(w.searchMatches) > 0 {
		w.activeMatch = 0
		w.scrollToMatch(0)
	}
}

func (w *Widget) advanceMatch() bool {
	if len(w.searchMatches) == 0 {
		return false
	}
	if w.activeMatch < 0 {
		w.activeMatch = 0
	} else {
		w.activeMatch = (w.activeMatch + 1) % len(w.searchMatches)
	}
	w.scrollToMatch(w.activeMatch)
	return true
}

func (w *Widget) previousMatch() bool {
	if len(w.searchMatches) == 0 {
		return false
	}
	if w.activeMatch < 0 {
		w.activeMatch = 0
	} else {
		w.activeMatch--
		if w.activeMatch < 0 {
			w.activeMatch = len(w.searchMatches) - 1
		}
	}
	w.scrollToMatch(w.activeMatch)
	return true
}

func (w *Widget) scrollToMatch(matchIdx int) {
	if matchIdx < 0 || matchIdx >= len(w.searchMatches) {
		return
	}
	line := w.searchMatches[matchIdx]
	page := w.page()
	target := line - page/2
	if target < 0 {
		target = 0
	}
	maxScroll := max(0, len(w.lines)-page)
	if target > maxScroll {
		target = maxScroll
	}
	w.offset = target
	if w.offset < 0 {
		w.offset = 0
	}
	w.hOffset = 0
}

func (w *Widget) matchState(line int) int {
	if w.searchMatchIndex == nil {
		return 0
	}
	if idx, ok := w.searchMatchIndex[line]; ok {
		if idx == w.activeMatch {
			return 2
		}
		return 1
	}
	return 0
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
			line := trimANSI(segment[i], w.hOffset, w.width)
			switch w.matchState(start + i) {
			case 2:
				result[i] = applySearchActiveBackground(line, w.width)
			case 1:
				result[i] = applySearchMatchBackground(line, w.width)
			default:
				result[i] = applyPanelBackground(line, w.width)
			}
		} else {
			result[i] = applyPanelBackground("", w.width)
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

func trimToWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	display := lipgloss.Width(text)
	if display <= width {
		if display < width {
			text += strings.Repeat(" ", width-display)
		}
		return text
	}
	if width <= 1 {
		return "…"
	}
	limit := width - 1
	var b strings.Builder
	current := 0
	for _, r := range text {
		rw := runeWidth(string(r))
		if current+rw > limit {
			break
		}
		b.WriteRune(r)
		current += rw
	}
	trimmed := b.String() + "…"
	padding := width - lipgloss.Width(trimmed)
	if padding > 0 {
		trimmed += strings.Repeat(" ", padding)
	}
	return trimmed
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

func applyPanelBackground(line string, width int) string {
	return applyBackground(line, width, "\033[44m")
}

func applySearchMatchBackground(line string, width int) string {
	return applyBackground(line, width, "\033[45m")
}

func applySearchActiveBackground(line string, width int) string {
	return applyBackground(line, width, "\033[43m")
}

func applyBackground(line string, width int, bg string) string {
	const reset = "\033[0m"
	if width <= 0 {
		return bg + reset
	}
	line = strings.TrimSuffix(line, reset)
	line = trimBackgroundPrefix(line)
	displayWidth := lipgloss.Width(line)
	if displayWidth < width {
		line += strings.Repeat(" ", width-displayWidth)
	}
	line = bg + line + reset
	return line
}

func trimBackgroundPrefix(line string) string {
	backgrounds := []string{"\033[44m", "\033[45m", "\033[43m"}
	for _, bg := range backgrounds {
		if strings.HasPrefix(line, bg) {
			return strings.TrimPrefix(line, bg)
		}
	}
	return line
}
