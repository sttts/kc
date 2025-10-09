package ui

import (
	"context"
	"strings"
	"testing"

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
	if !strings.Contains(view, "Manifest") {
		t.Fatalf("expected manifest content, got %q", view)
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
	} else if _, ok := msg.(PanelSelectionChangedMsg); !ok {
		t.Fatalf("expected PanelSelectionChangedMsg, got %#v", msg)
	}
}
