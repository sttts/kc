package tablecache

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
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

func TestReaderListCustomResourceColumnsEnvtest(t *testing.T) {
	if testCfg == nil {
		t.Skip("envtest not available")
	}

	ctx := t.Context()

	extClient, err := apiextensionsclientset.NewForConfig(testCfg)
	if err != nil {
		t.Fatalf("build apiextensions client: %v", err)
	}

	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "widgets.example.com",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "widgets",
				Singular: "widget",
				Kind:     "Widget",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    "v1alpha1",
				Served:  true,
				Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]apiextensionsv1.JSONSchemaProps{
									"flavor":  {Type: "string"},
									"enabled": {Type: "boolean"},
								},
							},
						},
					},
				},
				AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
					{Name: "Flavor", Type: "string", JSONPath: ".spec.flavor"},
					{Name: "Enabled", Type: "boolean", JSONPath: ".spec.enabled"},
				},
			}},
		},
	}

	if _, err := extClient.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, crd, metav1.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("create custom resource definition: %v", err)
	}

	waitErr := wait.PollImmediate(200*time.Millisecond, 10*time.Second, func() (bool, error) {
		current, err := extClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for _, cond := range current.Status.Conditions {
			if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
				return true, nil
			}
			if cond.Type == apiextensionsv1.NamesAccepted && cond.Status == apiextensionsv1.ConditionFalse {
				return false, fmt.Errorf("crd name not accepted: %s", cond.Message)
			}
		}
		return false, nil
	})
	if waitErr != nil {
		t.Fatalf("wait for crd established: %v", waitErr)
	}

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

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tablecache-crd"}}
	if err := cl.Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}

	dynClient, err := dynamic.NewForConfig(testCfg)
	if err != nil {
		t.Fatalf("build dynamic client: %v", err)
	}

	resource := schema.GroupVersionResource{Group: "example.com", Version: "v1alpha1", Resource: "widgets"}
	customObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "example.com/v1alpha1",
			"kind":       "Widget",
			"metadata": map[string]interface{}{
				"name":      "demo-widget",
				"namespace": ns.Name,
			},
			"spec": map[string]interface{}{
				"flavor":  "vanilla",
				"enabled": true,
			},
		},
	}

	createErr := wait.PollImmediate(200*time.Millisecond, 10*time.Second, func() (bool, error) {
		_, err := dynClient.Resource(resource).Namespace(ns.Name).Create(ctx, customObj.DeepCopy(), metav1.CreateOptions{})
		switch {
		case err == nil, errors.IsAlreadyExists(err):
			return true, nil
		case errors.IsNotFound(err):
			return false, nil
		default:
			return false, err
		}
	})
	if createErr != nil {
		t.Fatalf("create custom resource: %v", createErr)
	}

	reader, err := NewReader(cl, restMapper, testCfg, testScheme)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	widgetGVK := schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "Widget"}
	list := NewRowList(widgetGVK)
	if err := reader.List(ctx, list, client.InNamespace(ns.Name)); err != nil {
		t.Fatalf("list custom resource rows: %v", err)
	}

	if list.TableTarget() != widgetGVK {
		t.Fatalf("list target = %v, want %v", list.TableTarget(), widgetGVK)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 custom resource row, got %d", len(list.Items))
	}

	row := list.Items[0]
	if row.Name != "demo-widget" {
		t.Fatalf("row name = %s, want demo-widget", row.Name)
	}
	if row.Namespace != ns.Name {
		t.Fatalf("row namespace = %s, want %s", row.Namespace, ns.Name)
	}
	if row.TableTarget() != widgetGVK {
		t.Fatalf("row target = %v, want %v", row.TableTarget(), widgetGVK)
	}

	expected := map[string]string{
		"Flavor":  "vanilla",
		"Enabled": "true",
	}

	for column, want := range expected {
		index := -1
		for i, col := range list.Columns {
			if col.Name == column {
				index = i
				break
			}
		}
		if index == -1 {
			t.Fatalf("missing column %q", column)
		}
		if len(row.TableRow.Cells) <= index {
			t.Fatalf("row cells length = %d, want > %d", len(row.TableRow.Cells), index)
		}
		if got := fmt.Sprint(row.TableRow.Cells[index]); got != want {
			t.Fatalf("row column %q value = %s, want %s", column, got, want)
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
