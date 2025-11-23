package models

import (
	"context"
	"reflect"
	"testing"

	kccluster "github.com/sttts/kc/internal/cluster"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
)

func TestNodeChildFolderPopulatesPodsEntry(t *testing.T) {
	deps := Deps{}
	path := []string{"nodes", "node-a"}
	folder := NewNodeChildFolder(deps, "node-a", path)

	rows, err := folder.populate(context.Background())
	if err != nil {
		t.Fatalf("populate: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	item, ok := rows[0].(*ChildResourceItem)
	if !ok {
		t.Fatalf("row type = %T, want *ChildResourceItem", rows[0])
	}
	wantPath := []string{"nodes", "node-a", "pods"}
	if got := item.Path(); !reflect.DeepEqual(got, wantPath) {
		t.Fatalf("path = %v, want %v", got, wantPath)
	}
	if _, cells, _, ok := item.Columns(); !ok || len(cells) == 0 || cells[0] != "/pods" {
		t.Fatalf("first cell = %v, want /pods", cells)
	}
}

func TestNodePodsFolderFiltersByNodeName(t *testing.T) {
	deps := Deps{}
	path := []string{"nodes", "node-a", "pods"}
	folder := NewNodePodsFolder(deps, path, "node-a")

	matching := &unstructured.Unstructured{}
	matching.Object = map[string]interface{}{
		"spec": map[string]interface{}{
			"nodeName": "node-a",
		},
	}
	other := &unstructured.Unstructured{}
	other.Object = map[string]interface{}{
		"spec": map[string]interface{}{
			"nodeName": "node-b",
		},
	}

	if folder.FilterUnstructured == nil {
		t.Fatalf("FilterUnstructured should be set")
	}
	if !folder.FilterUnstructured(matching) {
		t.Fatalf("expected matching pod to pass filter")
	}
	if folder.FilterUnstructured(other) {
		t.Fatalf("expected non-matching pod to be filtered out")
	}
}

func TestDepsForSelectorScopeSetsSelectorKey(t *testing.T) {
	scope := map[schema.GroupVersionResource]crcache.ByObject{
		{Group: "", Version: "v1", Resource: "pods"}: {Field: fields.OneTermEqualSelector("spec.nodeName", "node-a")},
	}
	var gotKey string
	deps := Deps{
		Ctx: context.Background(),
		ClusterKey: kccluster.Key{
			KubeconfigPath: "/tmp/kubeconfig",
			ContextName:    "ctx",
		},
		NamespaceFactory: func(_ context.Context, key kccluster.Key, selectors map[schema.GroupVersionResource]crcache.ByObject) (*kccluster.Cluster, error) {
			gotKey = key.SelectorKey
			return &kccluster.Cluster{}, nil
		},
	}

	out := deps.ForSelectorScope(scope)
	if out.ClusterKey.SelectorKey == "" {
		t.Fatalf("expected SelectorKey to be set")
	}
	if gotKey != out.ClusterKey.SelectorKey {
		t.Fatalf("factory saw key %q, deps key %q", gotKey, out.ClusterKey.SelectorKey)
	}
}
