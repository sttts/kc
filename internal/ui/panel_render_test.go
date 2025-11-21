package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea/v2"
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
	if !strings.HasPrefix(bottom, "└") || !strings.HasSuffix(bottom, "▼┘") {
		t.Fatalf("expected bottom line to start/end with corners, got %q", bottom)
	}
	if !strings.Contains(bottom, "6/10 • 60% • List") {
		t.Fatalf("expected status and mode on bottom border, got %q", bottom)
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
	if !strings.HasPrefix(bottom, "├") || !strings.HasSuffix(bottom, "▼┤") {
		t.Fatalf("expected bottom line with adjusted corners and arrow when footer present, got %q", bottom)
	}
	if !strings.Contains(bottom, "6/10 • 60% • List") {
		t.Fatalf("expected status and mode on bottom border even with footer, got %q", bottom)
	}
}

func TestFooterIndentWithoutMode(t *testing.T) {
	p := NewPanel("test")
	p.width = 20
	stub := &footerStub{footer: "footer"}
	p.widgets[p.mode] = stub

	footer := p.renderFooter(context.Background(), "", false)
	lines := strings.Split(ansi.Strip(footer), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected footer lines")
	}
	last := lines[len(lines)-1]
	if strings.Contains(last, "List") {
		t.Fatalf("expected footer to omit mode label, got %q", last)
	}
	if !strings.HasPrefix(last, "footer") {
		t.Fatalf("expected footer content left-aligned without leading space, got %q", last)
	}
}

type footerStub struct {
	footer string
}

func (f *footerStub) Init(context.Context) tea.Cmd                    { return nil }
func (f *footerStub) Update(context.Context, tea.Msg) (tea.Cmd, bool) { return nil, false }
func (f *footerStub) View(context.Context, panelcontent.Frame) string { return f.footer }
func (f *footerStub) Resize(context.Context, panelcontent.Size)       {}
func (f *footerStub) SetFocus(context.Context, bool)                  {}
func (f *footerStub) Teardown(context.Context)                        {}
func (f *footerStub) Footer(context.Context, int) string              { return f.footer }
func (f *footerStub) FrameInfo(context.Context, panelcontent.FrameInfoRequest) panelcontent.FrameInfo {
	return panelcontent.FrameInfo{}
}
