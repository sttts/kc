package models

import (
	"context"
	"testing"
	"time"

	table "github.com/sttts/kc/internal/table"
	"github.com/sttts/kc/internal/tablecache"
	"github.com/sttts/kc/pkg/appconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
		func(onEvent func(), _ func()) (func(), error) {
			informer = onEvent
			return func() {}, nil
		},
	)

	src.watchTTL = time.Minute

	ctx := t.Context()
	first := src.Lines(ctx, 0, 10)
	if len(first) != len(rowsSet[0]) {
		t.Fatalf("expected %d rows, got %d", len(rowsSet[0]), len(first))
	}
	if informer == nil {
		t.Fatalf("expected informer callback to be registered on first access")
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

func TestObjectsFolderInstallsAgeHooks(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	folder := NewObjectsFolder(Deps{}, gvr, "default", []string{"namespaces", "default", "pods"}, nil)
	created := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	rl := &tablecache.RowList{
		Columns: []metav1.TableColumnDefinition{
			{Name: "Name"},
			{Name: "Age"},
		},
		Items: []tablecache.Row{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pod-a",
					CreationTimestamp: metav1.NewTime(created),
				},
				TableRow: metav1.TableRow{
					Cells: []interface{}{"/pod-a", "1s"},
				},
			},
		},
	}

	rows := folder.rowsFromRowList(rl, appconfig.ColumnsModeNormal, appconfig.ObjectsOrderName)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	folder.ageMu.Lock()
	if len(folder.ageHooks) != 1 {
		t.Fatalf("expected a single age hook, got %d", len(folder.ageHooks))
	}
	hook := folder.ageHooks[0]
	hook.now = func() time.Time { return created.Add(45 * time.Second) }
	if folder.ageTimer != nil {
		folder.ageTimer.Stop()
		folder.ageTimer = nil
	}
	folder.ageMu.Unlock()

	_, cells, _, ok := rows[0].Columns()
	if !ok {
		t.Fatalf("expected row columns")
	}
	if got := cells[1]; got != "45s" {
		t.Fatalf("expected Age column to update dynamically, got %q", got)
	}

	folder.installAgeHooks(nil)
}

func TestObjectsFolderMarksDeletingRowsRed(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	folder := NewObjectsFolder(Deps{}, gvr, "default", []string{"namespaces", "default", "pods"}, nil)
	now := metav1.NewTime(time.Now())
	rl := &tablecache.RowList{
		Columns: []metav1.TableColumnDefinition{
			{Name: "Name"},
		},
		Items: []tablecache.Row{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "delete-me",
					DeletionTimestamp: &now,
				},
				TableRow: metav1.TableRow{Cells: []interface{}{"/delete-me"}},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "regular"},
				TableRow:   metav1.TableRow{Cells: []interface{}{"/regular"}},
			},
		},
	}

	rows := folder.rowsFromRowList(rl, appconfig.ColumnsModeNormal, appconfig.ObjectsOrderName)
	var deleting table.Row
	for _, row := range rows {
		id, _, _, _ := row.Columns()
		if id == "delete-me" {
			deleting = row
			break
		}
	}
	if deleting == nil {
		t.Fatalf("expected to find deleting row")
	}
	_, _, styles, _ := deleting.Columns()
	if len(styles) == 0 || styles[0] != DeletingStyle() {
		t.Fatalf("expected deleting row to use red style")
	}
}

func TestObjectsFolderRowsFromListDeletingStyle(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	folder := NewObjectsFolder(Deps{}, gvr, "default", []string{"namespaces", "default", "configmaps"}, nil)
	ts := metav1.NewTime(time.Now())
	list := &unstructured.UnstructuredList{}
	obj := unstructured.Unstructured{}
	obj.SetName("delete-me")
	obj.SetDeletionTimestamp(&ts)
	list.Items = append(list.Items, obj)

	rows := folder.rowsFromList(list, appconfig.ObjectsOrderName)
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	_, _, styles, _ := rows[0].Columns()
	if len(styles) == 0 || styles[0] != DeletingStyle() {
		t.Fatalf("expected deleting style for fallback list path")
	}
}
