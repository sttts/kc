package describe

import (
	"context"
	"testing"

	"github.com/charmbracelet/lipgloss/v2"
	models "github.com/sttts/kc/internal/models"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
	describe "github.com/sttts/kc/pkg/describe"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestDescribeWidgetInvokesDescribeProvider(t *testing.T) {
	ctx := context.Background()
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	style := lipgloss.NewStyle()
	obj := models.NewObjectRow("pod/test", []string{"test"}, []string{"namespaces", "default", "pods", "test"}, gvr, "default", "test", &style)
	var called bool
	var target describe.Target
	w := New(panelcontent.WidgetDeps{
		Describe: func(_ context.Context, tgt describe.Target) (describe.Result, error) {
			called = true
			target = tgt
			return describe.Result{Title: "ok", Body: "body"}, nil
		},
	})

	w.OnSelectionChanged(ctx, panelcontent.Selection{Item: obj})

	if !called {
		t.Fatalf("expected describe provider to be invoked")
	}
	if target.Namespace != "default" || target.Name != "test" {
		t.Fatalf("unexpected describe target: %#v", target)
	}
	if target.GVR != gvr {
		t.Fatalf("unexpected GVR: %#v", target.GVR)
	}
}

func TestDescribeWidgetUsesSelectedItemFallback(t *testing.T) {
	ctx := context.Background()
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	style := lipgloss.NewStyle()
	obj := models.NewObjectRow("pod/test", []string{"test"}, []string{"namespaces", "default", "pods", "test"}, gvr, "default", "test", &style)
	var called bool
	w := New(panelcontent.WidgetDeps{
		SelectedItem: func(context.Context) (models.Item, bool) {
			return obj, true
		},
		Describe: func(context.Context, describe.Target) (describe.Result, error) {
			called = true
			return describe.Result{}, nil
		},
	})

	w.OnSelectionChanged(ctx, panelcontent.Selection{})

	if !called {
		t.Fatalf("expected describe provider to be invoked via SelectedItem fallback")
	}
}
