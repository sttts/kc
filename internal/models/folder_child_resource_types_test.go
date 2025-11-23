package models

import (
	"context"
	"reflect"
	"testing"

	"github.com/sttts/kc/pkg/appconfig"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestChildResourceTypesFolderListsEntries(t *testing.T) {
	deps := Deps{
		AppConfig: appconfig.Default(),
	}
	parentGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	childGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	path := []string{"namespaces", "demo", "deployments", "demo-deploy"}

	folder := NewChildResourceTypesFolder(deps, parentGVR, "demo", "demo-deploy", []schema.GroupVersionResource{childGVR}, path)
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
	wantPath := append(path, childGVR.Resource)
	if got := item.Path(); !reflect.DeepEqual(got, wantPath) {
		t.Fatalf("Path() = %v, want %v", got, wantPath)
	}
	if got := folder.Columns(); len(got) != 1 || got[0].Title != " Name" {
		t.Fatalf("Columns() = %v, want single Name column", got)
	}
}

func TestChildResourceTypesFolderMultipleEntries(t *testing.T) {
	deps := Deps{
		AppConfig: appconfig.Default(),
	}
	parentGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
	childA := schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}
	childB := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	path := []string{"namespaces", "demo", "services", "svc-a"}

	folder := NewChildResourceTypesFolder(deps, parentGVR, "demo", "svc-a", []schema.GroupVersionResource{childA, childB}, path)
	rows, err := folder.populate(context.Background())
	if err != nil {
		t.Fatalf("populate: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	paths := [][]string{rows[0].(*ChildResourceItem).Path(), rows[1].(*ChildResourceItem).Path()}
	want0 := append(path, childA.Resource)
	want1 := append(path, childB.Resource)
	if !reflect.DeepEqual(paths[0], want0) || !reflect.DeepEqual(paths[1], want1) {
		t.Fatalf("paths = %v, want [%v %v]", paths, want0, want1)
	}
}
