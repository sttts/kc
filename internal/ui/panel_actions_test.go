package ui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	models "github.com/sttts/kc/internal/models"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type stubObject struct {
	id        string
	namespace string
	name      string
}

var (
	_ models.Item       = stubObject{}
	_ models.ObjectItem = stubObject{}
	_ models.Viewable   = stubObject{}
)

func (s stubObject) Columns() (string, []string, []*lipgloss.Style, bool) {
	return s.id, []string{s.name}, nil, true
}

func (s stubObject) Details() string { return "" }
func (s stubObject) Path() []string  { return nil }

func (s stubObject) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "group", Version: "v1", Resource: "tests"}
}

func (s stubObject) Namespace() string { return s.namespace }
func (s stubObject) Name() string      { return s.name }

func (s stubObject) ViewContent() (string, string, string, string, string, error) {
	return "title", "body", "yaml", "application/yaml", "title.yaml", nil
}

func TestPanelCapabilitiesWithActions(t *testing.T) {
	panel := NewPanel("test")
	panel.SetEnvironmentSupplier(func() PanelEnvironment {
		return PanelEnvironment{
			AllowEditObjects:      true,
			AllowDeleteObjects:    true,
			AllowCreateNamespaces: true,
		}
	})
	obj := stubObject{
		id:        "group/v1/tests/foo",
		namespace: "ns",
		name:      "foo",
	}
	panel.items = []Item{
		{Item: obj, Name: obj.name},
	}
	panel.selected = 0
	panel.SetCurrentPath("/namespaces")

	var invoked []PanelAction
	panel.SetActionHandlers(PanelActionHandlers{
		PanelActionView: func(*Panel) tea.Cmd {
			invoked = append(invoked, PanelActionView)
			return nil
		},
		PanelActionEdit: func(*Panel) tea.Cmd {
			invoked = append(invoked, PanelActionEdit)
			return nil
		},
		PanelActionDelete: func(*Panel) tea.Cmd {
			invoked = append(invoked, PanelActionDelete)
			return nil
		},
		PanelActionCreateNamespace: func(*Panel) tea.Cmd {
			invoked = append(invoked, PanelActionCreateNamespace)
			return nil
		},
	})

	ctx := context.Background()
	caps := panel.Capabilities(ctx)
	if !caps.CanView || !caps.CanEdit || !caps.CanDelete || !caps.CanCreateNS {
		t.Fatalf("capabilities not computed correctly: %+v", caps)
	}
	panel.invokeActionIfAllowed(ctx, PanelActionView)
	panel.invokeActionIfAllowed(ctx, PanelActionEdit)
	panel.invokeActionIfAllowed(ctx, PanelActionDelete)
	panel.invokeActionIfAllowed(ctx, PanelActionCreateNamespace)

	if len(invoked) != 4 {
		t.Fatalf("expected 4 invocations, got %d", len(invoked))
	}
}
