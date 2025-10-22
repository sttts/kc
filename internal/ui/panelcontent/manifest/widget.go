package manifest

import (
	"context"
	"fmt"
	"math"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	models "github.com/sttts/kc/internal/models"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
	"github.com/sttts/kc/internal/ui/viewer"
)

// Widget renders resource manifests using the shared viewer widget.
type Widget struct {
	deps   panelcontent.WidgetDeps
	viewer *viewer.Widget
	sel    panelcontent.Selection
}

// New constructs a manifest widget backed by the generic viewer.
func New(deps panelcontent.WidgetDeps) *Widget {
	w := viewer.New("dracula")
	return &Widget{
		deps:   deps,
		viewer: w,
	}
}

func (w *Widget) Init(context.Context) tea.Cmd { return nil }

func (w *Widget) Update(ctx context.Context, msg tea.Msg) (tea.Cmd, bool) {
	return w.viewersUpdate(msg)
}

func (w *Widget) viewersUpdate(msg tea.Msg) (tea.Cmd, bool) {
	if cmd, handled := w.viewer.Update(msg); handled {
		return cmd, true
	}
	return nil, false
}

func (w *Widget) View(ctx context.Context, frame panelcontent.Frame) string {
	return w.viewer.View(viewer.Frame{
		Width:   frame.Size.Width,
		Height:  frame.Size.Height,
		Focused: frame.Focused,
	})
}

func (w *Widget) Resize(ctx context.Context, size panelcontent.Size) {
	w.viewer.Resize(size.Width, size.Height)
}

func (w *Widget) SetFocus(context.Context, bool) {}

func (w *Widget) Teardown(context.Context) {}

func (w *Widget) Footer(ctx context.Context, width int) string {
	return w.viewer.Footer(width)
}

func (w *Widget) FrameInfo(ctx context.Context, req panelcontent.FrameInfoRequest) panelcontent.FrameInfo {
	info := panelcontent.FrameInfo{SuppressFooter: true, TopIndicator: "─", BottomIndicator: "─"}
	breadcrumb := ""
	if w.sel.Item != nil {
		if path := w.sel.Item.Path(); len(path) > 0 {
			breadcrumb = "/" + strings.Join(path, "/")
		}
		if breadcrumb == "" {
			breadcrumb = w.sel.Path
		}
	}
	if breadcrumb == "" && w.deps.Path != nil {
		breadcrumb = w.deps.Path()
	}
	if breadcrumb != "" {
		info.Breadcrumb = breadcrumb
	}
	current, total := w.viewer.Position()
	if total > 0 {
		above, below := w.viewer.ScrollIndicators()
		if above {
			info.TopIndicator = "^"
		}
		if below {
			info.BottomIndicator = "v"
		}
		percent := int(math.Round(float64(current) * 100 / float64(total)))
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}
		info.FooterStatus = fmt.Sprintf("%d/%d • %d%%", current, total, percent)
	}
	return info
}

// OnSelectionChanged refreshes the manifest for the provided selection.
func (w *Widget) OnSelectionChanged(ctx context.Context, sel panelcontent.Selection) {
	w.sel = sel
	w.refresh(ctx)
}

func (w *Widget) refresh(ctx context.Context) {
	item := w.sel.Item
	if item == nil && w.deps.SelectedItem != nil {
		if i, ok := w.deps.SelectedItem(ctx); ok {
			item = i
		}
	}
	if item == nil {
		w.viewer.SetContent("Select a resource to view its manifest.", viewer.Metadata{Title: "Manifest"})
		return
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
		w.viewer.SetContent("Selected resource does not expose manifest content.", viewer.Metadata{Title: "Manifest"})
		return
	}
	title, body, lang, mime, filename, err := viewable.ViewContent()
	if err != nil {
		w.viewer.SetContent("Failed to load manifest: "+err.Error(), viewer.Metadata{Title: "Manifest"})
		return
	}
	w.viewer.SetContent(body, viewer.Metadata{
		Title:    title,
		Language: lang,
		MimeType: mime,
		Filename: filename,
	})
}
