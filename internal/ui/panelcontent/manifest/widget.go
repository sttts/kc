package manifest

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	models "github.com/sttts/kc/internal/models"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
	uistyles "github.com/sttts/kc/internal/ui/styles"
)

// Widget renders resource manifests (YAML/Text) based on the selected item.
type Widget struct {
	deps panelcontent.WidgetDeps

	width  int
	height int

	title   string
	lines   []string
	message string
	scroll  int
	last    panelcontent.Selection
}

// New constructs a manifest widget.
func New(deps panelcontent.WidgetDeps) *Widget {
	return &Widget{
		deps:  deps,
		lines: []string{"Select a resource to view its manifest."},
	}
}

func (w *Widget) Init(ctx context.Context) tea.Cmd {
	return w.refresh(ctx)
}

func (w *Widget) Update(ctx context.Context, msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		if w.handleKey(m.String()) {
			return nil, true
		}
	case panelcontent.MouseMsg:
		if m.Intent == panelcontent.MouseIntentWheel {
			w.scrollBy(m.DeltaY)
			return nil, true
		}
	}
	return nil, false
}

func (w *Widget) View(ctx context.Context, frame panelcontent.Frame) string {
	w.width = frame.Size.Width
	w.height = frame.Size.Height
	if w.width <= 0 {
		w.width = 1
	}
	if w.height <= 0 {
		w.height = 1
	}
	lines := w.visibleLines()
	view := strings.Join(lines, "\n")
	style := uistyles.PanelContentStyle.Width(w.width).Height(w.height)
	if frame.Focused {
		style = style.Bold(true)
	}
	return style.Render(view)
}

func (w *Widget) Resize(ctx context.Context, size panelcontent.Size) {
	w.width = size.Width
	w.height = size.Height
}

func (w *Widget) SetFocus(context.Context, bool) {}

func (w *Widget) Teardown(context.Context) {}

func (w *Widget) Footer(ctx context.Context, width int) string {
	info := w.title
	if info == "" {
		info = "Manifest"
	}
	return uistyles.PanelFooterStyle.Width(width).Render(info)
}

// OnSelectionChanged refreshes content when the host reports a new selection.
func (w *Widget) OnSelectionChanged(ctx context.Context, sel panelcontent.Selection) {
	w.last = sel
	_ = w.refresh(ctx)
}

func (w *Widget) refresh(ctx context.Context) tea.Cmd {
	sel := w.last
	if sel.Item == nil && w.deps.SelectedItem != nil {
		if item, ok := w.deps.SelectedItem(ctx); ok {
			sel.Item = item
		}
	}
	item := sel.Item
	if item == nil {
		w.message = "Select a resource to view its manifest."
		w.lines = []string{w.message}
		w.title = ""
		return nil
	}
	modelItem, _ := item.(models.Item)
	var viewable models.Viewable
	if v, ok := item.(models.Viewable); ok {
		viewable = v
	} else if modelItem != nil {
		if v, ok := modelItem.(models.Viewable); ok {
			viewable = v
		}
	}
	if viewable == nil {
		w.message = "Selected resource does not expose manifest content."
		w.lines = []string{w.message}
		w.title = ""
		return nil
	}
	title, body, _, _, _, err := viewable.ViewContent()
	if err != nil {
		w.message = "Failed to load manifest: " + err.Error()
		w.lines = []string{w.message}
		w.title = ""
		return nil
	}
	if body == "" {
		w.message = "Manifest is empty."
		w.lines = []string{w.message}
		w.title = title
		return nil
	}
	w.title = title
	w.lines = strings.Split(body, "\n")
	w.message = ""
	w.scroll = 0
	return nil
}

func (w *Widget) handleKey(key string) bool {
	switch key {
	case "up", "k":
		w.scrollBy(-1)
	case "down", "j":
		w.scrollBy(1)
	case "pgup":
		w.scrollBy(-w.page())
	case "pgdown":
		w.scrollBy(w.page())
	case "home", "g":
		w.scroll = 0
	case "end", "G":
		w.scroll = max(0, len(w.lines)-w.page())
	default:
		return false
	}
	return true
}

func (w *Widget) scrollBy(delta int) {
	if len(w.lines) == 0 {
		w.scroll = 0
		return
	}
	w.scroll += delta
	if w.scroll < 0 {
		w.scroll = 0
	}
	maxScroll := max(0, len(w.lines)-w.page())
	if w.scroll > maxScroll {
		w.scroll = maxScroll
	}
}

func (w *Widget) page() int {
	if w.height <= 0 {
		return 1
	}
	return w.height
}

func (w *Widget) visibleLines() []string {
	if len(w.lines) == 0 {
		return []string{w.message}
	}
	h := w.page()
	if h <= 0 {
		h = 1
	}
	start := w.scroll
	if start < 0 {
		start = 0
	}
	end := start + h
	if end > len(w.lines) {
		end = len(w.lines)
	}
	segment := w.lines[start:end]
	padded := make([]string, h)
	copy(padded, segment)
	for i := len(segment); i < h; i++ {
		padded[i] = ""
	}
	style := lipgloss.NewStyle().Width(max(1, w.width))
	for i := range padded {
		padded[i] = style.Render(padded[i])
	}
	return padded
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
