package ui

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	chroma "github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	tea "github.com/charmbracelet/bubbletea/v2"
)

// YAMLViewer is a simple scrollable text viewer for YAML content.
// Note: Syntax highlighting to be integrated with a library (e.g., chroma) in a follow-up.
type YAMLViewer struct {
	title    string
	content  []string // highlighted, ANSI-colored lines
	raw      string   // original, uncolored content for re-highlight on resize/theme
	width    int
	height   int
	offset   int // vertical scroll (top line index)
	hOffset  int // horizontal scroll (left column index)
    theme    string
    // Syntax detection hints
    lang     string // e.g., "yaml", "json", "go" (lexer name)
    mime     string // e.g., "application/yaml"
    filename string // e.g., "file.yaml"
	onEdit   func() tea.Cmd // invoked on F4
	onTheme  func() tea.Cmd // invoked on F9 to open theme selector
	onClose  func() tea.Cmd // invoked on F10 to close modal
	rawLines []string       // raw, uncolored lines for measuring widths
}

func NewYAMLViewer(title, text, theme string, onEdit func() tea.Cmd, onTheme func() tea.Cmd, onClose func() tea.Cmd) *YAMLViewer {
    v := &YAMLViewer{title: title, raw: text, theme: theme, onEdit: onEdit, onTheme: onTheme, onClose: onClose}
    v.rawLines = strings.Split(text, "\n")
    v.content = v.highlightWithTheme(text, theme)
    return v
}

// NewTextViewer creates a syntax-highlighted viewer for arbitrary text.
// Provide at least one of lang/mime/filename for best detection; otherwise a fallback will be used.
func NewTextViewer(title, text, lang, mime, filename, theme string, onEdit func() tea.Cmd, onTheme func() tea.Cmd, onClose func() tea.Cmd) *YAMLViewer {
    v := &YAMLViewer{title: title, raw: text, theme: theme, lang: lang, mime: mime, filename: filename, onEdit: onEdit, onTheme: onTheme, onClose: onClose}
    v.rawLines = strings.Split(text, "\n")
    v.content = v.highlightWithTheme(text, theme)
    return v
}

func (v *YAMLViewer) Init() tea.Cmd { return nil }

func (v *YAMLViewer) SetDimensions(w, h int) { v.width, v.height = w, h }

func (v *YAMLViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch m.String() {
		case "up":
			if v.offset > 0 {
				v.offset--
			}
		case "down":
			if v.offset < max(0, len(v.content)-v.height) {
				v.offset++
			}
		case "left":
			if v.hOffset > 0 {
				v.hOffset--
			}
		case "right":
			// Optimistically increase; View clamps effectively by slicing
			v.hOffset++
		case "pgup":
			v.offset = max(0, v.offset-(v.height-1))
		case "pgdown":
			v.offset = min(max(0, len(v.content)-v.height), v.offset+(v.height-1))
		case "home":
			v.offset = 0
		case "end":
			v.offset = max(0, len(v.content)-v.height)
		case "ctrl+a":
			v.hOffset = 0
		case "ctrl+e":
			// Move to the horizontal end for current viewport (based on raw widths)
			start := v.offset
			end := min(len(v.rawLines), v.offset+v.height)
			maxLen := 0
			for i := start; i < end; i++ {
				if l := runeWidth(v.rawLines[i]); l > maxLen {
					maxLen = l
				}
			}
			if maxLen > v.width {
				v.hOffset = maxLen - v.width
			} else {
				v.hOffset = 0
			}
		case "f4":
			if v.onEdit != nil {
				return v, v.onEdit()
			}
		case "f9":
			if v.onTheme != nil {
				return v, v.onTheme()
			}
		case "f10":
			if v.onClose != nil {
				return v, v.onClose()
			}
		}
	}
	return v, nil
}

func (v *YAMLViewer) View() string {
	if v.height <= 0 || v.width <= 0 {
		return ""
	}
	// If content somehow empty (e.g., style failed), use raw
	if len(v.content) == 0 {
		v.content = strings.Split(v.raw, "\n")
	}
	end := min(len(v.content), v.offset+v.height)
	lines := v.content[v.offset:end]
	// Apply horizontal slicing without wrapping
	sliced := make([]string, len(lines))
	for i, ln := range lines {
		sliced[i] = sliceANSIByColumns(ln, v.hOffset, v.width)
	}
	return PanelContentStyle.Width(v.width).Height(v.height).Render(strings.Join(sliced, "\n"))
}

// FooterHints implements ModalFooterHints to show extra footer actions.
func (v *YAMLViewer) FooterHints() [][2]string {
	return [][2]string{{"F9", "Theme"}, {"F10", "Close"}}
}

