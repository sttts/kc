package models

import (
	"context"
	"testing"

	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestLiveObjectRowSourceRefresh(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	rowsSet := [][]table.Row{
		{
			NewObjectRow("apps/v1/deployments/foo", []string{"/foo", "apps/v1", ""}, []string{"root", "foo"}, gvr, "default", "foo", WhiteStyle()),
			NewObjectRow("apps/v1/deployments/bar", []string{"/bar", "apps/v1", ""}, []string{"root", "bar"}, gvr, "default", "bar", WhiteStyle()),
		},
		{
			NewObjectRow("apps/v1/deployments/baz", []string{"/baz", "apps/v1", ""}, []string{"root", "baz"}, gvr, "default", "baz", WhiteStyle()),
		},
	}

	idx := 0
	populateCalls := 0
	folderDirty := 0
	var informer func()

	src := newLiveObjectRowSourceWithHooks(
		func(context.Context) ([]table.Row, error) {
			populateCalls++
			rows := append([]table.Row(nil), rowsSet[idx]...)
			return rows, nil
		},
		func() { folderDirty++ },
		func(cb func()) { informer = cb },
	)

	if informer == nil {
		t.Fatalf("expected informer callback to be registered")
	}

	ctx := t.Context()
	first := src.Lines(ctx, 0, 10)
	if len(first) != len(rowsSet[0]) {
		t.Fatalf("expected %d rows, got %d", len(rowsSet[0]), len(first))
	}
	if populateCalls != 1 {
		t.Fatalf("expected one populate call, got %d", populateCalls)
	}
	if src.Len(ctx) != len(rowsSet[0]) {
		t.Fatalf("Len mismatch: got %d", src.Len(ctx))
	}
	if _, row, ok := src.Find(ctx, "apps/v1/deployments/foo"); !ok || row == nil {
		t.Fatalf("expected to find foo row")
	}
	if above := src.Above(ctx, "apps/v1/deployments/foo", 1); len(above) != 0 {
		t.Fatalf("expected no rows above the first entry")
	}
	if below := src.Below(ctx, "apps/v1/deployments/foo", 1); len(below) != 1 || below[0] == nil {
		t.Fatalf("expected one row below the first entry")
	}

	idx = 1
	informer()
	if folderDirty != 1 {
		t.Fatalf("expected folder dirty once, got %d", folderDirty)
	}

	second := src.Lines(ctx, 0, 10)
	if len(second) != len(rowsSet[1]) {
		t.Fatalf("expected %d rows after refresh, got %d", len(rowsSet[1]), len(second))
	}
	if populateCalls != 2 {
		t.Fatalf("expected populate to be called again, got %d", populateCalls)
	}
	if _, _, ok := src.Find(ctx, "apps/v1/deployments/foo"); ok {
		t.Fatalf("expected old row to disappear after refresh")
	}
	if _, row, ok := src.Find(ctx, "apps/v1/deployments/baz"); !ok || row == nil {
		t.Fatalf("expected new row to be findable")
	}

	secondAgain := src.Lines(ctx, 0, 10)
	if populateCalls != 2 {
		t.Fatalf("expected no extra populate on cache hit, got %d", populateCalls)
	}
	if len(secondAgain) != len(rowsSet[1]) {
		t.Fatalf("unexpected row count on cached read")
	}
}
