package ui

import (
	"context"
	"testing"
)

func TestPanelModeSwitchesToPlaceholder(t *testing.T) {
	panel := NewPanel("test")
	ctx := context.Background()
	panel.SetDimensions(ctx, 20, 5)
	panel.SetMode(ctx, PanelModeManifest)
	if panel.Mode() != PanelModeManifest {
		t.Fatalf("expected manifest mode, got %v", panel.Mode())
	}
	view := panel.renderContent(ctx)
	if view == "" {
		t.Fatalf("expected placeholder content")
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
	panel.items = []Item{{Name: "a"}, {Name: "b"}}
	panel.SetDimensions(ctx, 10, 5)
	panel.selectionChangedCmd(ctx) // seed initial state
	panel.moveDown(ctx)
	cmd := panel.selectionChangedCmd(ctx)
	if cmd == nil {
		t.Fatalf("expected selection change command")
	}
	if msg := cmd(); msg == nil {
		t.Fatalf("expected selection change message")
	} else if _, ok := msg.(PanelSelectionChangedMsg); !ok {
		t.Fatalf("expected PanelSelectionChangedMsg, got %#v", msg)
	}
}
