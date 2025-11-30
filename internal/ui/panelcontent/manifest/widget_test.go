package manifest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

type objectViewableStub struct {
	viewableStub
	gvr schema.GroupVersionResource
	ns  string
	nm  string
}

func (o objectViewableStub) GVR() schema.GroupVersionResource { return o.gvr }
func (o objectViewableStub) Namespace() string                { return o.ns }
func (o objectViewableStub) Name() string                     { return o.nm }
func (objectViewableStub) SupportsVerb(string) bool           { return false }

func TestFrameInfoProvidesIndicatorsAndStatus(t *testing.T) {
	ctx := context.Background()
	w := New(panelcontent.WidgetDeps{})
	_, _ = w.Update(ctx, tea.WindowSizeMsg{Width: 40, Height: 5})

	lines := make([]string, 10)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %02d", i+1)
	}
	item := objectViewableStub{
		viewableStub: viewableStub{
			body: lines,
			path: []string{"namespaces", "default", "pods", "stub"},
		},
		gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		ns:  "default",
		nm:  "stub",
	}
	w.OnSelectionChanged(ctx, panelcontent.Selection{
		ID:   "stub",
		Path: "/namespaces/default/pods/stub",
		Item: item,
	})

	info := w.FrameInfo(ctx, panelcontent.FrameInfoRequest{Width: 40})
	if info.SuppressFooter {
		t.Fatalf("expected footer to be rendered for manifest mode selection")
	}
	if info.TopIndicator != "─" {
		t.Fatalf("expected top indicator '─', got %q", info.TopIndicator)
	}
	if info.BottomIndicator != "v" {
		t.Fatalf("expected bottom indicator 'v', got %q", info.BottomIndicator)
	}
	if got := info.FooterStatus; got != "5/10" {
		t.Fatalf("expected footer status %q, got %q", "5/10", got)
	}
	expectedBreadcrumb := "/namespaces/default/pods/stub"
	if info.Breadcrumb != expectedBreadcrumb {
		t.Fatalf("expected breadcrumb %q, got %q", expectedBreadcrumb, info.Breadcrumb)
	}
}
