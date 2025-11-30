package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
	p.Update(tea.WindowSizeMsg{Width: 20, Height: 5})
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

func TestRenderWithoutFooterKeepsContentHeight(t *testing.T) {
	ctx := context.Background()
	p := NewPanel("test")
	stub := &sizingWidget{
		info: panelcontent.FrameInfo{
			SuppressFooter: true,
			FooterStatus:   "status",
		},
	}
	p.widgets[p.mode] = stub

	panelHeight := 8
	model, _ := p.Update(tea.WindowSizeMsg{Width: 30, Height: panelHeight})
	p = model.(*Panel)
	p.Render(ctx, 30, panelHeight, false)

	expectedHeight := max(1, panelHeight-2)
	if stub.last.Height != expectedHeight {
		t.Fatalf("expected widget height %d, got %d", expectedHeight, stub.last.Height)
	}
}

type footerStub struct {
	footer string
}

func (f *footerStub) Init(context.Context) tea.Cmd                    { return nil }
func (f *footerStub) Update(context.Context, tea.Msg) (tea.Cmd, bool) { return nil, false }
func (f *footerStub) View(context.Context, panelcontent.Frame) tea.View {
	return tea.NewView(f.footer)
}
func (f *footerStub) SetFocus(context.Context, bool)     {}
func (f *footerStub) Teardown(context.Context)           {}
func (f *footerStub) Footer(context.Context, int) string { return f.footer }
func (f *footerStub) FrameInfo(context.Context, panelcontent.FrameInfoRequest) panelcontent.FrameInfo {
	return panelcontent.FrameInfo{}
}

type sizingWidget struct {
	info panelcontent.FrameInfo
	last panelcontent.Size
}

func (s *sizingWidget) Init(context.Context) tea.Cmd { return nil }
func (s *sizingWidget) Update(_ context.Context, msg tea.Msg) (tea.Cmd, bool) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		s.last = panelcontent.Size{Width: ws.Width, Height: ws.Height}
		return nil, true
	}
	return nil, false
}
func (s *sizingWidget) View(_ context.Context, frame panelcontent.Frame) tea.View {
	lines := make([]string, max(1, frame.Size.Height))
	for i := range lines {
		lines[i] = "x"
	}
	return tea.NewView(strings.Join(lines, "\n"))
}
func (s *sizingWidget) SetFocus(context.Context, bool) {}
func (s *sizingWidget) Teardown(context.Context)       {}
func (s *sizingWidget) FrameInfo(context.Context, panelcontent.FrameInfoRequest) panelcontent.FrameInfo {
	return s.info
}
