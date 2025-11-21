package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
)

func TestRenderFrameIndicatorsWithoutFooter(t *testing.T) {
	p := NewPanel("test")
	content := strings.Repeat(" ", 10)
	info := panelcontent.FrameInfo{
		Breadcrumb:      "/paths/example",
		FooterStatus:    "6/10 • 60%",
		TopIndicator:    "^",
		BottomIndicator: "v",
		SuppressFooter:  true,
	}
	frame := p.renderFrame(content, info, "Title", 30, 6, true, false)
	lines := strings.Split(ansi.Strip(frame), "\n")
	if len(lines) == 0 {
		t.Fatalf("rendered frame empty")
	}
	top := lines[0]
	if !strings.HasSuffix(top, "▲┐") {
		t.Fatalf("expected top line to end with '▲┐', got %q", top)
	}
	bottom := lines[len(lines)-1]
	if !strings.Contains(bottom, "6/10 • 60%─▼") || !strings.HasSuffix(bottom, "▼┘") {
		t.Fatalf("expected bottom line to contain status and arrow before corner, got %q", bottom)
	}
}

func TestRenderFrameIndicatorsWithFooter(t *testing.T) {
	p := NewPanel("test")
	content := strings.Repeat(" ", 10)
	info := panelcontent.FrameInfo{
		Breadcrumb:      "/paths/example",
		FooterStatus:    "6/10 • 60%",
		TopIndicator:    "^",
		BottomIndicator: "v",
		SuppressFooter:  false,
	}
	frame := p.renderFrame(content, info, "Title", 30, 6, true, true)
	lines := strings.Split(ansi.Strip(frame), "\n")
	if len(lines) == 0 {
		t.Fatalf("rendered frame empty")
	}
	top := lines[0]
	if !strings.HasSuffix(top, "▲┐") {
		t.Fatalf("expected top line to end with '▲┐', got %q", top)
	}
	bottom := lines[len(lines)-1]
	if !strings.HasPrefix(bottom, "├") || strings.Contains(bottom, "6/10") || !strings.HasSuffix(bottom, "▼┤") {
		t.Fatalf("expected bottom line with arrow and no status before corner when footer present, got %q", bottom)
	}
}
