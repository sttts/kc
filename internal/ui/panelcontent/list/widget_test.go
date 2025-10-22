package list

import (
	"context"
	"fmt"
	"testing"

	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
)

func TestFrameInfoEmptyList(t *testing.T) {
	w := New(panelcontent.WidgetDeps{})

	info := w.FrameInfo(context.Background(), panelcontent.FrameInfoRequest{Width: 20})
	if info.TopIndicator != "─" || info.BottomIndicator != "─" {
		t.Fatalf("expected default indicators, got top=%q bottom=%q", info.TopIndicator, info.BottomIndicator)
	}
	if info.FooterStatus != "" {
		t.Fatalf("expected empty footer status, got %q", info.FooterStatus)
	}
	if info.SuppressFooter {
		t.Fatalf("expected footer not suppressed")
	}
}

func TestFrameInfoIndicatorsAndStatus(t *testing.T) {
	ctx := context.Background()
	w := New(panelcontent.WidgetDeps{})
	w.Resize(ctx, panelcontent.Size{Width: 16, Height: 6})

	items := make([]Item, 10)
	for i := range items {
		items[i] = Item{Name: fmt.Sprintf("item-%d", i+1)}
	}
	w.items = items

	w.selected = 0
	w.scroll = 0
	info := w.FrameInfo(ctx, panelcontent.FrameInfoRequest{Width: 16})
	if info.TopIndicator != "─" {
		t.Fatalf("expected top indicator '─', got %q", info.TopIndicator)
	}
	if info.BottomIndicator != "v" {
		t.Fatalf("expected bottom indicator 'v', got %q", info.BottomIndicator)
	}
	if got := info.FooterStatus; got != "1/10 • 40%" {
		t.Fatalf("expected footer status '1/10 • 40%%', got %q", got)
	}

	w.selected = 6
	w.scroll = 5
	info = w.FrameInfo(ctx, panelcontent.FrameInfoRequest{Width: 16})
	if info.TopIndicator != "^" {
		t.Fatalf("expected top indicator '^', got %q", info.TopIndicator)
	}
	if info.BottomIndicator != "v" {
		t.Fatalf("expected bottom indicator 'v', got %q", info.BottomIndicator)
	}
	if got := info.FooterStatus; got != "7/10 • 90%" {
		t.Fatalf("expected footer status '7/10 • 90%%', got %q", got)
	}
}