// SetTheme updates the theme and re-highlights content.
func (v *YAMLViewer) SetTheme(theme string) {
	if theme == "" {
		return
	}
	v.theme = theme
	v.content = v.highlightWithTheme(v.raw, v.theme)
}

// SetOnTheme sets the callback invoked when user requests theme selection.
func (v *YAMLViewer) SetOnTheme(fn func() tea.Cmd) { v.onTheme = fn }

// SetOnClose sets the callback used to close the surrounding modal.
func (v *YAMLViewer) SetOnClose(fn func() tea.Cmd) { v.onClose = fn }

// RequestTheme allows external callers (e.g., modal ESC-number mapping)
// to trigger the theme selector without synthesizing a key event.
func (v *YAMLViewer) RequestTheme() tea.Cmd {
	if v.onTheme != nil {
		return v.onTheme()
	}
	return nil
}

// highlightWithTheme converts YAML to ANSI-colored lines using chroma with no background so it
// blends with the panel theme. On failure it returns the plain, uncolored lines.
func (v *YAMLViewer) highlightWithTheme(text, theme string) []string {
    // Pick lexer using hints: lang > mime > filename > analyse > fallback
    var lexer chroma.Lexer
    if v.lang != "" { lexer = lexers.Get(v.lang) }
    if lexer == nil && v.mime != "" { lexer = lexers.MatchMimeType(v.mime) }
    if lexer == nil && v.filename != "" { lexer = lexers.Match(v.filename) }
    if lexer == nil { lexer = lexers.Analyse(text) }
    if lexer == nil { lexer = lexers.Fallback }
	iterator, err := lexer.Tokenise(nil, text)
	if err != nil {
		return strings.Split(text, "\n")
	}
	if theme == "turbo-pascal" {
		out := formatTurboPascalANSI(iterator)
		return strings.Split(out, "\n")
	}
	// Ensure custom styles are registered for theme names.
	registerCustomStylesOnce()
	st := styles.Get(theme)
	if st == nil {
		st = styles.Fallback
	}
	// Render with truecolor foregrounds as defined by Chroma; only background is enforced.
	out := formatTTY16mWithPanelBG(st, iterator)
	return strings.Split(out, "\n")
}

// formatTurboPascalANSI renders with ANSI 16 colors to mimic Turbo Pascal IDE
// colors on a blue background and adds YAML-aware key/value heuristics.
func formatTurboPascalANSI(it chroma.Iterator) string {
	var buf bytes.Buffer
	buf.WriteString("\033[44m") // blue background
	prevWasColon := false
	atLineStart := true
	for token := it(); token != chroma.EOF; token = it() {
		t := token.Type
		ansi := "37" // light gray
		bold := false
		val := token.Value
		hasNL := strings.Contains(val, "\n")

		// Colon handling
		if t == chroma.Punctuation && strings.Contains(val, ":") {
			buf.WriteString("\033[1m\033[35m") // magenta
			buf.WriteString(val)
			buf.WriteString("\033[39m\033[22m")
			prevWasColon = true
			if hasNL {
				prevWasColon, atLineStart = false, true
			} else {
				atLineStart = false
			}
			continue
		}

		// Track whether we force a color (to avoid being overridden by token mapping)
		forced := false

		// Value after colon becomes bold yellow
		if prevWasColon {
			if strings.TrimSpace(val) != "" && t != chroma.Punctuation {
				ansi, bold = "33", true
				prevWasColon = false
				forced = true
			}
		}

		// Keys: at start of a line until ':'
		if !forced && atLineStart && !prevWasColon && strings.TrimSpace(val) != "" {
			if !(t == chroma.Punctuation && strings.TrimSpace(val) != "-") {
				ansi, bold = "36", true // cyan bold
				forced = true
			}
		}

		switch {
		case t == chroma.NameTag || t.Category() == chroma.Name:
			ansi, bold = "36", true // cyan keys/tags
		case !forced && (t == chroma.LiteralString || t.Category() == chroma.LiteralString):
			ansi, bold = "33", true // yellow strings
		case !forced && (t == chroma.LiteralNumber || t.Category() == chroma.LiteralNumber):
			ansi, bold = "32", true // green numbers
		case !forced && (t == chroma.Punctuation || t == chroma.Operator):
			ansi, bold = "35", true // magenta punctuation
		case !forced && (t == chroma.Comment || t.Category() == chroma.Comment):
			ansi = "34" // blue comments
		default:
			ansi = "37"
		}

		if bold {
			buf.WriteString("\033[1m")
		}
		buf.WriteString("\033[" + ansi + "m")
		buf.WriteString(val)
		buf.WriteString("\033[39m\033[22m")
		if hasNL {
			prevWasColon = false
			atLineStart = true
		} else {
			atLineStart = false
		}
	}
	buf.WriteString("\033[0m")
	return buf.String()
}

