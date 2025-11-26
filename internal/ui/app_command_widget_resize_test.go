package ui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
)

type recordingWidget struct {
	lastSize    panelcontent.Size
	resizeCount int
}

func (w *recordingWidget) Init(context.Context) tea.Cmd                    { return nil }
func (w *recordingWidget) Update(context.Context, tea.Msg) (tea.Cmd, bool) { return nil, false }
func (w *recordingWidget) View(context.Context, panelcontent.Frame) string { return "" }
func (w *recordingWidget) Resize(_ context.Context, size panelcontent.Size) {
	w.resizeCount++
	w.lastSize = size
}
func (w *recordingWidget) SetFocus(context.Context, bool) {}
func (w *recordingWidget) Teardown(context.Context)       {}

func TestCommandWidgetResizedWhenPanelWidthChangesViaViewOptions(t *testing.T) {
	app := NewApp()
	app.width = 120
	app.height = 30

	leftWidget := &recordingWidget{}
	rightWidget := &recordingWidget{}
	app.leftPanel.RegisterMode(PanelModeCommand, func(*Panel) panelcontent.Widget { return leftWidget })
	app.rightPanel.RegisterMode(PanelModeCommand, func(*Panel) panelcontent.Widget { return rightWidget })

	ctx := t.Context()
	app.leftPanel.SetMode(ctx, PanelModeCommand)
	app.rightPanel.SetMode(ctx, PanelModeCommand)

	app.renderMainView()
	initialSize := leftWidget.lastSize

	app.showModal("view_options")
	app.Update(ViewOptionsCommittedMsg{
		PanelIndex:        0,
		SetPanelWidth:     true,
		PanelWidthPercent: 75,
		Accept:            true,
		Close:             true,
	})
	// Re-render to apply new frame sizes to the widget.
	app.renderMainView()

	updatedSize := leftWidget.lastSize
	if updatedSize.Width <= initialSize.Width {
		t.Fatalf("expected command widget width to increase after resize, got %d -> %d", initialSize.Width, updatedSize.Width)
	}
	leftWidth, _, panelHeight, _ := app.panelAreaMetrics()
	expected, _ := app.leftPanel.FrameContentSize(ctx, leftWidth, panelHeight)
	if updatedSize != expected {
		t.Fatalf("expected command widget size %+v after resize, got %+v", expected, updatedSize)
	}
}
