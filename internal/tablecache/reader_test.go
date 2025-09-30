package tablecache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReaderListTables(t *testing.T) {
	ctx := context.Background()

	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
	target := corev1.SchemeGroupVersion.WithKind("Pod")
	mapper.AddSpecific(target, schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pod"}, meta.RESTScopeNamespace)

	table := buildTestTable(t)
	fetcher := &fakeFetcher{table: table}
	delegate := &fakeDelegate{}
	reader := NewReaderWithFetcher(delegate, mapper, fetcher)

	list := NewRowList(target)
	if err := reader.List(ctx, list, client.InNamespace("default")); err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if delegate.listCalled {
		t.Fatalf("expected delegate List not to be called")
	}

	if fetcher.lastNamespace != "default" {
		t.Fatalf("expected namespace 'default', got %q", fetcher.lastNamespace)
	}

	if got := list.ListMeta.ResourceVersion; got != table.ResourceVersion {
		t.Fatalf("expected resourceVersion %q, got %q", table.ResourceVersion, got)
	}

	if len(list.Items) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(list.Items))
	}

	row := list.Items[0]
	if row.Name != "pod-a" || row.Namespace != "default" {
		t.Fatalf("unexpected metadata: %s/%s", row.Namespace, row.Name)
	}

	if row.TableTarget() != target {
		t.Fatalf("expected row target %v", target)
	}

	if len(row.Columns) != len(table.ColumnDefinitions) {
		t.Fatalf("expected %d columns, got %d", len(table.ColumnDefinitions), len(row.Columns))
	}
	if row.Columns[0].Name != "Name" {
		t.Fatalf("expected first column Name, got %s", row.Columns[0].Name)
	}
	if len(row.TableRow.Cells) == 0 || row.TableRow.Cells[0] != "pod-a" {
		t.Fatalf("expected first cell pod-a, got %v", row.TableRow.Cells)
	}

	if list.TableTarget() != target {
		t.Fatalf("expected list target %v", target)
	}
}

func TestReaderListFallback(t *testing.T) {
	ctx := context.Background()

	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
	fetcher := &fakeFetcher{}
	delegate := &fakeDelegate{listErr: fmt.Errorf("delegate called")}
	reader := NewReaderWithFetcher(delegate, mapper, fetcher)

	if err := reader.List(ctx, &corev1.PodList{}); err == nil || !strings.Contains(err.Error(), "delegate called") {
		t.Fatalf("expected delegate error, got %v", err)
	}
}

func TestReaderListMissingTarget(t *testing.T) {
	ctx := context.Background()

	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
	fetcher := &fakeFetcher{}
	delegate := &fakeDelegate{}
	reader := NewReaderWithFetcher(delegate, mapper, fetcher)

	if err := reader.List(ctx, &RowList{}); err == nil || !strings.Contains(err.Error(), "missing table target") {
		t.Fatalf("expected missing target error, got %v", err)
	}
}

