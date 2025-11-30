package ui

import (
	"context"
	"testing"

	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
)

func TestPlaceholderFrameInfoSuppressesFooter(t *testing.T) {
	widget := newPlaceholderWidget(nil, "unimplemented", "")
	provider, ok := widget.(panelcontent.FrameInfoProvider)
	if !ok {
		t.Fatalf("placeholder widget does not implement FrameInfoProvider")
	}

	info := provider.FrameInfo(context.Background(), panelcontent.FrameInfoRequest{Width: 20})
	if !info.SuppressFooter {
		t.Fatalf("expected footer suppression")
	}
	if info.TopIndicator != "─" || info.BottomIndicator != "─" {
		t.Fatalf("expected default indicators, got top=%q bottom=%q", info.TopIndicator, info.BottomIndicator)
	}
	if info.FooterStatus != "" {
		t.Fatalf("expected empty footer status, got %q", info.FooterStatus)
	}
}
