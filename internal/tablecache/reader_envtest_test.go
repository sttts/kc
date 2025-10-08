package tablecache

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func TestReaderListPodsEnvtest(t *testing.T) {
	if testCfg == nil {
		t.Skip("envtest not available")
	}

	ctx := t.Context()

	httpClient, err := rest.HTTPClientFor(testCfg)
	if err != nil {
		t.Fatalf("build http client: %v", err)
	}
	restMapper, err := apiutil.NewDynamicRESTMapper(testCfg, httpClient)
	if err != nil {
		t.Fatalf("build rest mapper: %v", err)
	}

	cl, err := client.NewWithWatch(testCfg, client.Options{Scheme: testScheme})
	if err != nil {
		t.Fatalf("build client: %v", err)
	}

	reader, err := NewReader(cl, restMapper, testCfg, testScheme)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tablecache-list"}}
	if err := cl.Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}

	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: ns.Name}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pause", Image: "pause"}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: ns.Name}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pause", Image: "pause"}}}},
	}

	for i := range pods {
		pod := pods[i]
		pod.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
		if err := cl.Create(ctx, &pod); err != nil {
			t.Fatalf("create pod %s: %v", pod.Name, err)
		}
	}

	podGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
	list := NewRowList(podGVK)
	if err := reader.List(ctx, list, client.InNamespace(ns.Name)); err != nil {
		t.Fatalf("list rows: %v", err)
	}

	if len(list.Items) != len(pods) {
		t.Fatalf("expected %d rows, got %d", len(pods), len(list.Items))
	}

	if list.TableTarget() != podGVK {
		t.Fatalf("expected list target %v, got %v", podGVK, list.TableTarget())
	}

	expectedColumns := []string{"Name", "Ready", "Status", "Restarts"}
	for _, row := range list.Items {
		if row.TableTarget() != podGVK {
			t.Fatalf("row %s target mismatch", row.Name)
		}
		if row.Namespace != ns.Name {
			t.Fatalf("row %s namespace = %s, want %s", row.Name, row.Namespace, ns.Name)
		}
		if len(row.Columns) < len(expectedColumns) {
			t.Fatalf("row %s columns = %d, want >= %d", row.Name, len(row.Columns), len(expectedColumns))
		}
		for i, name := range expectedColumns {
			if row.Columns[i].Name != name {
				t.Fatalf("row column[%d] = %s, want %s", i, row.Columns[i].Name, name)
			}
		}
		if len(row.TableRow.Cells) < len(expectedColumns) {
			t.Fatalf("row %s cells = %d, want >= %d", row.Name, len(row.TableRow.Cells), len(expectedColumns))
		}
		expectedValues := []string{row.Name, "0/1", "Pending", "0"}
		for i, want := range expectedValues {
			if fmt.Sprint(row.TableRow.Cells[i]) != want {
				t.Fatalf("row %s cell[%d] = %v, want %s", row.Name, i, row.TableRow.Cells[i], want)
			}
		}
	}
}

func TestReaderWatchPodsEnvtest(t *testing.T) {
	if testCfg == nil {
		t.Skip("envtest not available")
	}

	ctx := t.Context()

	httpClient, err := rest.HTTPClientFor(testCfg)
	if err != nil {
		t.Fatalf("build http client: %v", err)
	}
	restMapper, err := apiutil.NewDynamicRESTMapper(testCfg, httpClient)
	if err != nil {
		t.Fatalf("build rest mapper: %v", err)
	}

	cl, err := client.NewWithWatch(testCfg, client.Options{Scheme: testScheme})
	if err != nil {
		t.Fatalf("build client: %v", err)
	}

	reader, err := NewReader(cl, restMapper, testCfg, testScheme)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tablecache-watch"}}
	if err := cl.Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}

	podGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	watcher, err := reader.Watch(watchCtx, NewRowList(podGVK), client.InNamespace(ns.Name))
	if err != nil {
		t.Fatalf("watch rows: %v", err)
	}
	defer watcher.Stop()

	pod := &corev1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: "pod-watch", Namespace: ns.Name},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "pause", Image: "pause"}}},
	}
	if err := cl.Create(ctx, pod); err != nil {
		t.Fatalf("create pod: %v", err)
	}

	expectedColumns := []string{"Name", "Ready", "Status", "Restarts"}

	timeout := time.After(10 * time.Second)
	for {
		select {
		case evt, ok := <-watcher.ResultChan():
			if !ok {
				t.Fatal("watch channel closed before event")
			}
			if evt.Object == nil {
				continue
			}
			if status, ok := evt.Object.(*metav1.Status); ok {
				if strings.Contains(status.Message, "unable to decode an event from the watch stream") {
					continue
				}
				t.Fatalf("watch error: %v", status)
			}
			row, ok := evt.Object.(*Row)
			if !ok {
				continue
			}
			if row.Name == pod.Name {
				if evt.Type != watch.Added {
					t.Fatalf("expected Added event, got %v", evt.Type)
				}
				if row.TableTarget() != podGVK {
					t.Fatalf("row target = %v, want %v", row.TableTarget(), podGVK)
				}
				if len(row.Columns) < len(expectedColumns) {
					t.Fatalf("watch row has %d columns, want >= %d", len(row.Columns), len(expectedColumns))
				}
				for i, name := range expectedColumns {
					if row.Columns[i].Name != name {
						t.Fatalf("watch row column[%d] = %s, want %s", i, row.Columns[i].Name, name)
					}
				}
				if len(row.TableRow.Cells) < len(expectedColumns) {
					t.Fatalf("watch row cells length = %d, want >= %d", len(row.TableRow.Cells), len(expectedColumns))
				}
				expectedValues := []string{pod.Name, "0/1", "Pending", "0"}
				for i, want := range expectedValues {
					if fmt.Sprint(row.TableRow.Cells[i]) != want {
						t.Fatalf("watch row cell[%d] = %v, want %s", i, row.TableRow.Cells[i], want)
					}
				}
				return
			}
		case <-timeout:
			t.Fatal("timed out waiting for watch event")
		}
	}
}