func TestReaderWatchTables(t *testing.T) {
	ctx := context.Background()

	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
	target := corev1.SchemeGroupVersion.WithKind("Pod")
	mapper.AddSpecific(target, schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pod"}, meta.RESTScopeNamespace)

	table := buildTestTable(t)
	fakeWatch := watch.NewFake()
	fetcher := &fakeFetcher{table: table, watch: fakeWatch}
	delegate := &fakeDelegate{}
	reader := NewReaderWithFetcher(delegate, mapper, fetcher)

	result, err := reader.Watch(ctx, NewRowList(target))
	if err != nil {
		t.Fatalf("Watch returned error: %v", err)
	}

	if delegate.watchCalled {
		t.Fatalf("expected delegate Watch not to be called")
	}

	fakeWatch.Add(table.DeepCopy())

	for i, name := range []string{"pod-a", "pod-b"} {
		select {
		case evt := <-result.ResultChan():
			row, ok := evt.Object.(*Row)
			if !ok {
				t.Fatalf("expected Row event, got %T", evt.Object)
			}
			if evt.Type != watch.Added {
				t.Fatalf("unexpected event type %v", evt.Type)
			}
			if row.Name != name {
				t.Fatalf("expected name %q at index %d, got %q", name, i, row.Name)
			}
			if row.TableTarget() != target {
				t.Fatalf("expected row target %v", target)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for watch event")
		}
	}

	result.Stop()
	fakeWatch.Stop()
}

func TestReaderWatchFallback(t *testing.T) {
	ctx := context.Background()

	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
	fakeWatch := watch.NewFake()
	fetcher := &fakeFetcher{}
	delegate := &fakeDelegate{watch: fakeWatch}
	reader := NewReaderWithFetcher(delegate, mapper, fetcher)

	if _, err := reader.Watch(ctx, &corev1.PodList{}); err != nil {
		t.Fatalf("expected delegate Watch to succeed, got %v", err)
	}

	if !delegate.watchCalled {
		t.Fatalf("expected delegate watch to be called")
	}
}

func buildTestTable(t *testing.T) *metav1.Table {
	t.Helper()

	pods := []corev1.Pod{
		{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "pod-a",
				Namespace:       "default",
				Labels:          map[string]string{"app": "demo"},
				Annotations:     map[string]string{"anno": "value"},
				Finalizers:      []string{"finalizer"},
				ResourceVersion: "100",
				UID:             "uid-a",
				ManagedFields:   []metav1.ManagedFieldsEntry{{Manager: "manager"}},
			},
		},
		{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "pod-b",
				Namespace:       "default",
				ResourceVersion: "101",
				UID:             "uid-b",
			},
		},
	}

	rows := make([]metav1.TableRow, len(pods))
	for i := range pods {
		raw, err := json.Marshal(&pods[i])
		if err != nil {
			t.Fatalf("marshal pod: %v", err)
		}
		rows[i] = metav1.TableRow{
			Cells:  []interface{}{pods[i].Name},
			Object: runtime.RawExtension{Raw: raw},
		}
	}

	return &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name", Type: "string"}},
		Rows:              rows,
		ListMeta:          metav1.ListMeta{ResourceVersion: "200"},
	}
}

type fakeFetcher struct {
	table         *metav1.Table
	watch         watch.Interface
	listErr       error
	watchErr      error
	lastNamespace string
	lastListOpts  metav1.ListOptions
	lastWatchOpts metav1.ListOptions
}

func (f *fakeFetcher) ListTable(_ context.Context, _ *meta.RESTMapping, namespace string, opts metav1.ListOptions) (*metav1.Table, error) {
	f.lastNamespace = namespace
	f.lastListOpts = opts
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.table == nil {
		return nil, fmt.Errorf("no table configured")
	}
	return f.table.DeepCopy(), nil
}

func (f *fakeFetcher) WatchTable(_ context.Context, _ *meta.RESTMapping, namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	f.lastNamespace = namespace
	f.lastWatchOpts = opts
	if f.watchErr != nil {
		return nil, f.watchErr
	}
	if f.watch == nil {
		return nil, fmt.Errorf("no watch configured")
	}
	return f.watch, nil
}

func (f *fakeFetcher) WatchObjects(ctx context.Context, mapping *meta.RESTMapping, namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return f.WatchTable(ctx, mapping, namespace, opts)
}

func (f *fakeFetcher) GetTable(context.Context, *meta.RESTMapping, string, string) (*metav1.Table, error) {
	if f.table == nil {
		return nil, fmt.Errorf("no table configured")
	}
	return f.table.DeepCopy(), nil
}

type fakeDelegate struct {
	listCalled  bool
	watchCalled bool
	listErr     error
	watch       watch.Interface
}

func (f *fakeDelegate) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return nil
}

func (f *fakeDelegate) List(context.Context, client.ObjectList, ...client.ListOption) error {
	f.listCalled = true
	return f.listErr
}

func (f *fakeDelegate) Watch(context.Context, client.ObjectList, ...client.ListOption) (watch.Interface, error) {
	f.watchCalled = true
	if f.watch != nil {
		return f.watch, nil
	}
	return nil, fmt.Errorf("delegate watch not configured")
}
