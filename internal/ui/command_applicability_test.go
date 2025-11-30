package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
	models "github.com/sttts/kc/internal/models"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// namespaceObject is a minimal namespace item.
type namespaceObject struct {
	name string
}

func (n namespaceObject) Columns() (string, []string, []*lipgloss.Style, bool) {
	return "", nil, nil, true
}
func (n namespaceObject) Details() string { return "" }
func (n namespaceObject) Path() []string  { return nil }
func (n namespaceObject) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
}
func (n namespaceObject) Namespace() string        { return "" }
func (n namespaceObject) Name() string             { return n.name }
func (n namespaceObject) SupportsVerb(string) bool { return true }

var _ models.ObjectItem = namespaceObject{}

func TestDeriveNamespace(t *testing.T) {
	tests := []struct {
		name            string
		item            models.Item
		selected        []Item
		folderNamespace string
		want            string
	}{
		{
			name: "namespaced object wins",
			item: stubObject{namespace: "dev"},
			want: "dev",
		},
		{
			name: "namespace object uses its name",
			item: namespaceObject{name: "kube-system"},
			want: "kube-system",
		},
		{
			name: "cluster-scoped object does not inherit folder namespace",
			item: stubObject{namespace: "", id: "node"},
			selected: []Item{
				{Item: stubObject{namespace: "", id: "node"}},
			},
			folderNamespace: "dev",
			want:            "",
		},
		{
			name:            "folder namespace used when no selection",
			folderNamespace: "dev",
			want:            "dev",
		},
		{
			name: "selected slice with namespaced item wins",
			item: nil,
			selected: []Item{
				{Item: stubObject{namespace: "staging"}},
			},
			folderNamespace: "dev",
			want:            "staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveNamespace(tt.item, tt.selected, tt.folderNamespace); got != tt.want {
				t.Fatalf("deriveNamespace() = %q, want %q", got, tt.want)
			}
		})
	}
}
