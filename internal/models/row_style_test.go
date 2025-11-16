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

func TestDeploymentStyler(t *testing.T) {
	deploy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"spec": map[string]interface{}{
				"replicas": int64(3),
			},
			"status": map[string]interface{}{
				"readyReplicas":     int64(2),
				"availableReplicas": int64(2),
			},
		},
	}
	info := RowStyleInfo{
		GVR:          schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		GVK:          schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		ObjectMeta:   metav1.ObjectMeta{Name: "dep"},
		Unstructured: deploy,
		BaseStyle:    WhiteStyle(),
	}
	if got := applyRowStylers(info); got != NotReadyStyle() {
		t.Fatalf("expected deployment to be marked not ready")
	}
	deploy.Object["status"] = map[string]interface{}{
		"readyReplicas":     int64(3),
		"availableReplicas": int64(3),
		"conditions": []interface{}{
			map[string]interface{}{"type": "Available", "status": "True"},
		},
	}
	if got := applyRowStylers(info); got != info.BaseStyle {
		t.Fatalf("expected ready deployment to keep base style")
	}
}

func TestDaemonSetStyler(t *testing.T) {
	ds := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "DaemonSet",
			"status": map[string]interface{}{
				"desiredNumberScheduled": int64(4),
				"numberReady":            int64(3),
				"numberAvailable":        int64(3),
			},
		},
	}
	info := RowStyleInfo{
		GVR:          schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"},
		GVK:          schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"},
		ObjectMeta:   metav1.ObjectMeta{Name: "ds"},
		Unstructured: ds,
		BaseStyle:    WhiteStyle(),
	}
	if got := applyRowStylers(info); got != NotReadyStyle() {
		t.Fatalf("expected daemonset to be marked not ready when ready<desired")
	}
	ds.Object["status"] = map[string]interface{}{
		"desiredNumberScheduled": int64(3),
		"numberReady":            int64(3),
		"numberAvailable":        int64(3),
	}
	if got := applyRowStylers(info); got != info.BaseStyle {
		t.Fatalf("expected daemonset to keep base style once satisfied")
	}
}
