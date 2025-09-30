package tablecache

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestRowTableTarget(t *testing.T) {
	target := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	row := NewRow(target)

	if got := row.TableTarget(); got != target {
		t.Fatalf("expected target %v, got %v", target, got)
	}

	row.SetTableTarget(schema.GroupVersionKind{})
	if !row.TableTarget().Empty() {
		t.Fatalf("expected target to be empty after reset")
	}
}

func TestRowListTableTarget(t *testing.T) {
	target := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
	list := NewRowList(target)

	if got := list.TableTarget(); got != target {
		t.Fatalf("expected target %v, got %v", target, got)
	}

	if got := list.TypeMeta.APIVersion; got != SchemeGroupVersion.String() {
		t.Fatalf("expected type meta apiVersion %q, got %q", SchemeGroupVersion.String(), got)
	}

	if got := list.TypeMeta.Kind; got != RowListKind {
		t.Fatalf("expected kind %q, got %q", RowListKind, got)
	}
}
