package manifest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss/v2"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
)

type viewableStub struct {
	body []string
	path []string
}

func (viewableStub) Columns() (string, []string, []*lipgloss.Style, bool) {
	return "stub", []string{"stub"}, nil, true
}

func (viewableStub) Details() string { return "" }

func (v viewableStub) Path() []string { return v.path }

func (v viewableStub) ViewContent() (string, string, string, string, string, error) {
	return "Manifest", strings.Join(v.body, "\n"), "yaml", "text/yaml", "stub.yaml", nil
}

func TestFrameInfoProvidesIndicatorsAndStatus(t *testing.T) {
	ctx := context.Background()
	w := New(panelcontent.WidgetDeps{})
	w.Resize(ctx, panelcontent.Size{Width: 40, Height: 5})

	lines := make([]string, 10)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %02d", i+1)
	}
	item := viewableStub{
		body: lines,
		path: []string{"namespaces", "default", "pods", "stub"},
	}
	w.OnSelectionChanged(ctx, panelcontent.Selection{
		ID:   "stub",
		Path: "/namespaces/default/pods/stub",
		Item: item,
	})

	info := w.FrameInfo(ctx, panelcontent.FrameInfoRequest{Width: 40})
	if !info.SuppressFooter {
		t.Fatalf("expected footer suppression for manifest mode")
	}
	if info.TopIndicator != "─" {
		t.Fatalf("expected top indicator '─', got %q", info.TopIndicator)
	}
	if info.BottomIndicator != "v" {
		t.Fatalf("expected bottom indicator 'v', got %q", info.BottomIndicator)
	}
	if got := info.FooterStatus; got != "5/10 • 50%" {
		t.Fatalf("expected footer status '5/10 • 50%%', got %q", got)
	}
	expectedBreadcrumb := "/namespaces/default/pods/stub"
	if info.Breadcrumb != expectedBreadcrumb {
		t.Fatalf("expected breadcrumb %q, got %q", expectedBreadcrumb, info.Breadcrumb)
	}
}
