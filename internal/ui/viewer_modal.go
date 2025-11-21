package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sttts/kc/internal/ui/viewer"
)

// TextViewer wraps the shared viewer widget for modal usage.
type TextViewer struct {
	inner     *viewer.Widget
	width     int
	height    int
	onEdit    func() tea.Cmd
	onOptions func() tea.Cmd
	onClose   func() tea.Cmd
}

func newViewer(theme string, onEdit, onOptions, onClose func() tea.Cmd) *TextViewer {
	w := viewer.New(theme)
	w.SetCallbacks(onEdit, onOptions, onClose)
	return &TextViewer{inner: w, onEdit: onEdit, onOptions: onOptions, onClose: onClose}
}

// NewYAMLViewer preserves backwards compatibility for callers expecting YAML defaults.
func NewYAMLViewer(title, text, theme string, onEdit func() tea.Cmd, onOptions func() tea.Cmd, onClose func() tea.Cmd) *TextViewer {
	return NewTextViewer(title, text, "yaml", "application/yaml", title, theme, onEdit, onOptions, onClose)
}

// NewTextViewer creates a syntax-highlighted viewer for arbitrary text.
func NewTextViewer(title, text, lang, mime, filename, theme string, onEdit func() tea.Cmd, onOptions func() tea.Cmd, onClose func() tea.Cmd) *TextViewer {
	tv := newViewer(theme, onEdit, onOptions, onClose)
	tv.inner.SetContent(text, viewer.Metadata{
		Title:    title,
		Language: lang,
		MimeType: mime,
		Filename: filename,
	})
	return tv
}

func (v *TextViewer) Init() tea.Cmd { return nil }

func (v *TextViewer) SetDimensions(w, h int) { v.width, v.height = w, h }

func (v *TextViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if cmd, handled := v.inner.Update(msg); handled {
		return v, cmd
	}
	return v, nil
}

func (v *TextViewer) View() tea.View {
	if v.height <= 0 || v.width <= 0 {
		return tea.NewView("")
	}
	return tea.NewView(v.inner.View(viewer.Frame{Width: v.width, Height: v.height, Focused: true}))
}

func (v *TextViewer) FooterHints() []FooterHint {
	if v.inner.SearchMode() {
		return []FooterHint{
			{Key: "Enter", Label: "Search", Enabled: true},
			{Key: "Esc", Label: "Cancel", Enabled: true},
		}
	}
	return []FooterHint{
		{Key: "F2", Label: "Options", Enabled: v.onOptions != nil},
		{Key: "F3", Label: "Next", Enabled: v.inner.HasSearchMatches()},
		{Key: "F4", Label: "Edit", Enabled: v.onEdit != nil},
		{Key: "F7", Label: "Search", Enabled: true},
		{Key: "F10", Label: "Close", Enabled: v.onClose != nil},
	}
}

func (v *TextViewer) SetTheme(theme string) {
	v.inner.SetTheme(theme)
}

func (v *TextViewer) Theme() string { return v.inner.Theme() }

func (v *TextViewer) SetWrapMode(on bool) { v.inner.SetWrapMode(on) }

func (v *TextViewer) WrapMode() bool { return v.inner.WrapMode() }

func (v *TextViewer) SetOnOptions(fn func() tea.Cmd) {
	v.onOptions = fn
	v.inner.SetCallbacks(v.onEdit, v.onOptions, v.onClose)
}

func (v *TextViewer) SetOnClose(fn func() tea.Cmd) {
	v.onClose = fn
	v.inner.SetCallbacks(v.onEdit, v.onOptions, v.onClose)
}

func (v *TextViewer) RequestTheme() tea.Cmd {
	if v.onOptions != nil {
		return v.onOptions()
	}
	return nil
}

func (v *TextViewer) SetContent(text string, meta viewer.Metadata) {
	v.inner.SetContent(text, meta)
}

func (v *TextViewer) SetThemeAndContent(theme, text string, meta viewer.Metadata) {
	v.SetTheme(theme)
	v.inner.SetContent(text, meta)
}

func (v *TextViewer) FooterStatus(width int) string {
	return v.inner.FooterStatusText(width)
}

func (v *TextViewer) FooterCursor(width int) *tea.Cursor {
	return v.inner.FooterCursor(width)
}

func (v *TextViewer) HandleModalEscape(key tea.KeyMsg) (bool, tea.Cmd) {
	if key.String() != "esc" {
		return false, nil
	}
	if v.inner == nil {
		return false, nil
	}
	if cmd, handled := v.inner.HandleEscape(); handled {
		return true, cmd
	}
	return false, nil
}
