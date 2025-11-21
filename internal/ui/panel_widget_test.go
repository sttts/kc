package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea/v2"
	models "github.com/sttts/kc/internal/models"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
)

func TestPanelModeSwitchesToManifest(t *testing.T) {
	panel := NewPanel("test")
	ctx := context.Background()
	panel.SetDimensions(ctx, 20, 5)
	panel.SetMode(ctx, PanelModeManifest)
	if panel.Mode() != PanelModeManifest {
		t.Fatalf("expected manifest mode, got %v", panel.Mode())
	}
	view := panel.View()
	if !strings.Contains(view, "Select a resource") {
		t.Fatalf("expected manifest placeholder, got %q", view)
	}
	widget := panel.ensureActiveWidget(ctx)
	provider, ok := widget.(panelcontent.FrameInfoProvider)
	if !ok {
		t.Fatalf("manifest widget does not implement FrameInfoProvider")
	}
	info := provider.FrameInfo(ctx, panelcontent.FrameInfoRequest{Width: panel.width})
	if !info.SuppressFooter {
		t.Fatalf("expected manifest to suppress footer")
	}
	if info.FooterStatus != "" {
		t.Fatalf("expected manifest footer status to be empty without a selection, got %q", info.FooterStatus)
	}
}

func TestNextPanelModeCycles(t *testing.T) {
	modes := PanelModeOrder()
	for i := 0; i < len(modes); i++ {
		next := NextPanelMode(modes[i])
		if next == modes[i] {
			t.Fatalf("mode did not advance for %v", modes[i])
		}
	}
}

func TestPanelSelectionChangedMessage(t *testing.T) {
	ctx := context.Background()
	panel := NewPanel("test")
	panel.listWidget(ctx) // ensure list widget initialized
	cmd := panel.widgetSelectionChanged(ctx, panelcontent.Selection{ID: "item-a", Path: "/"})
	if cmd == nil {
		t.Fatalf("expected selection change command")
	}
	if msg := cmd(); msg == nil {
		t.Fatalf("expected selection change message")
	} else if selMsg, ok := msg.(PanelSelectionChangedMsg); !ok {
		t.Fatalf("expected PanelSelectionChangedMsg, got %#v", msg)
	} else if selMsg.Selection.ID != "item-a" {
		t.Fatalf("expected selection ID item-a, got %s", selMsg.Selection.ID)
	}
}

type selectionSpyWidget struct {
	panelcontent.Widget
	events []panelcontent.Selection
}

func (s *selectionSpyWidget) Init(context.Context) tea.Cmd                    { return nil }
func (s *selectionSpyWidget) Update(context.Context, tea.Msg) (tea.Cmd, bool) { return nil, false }
func (s *selectionSpyWidget) View(context.Context, panelcontent.Frame) string { return "" }
func (s *selectionSpyWidget) Resize(context.Context, panelcontent.Size)       {}
func (s *selectionSpyWidget) SetFocus(context.Context, bool)                  {}
func (s *selectionSpyWidget) Teardown(context.Context)                        {}
func (s *selectionSpyWidget) OnSelectionChanged(_ context.Context, sel panelcontent.Selection) {
	s.events = append(s.events, sel)
}

func TestPanelDescribeModeReplaysSelection(t *testing.T) {
	ctx := context.Background()
	panel := NewPanel("test")
	panel.SetDimensions(ctx, 20, 5)
	spy := &selectionSpyWidget{}
	panel.RegisterMode(PanelModeDescribe, func(*Panel) PanelWidget { return spy })
	item := models.NewSimpleItem("foo", []string{"/foo"}, nil, models.WhiteStyle())
	panel.SetMode(ctx, PanelModeDescribe)
	panel.NotifySelection(ctx, panelcontent.Selection{ID: "foo", Item: item})
	if len(spy.events) == 0 {
		t.Fatalf("expected describe widget to receive selection replay")
	}
	got := spy.events[len(spy.events)-1]
	if got.ID != "foo" {
		t.Fatalf("expected selection ID foo, got %s", got.ID)
	}
	if got.Item == nil {
		t.Fatalf("expected selection item to be populated")
	}
}

func TestPanelSelectionForceMessage(t *testing.T) {
	ctx := context.Background()
	panel := NewPanel("test")
	panel.listWidget(ctx)
	// Seed first selection to populate lastSelectionID.
	if cmd := panel.widgetSelectionChanged(ctx, panelcontent.Selection{ID: "item-a"}); cmd == nil {
		t.Fatalf("expected initial selection change command")
	} else {
		_ = cmd()
	}
	// Repeating the same selection without force should not emit a message.
	if cmd := panel.widgetSelectionChanged(ctx, panelcontent.Selection{ID: "item-a"}); cmd != nil {
		t.Fatalf("unexpected command when selection unchanged without force")
	}
	// Forcing should emit a PanelSelectionChangedMsg even without ID change.
	cmd := panel.widgetSelectionChanged(ctx, panelcontent.Selection{ID: "item-a", Force: true})
	if cmd == nil {
		t.Fatalf("expected forced selection change command")
	}
	if msg := cmd(); msg == nil {
		t.Fatalf("expected forced selection change message")
	} else if selMsg, ok := msg.(PanelSelectionChangedMsg); !ok {
		t.Fatalf("expected PanelSelectionChangedMsg, got %#v", msg)
	} else if selMsg.Selection.ID != "item-a" {
		t.Fatalf("expected selection ID item-a, got %s", selMsg.Selection.ID)
	}
}