// formatTurboPascalANSI renders with ANSI 16 colors to mimic Turbo Pascal IDE
// colors on a blue background.
// duplicate function removed; see YAML-aware version below

// --- Custom Styles Registration ---
var customStylesRegistered = false

func registerCustomStylesOnce() {
	if customStylesRegistered {
		return
	}
	// Register Turbo Pascal inspired style (foreground-only truecolor definitions);
	// we still render with a panel blue background and do not set backgrounds here.
	tp := chroma.MustNewStyle("turbo-pascal", chroma.StyleEntries{
		chroma.Background:       "",
		chroma.Text:             "#d7d7d7",      // light gray text
		chroma.Comment:          "#00afff",      // bright cyan-ish for comments
		chroma.Keyword:          "bold #00afff", // bright blue keywords
		chroma.Name:             "#d7d7d7",
		chroma.NameAttribute:    "#d7d7d7",
		chroma.NameTag:          "#d7d7d7",
		chroma.LiteralString:    "#ffd75f", // yellow strings
		chroma.LiteralStringDoc: "#ffd75f",
		chroma.LiteralNumber:    "#5fff5f", // green numbers
		chroma.Operator:         "#ffffff", // white operators
		chroma.Punctuation:      "#ffffff", // white punctuation
		chroma.Error:            "#ff5555",
	})
	styles.Register(tp)
	customStylesRegistered = true
}

// kcChromaStyle returns a style with foreground-only colors optimized for
// a dark blue background. No background colors are set to allow the panel
// theme to show through.
// Note: We intentionally avoid defining a bespoke style; we wrap the
// predefined Dracula style and clear its background via the style builder.

// formatTTY16mWithPanelBG renders tokens with true-color foregrounds while
// keeping a persistent background equal to the panel's dark blue.
// It resets only foreground/bold/italic/underline (not background) between tokens.
func formatTTY16mWithPanelBG(style *chroma.Style, it chroma.Iterator) string {
	var buf bytes.Buffer
	// Emit persistent ANSI dark blue background once (44)
	buf.WriteString("\033[44m")
	for token := it(); token != chroma.EOF; token = it() {
		entry := style.Get(token.Type)
		// Apply foreground-related attributes
		if entry.Bold == chroma.Yes {
			buf.WriteString("\033[1m")
		}
		if entry.Underline == chroma.Yes {
			buf.WriteString("\033[4m")
		}
		if entry.Italic == chroma.Yes {
			buf.WriteString("\033[3m")
		}
		if entry.Colour.IsSet() {
			fmt.Fprintf(&buf, "\033[38;2;%d;%d;%dm", entry.Colour.Red(), entry.Colour.Green(), entry.Colour.Blue())
		}
		// Write token value
		buf.WriteString(token.Value)
		// Reset only foreground and attributes; keep background
		buf.WriteString("\033[39m\033[22m\033[24m\033[23m")
	}
	// Reset everything at the end to avoid leaking styles
	buf.WriteString("\033[0m")
	return buf.String()
}

// runeWidth returns the number of runes (columns) in a string. This treats
// each rune as width 1, which is sufficient for ASCII YAML.
func runeWidth(s string) int { return len([]rune(s)) }

// sliceANSIByColumns returns a substring by visible columns, ignoring ANSI
// escape sequences for counting. It preserves escape sequences encountered
// within the slice and terminates with a reset.
func sliceANSIByColumns(s string, start, width int) string {
	if start < 0 {
		start = 0
	}
	if width <= 0 {
		return ""
	}
	var b bytes.Buffer
	col := 0
	collecting := false
	for i := 0; i < len(s); {
		if s[i] == 0x1b { // ESC
			// Copy full SGR sequence without affecting column count
			// Expect CSI ... 'm'
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && s[j] != 'm' {
					j++
				}
				if j < len(s) {
					j++
				} // include 'm'
			}
			if collecting {
				b.WriteString(s[i:j])
			}
			i = j
			continue
		}
		r, sz := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && sz == 1 {
			sz = 1
		}
		if col >= start && col < start+width {
			b.WriteString(s[i : i+sz])
			collecting = true
		}
		col++
		if col >= start+width {
			break
		}
		i += sz
	}
	// Reset styles at the end to avoid leaking
	b.WriteString("\033[0m")
	return b.String()
}
