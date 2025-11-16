package models

import (
	"encoding/json"
	"testing"

	"github.com/sttts/kc/internal/tablecache"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestRowStylerReadyCondition(t *testing.T) {
	pod := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Ready", "status": "False"},
				},
			},
		},
	}
	info := RowStyleInfo{
		GVR:          schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		GVK:          schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		ObjectMeta:   metav1.ObjectMeta{Name: "demo"},
		Unstructured: pod,
		BaseStyle:    WhiteStyle(),
	}
	style := applyRowStylers(info)
	if style == info.BaseStyle {
		t.Fatalf("expected ready styler to override base style")
	}
	if style != NotReadyStyle() {
		t.Fatalf("expected NotReadyStyle pointer")
	}
}

func TestRowStylerJobStatus(t *testing.T) {
	job := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"status": map[string]interface{}{
				"succeeded": int64(1),
			},
		},
	}
	info := RowStyleInfo{
		GVR:          schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"},
		GVK:          schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"},
		ObjectMeta:   metav1.ObjectMeta{Name: "job"},
		Unstructured: job,
		BaseStyle:    WhiteStyle(),
	}
	if got := applyRowStylers(info); got != JobSuccessStyle() {
		t.Fatalf("expected success style for succeeded job")
	}
	job.Object["status"] = map[string]interface{}{"failed": int64(5)}
	job.Object["spec"] = map[string]interface{}{"backoffLimit": int64(3)}
	if got := applyRowStylers(info); got != JobFailedStyle() {
		t.Fatalf("expected failure style when backoff exceeded")
	}
	job.Object["status"] = map[string]interface{}{"failed": int64(1)}
	if got := applyRowStylers(info); got != info.BaseStyle {
		t.Fatalf("expected base style while job still retrying")
	}
}

func TestUnstructuredFromRow(t *testing.T) {
	row := &tablecache.Row{}
	pod := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "table-pod",
			"namespace": "default",
		},
	}
	raw, err := json.Marshal(pod)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	row.Object.Raw = raw
	row.SetTableTarget(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
	u := unstructuredFromRow(row)
	if u == nil || u.GetName() != "table-pod" {
		t.Fatalf("expected decoded pod, got %#v", u)
	}
	info := RowStyleInfo{
		GVR:          schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		GVK:          row.TableTarget(),
		ObjectMeta:   metav1.ObjectMeta{Name: "table-pod"},
		Unstructured: u,
		BaseStyle:    WhiteStyle(),
	}
	if style := applyRowStylers(info); style == nil {
		t.Fatalf("expected a style result")
	}
}
