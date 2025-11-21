package describe

import (
	"context"
	"fmt"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	models "github.com/sttts/kc/internal/models"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
	"github.com/sttts/kc/internal/ui/viewer"
	describe "github.com/sttts/kc/pkg/describe"
)

// Widget renders kubectl describe output for the selected object.
type Widget struct {
	deps   panelcontent.WidgetDeps
	viewer *viewer.Widget
	sel    panelcontent.Selection
}

// New constructs a describe widget backed by the shared viewer.
func New(deps panelcontent.WidgetDeps) *Widget {
	return &Widget{
		deps:   deps,
		viewer: viewer.New("dracula"),
	}
}

func (w *Widget) Init(context.Context) tea.Cmd { return nil }

func (w *Widget) Update(_ context.Context, msg tea.Msg) (tea.Cmd, bool) {
	return w.viewer.Update(msg)
}

func (w *Widget) View(_ context.Context, frame panelcontent.Frame) string {
	return w.viewer.View(viewer.Frame{
		Width:   frame.Size.Width,
		Height:  frame.Size.Height,
		Focused: frame.Focused,
	})
}

func (w *Widget) Resize(_ context.Context, size panelcontent.Size) {
	w.viewer.Resize(size.Width, size.Height)
}

func (w *Widget) SetFocus(context.Context, bool) {}

func (w *Widget) Teardown(context.Context) {}

func (w *Widget) Footer(_ context.Context, width int) string {
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

// OnSelectionChanged refreshes the describe output when the selection changes.
func (w *Widget) OnSelectionChanged(ctx context.Context, sel panelcontent.Selection) {
	w.sel = sel
	w.refresh(ctx)
}

func (w *Widget) refresh(ctx context.Context) {
	target, ok := w.selectionTarget(ctx)
	if !ok {
		w.viewer.SetContent("Select a resource to describe.", viewer.Metadata{Title: "Describe"})
		return
	}
	if w.deps.Describe == nil {
		w.viewer.SetContent("Describe provider unavailable.", viewer.Metadata{Title: describeTitle(target, "")})
		return
	}
	result, err := w.deps.Describe(ctx, target)
	title := describeTitle(target, result.Title)
	if err != nil {
		w.viewer.SetContent(fmt.Sprintf("Describe failed: %v", err), viewer.Metadata{Title: title})
		return
	}
	body := result.Body
	if strings.TrimSpace(body) == "" {
		body = "No describe output available."
	}
	w.viewer.SetContent(body, viewer.Metadata{
		Title:    title,
		MimeType: "text/plain",
	})
}

func (w *Widget) selectionTarget(ctx context.Context) (describe.Target, bool) {
	item := w.sel.Item
	if item == nil && w.deps.SelectedItem != nil {
		if selected, ok := w.deps.SelectedItem(ctx); ok {
			item = selected
		}
	}
	obj, ok := item.(models.ObjectItem)
	if !ok || obj == nil {
		return describe.Target{}, false
	}
	gvr := obj.GVR()
	if gvr.Resource == "" {
		return describe.Target{}, false
	}
	return describe.Target{
		GVR:       gvr,
		Namespace: obj.Namespace(),
		Name:      obj.Name(),
	}, true
}

func describeTitle(target describe.Target, override string) string {
	if override != "" {
		return override
	}
	name := target.Name
	if name == "" {
		name = target.GVR.Resource
	}
	if target.Namespace != "" {
		name = fmt.Sprintf("%s/%s", target.Namespace, name)
	}
	return fmt.Sprintf("Describe %s", name)
}
