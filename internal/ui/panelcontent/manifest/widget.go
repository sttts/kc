package manifest

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
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
	if m, ok := msg.(tea.WindowSizeMsg); ok {
		width := m.Width
		height := m.Height
		if width < 1 {
			width = 1
		}
		if height < 1 {
			height = 1
		}
		w.viewer.Resize(width, height)
		return nil, true
	}
	return w.viewersUpdate(msg)
}

func (w *Widget) viewersUpdate(msg tea.Msg) (tea.Cmd, bool) {
	if cmd, handled := w.viewer.Update(msg); handled {
		return cmd, true
	}
	return nil, false
}

func (w *Widget) View(ctx context.Context, frame panelcontent.Frame) tea.View {
	return tea.NewView(w.viewer.View(viewer.Frame{
		Width:   frame.Size.Width,
		Height:  frame.Size.Height,
		Focused: frame.Focused,
	}))
}

func (w *Widget) SetFocus(context.Context, bool) {}

func (w *Widget) Teardown(context.Context) {}

func (w *Widget) Footer(ctx context.Context, width int) string {
	if status := strings.TrimSpace(w.viewer.FooterStatusText(width)); status != "" {
		return status
	}
	return w.apiPathForSelection()
}

func (w *Widget) FrameInfo(ctx context.Context, req panelcontent.FrameInfoRequest) panelcontent.FrameInfo {
	info := panelcontent.FrameInfo{TopIndicator: "─", BottomIndicator: "─"}
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
	above, below := w.viewer.ScrollIndicators()
	if above {
		info.TopIndicator = "^"
	}
	if below {
		info.BottomIndicator = "v"
	}
	api := w.apiPathForSelection()
	if api != "" {
		current, total := w.viewer.Position()
		if total > 0 {
			info.FooterStatus = fmt.Sprintf("%d/%d", current, total)
		}
		info.SuppressFooter = false
	} else {
		info.SuppressFooter = true
	}
	return info
}

func (w *Widget) apiPathForSelection() string {
	if obj, ok := w.sel.Item.(models.ObjectItem); ok && obj != nil {
		return apiPath(obj.GVR(), obj.Namespace(), obj.Name())
	}
	return ""
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
