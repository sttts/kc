package models

import (
	"encoding/json"
	"sync"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sttts/kc/internal/tablecache"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// RowStyleInfo carries the metadata available when styling a row.
type RowStyleInfo struct {
	GVR          schema.GroupVersionResource
	GVK          schema.GroupVersionKind
	ObjectMeta   metav1.ObjectMeta
	Unstructured *unstructured.Unstructured
	BaseStyle    *lipgloss.Style
}

// RowStyler can override the base style for a given row.
type RowStyler interface {
	Style(RowStyleInfo) *lipgloss.Style
}

type rowStylerFunc func(RowStyleInfo) *lipgloss.Style

func (f rowStylerFunc) Style(info RowStyleInfo) *lipgloss.Style { return f(info) }

var (
	stylerMu        sync.RWMutex
	stylersByGVR    = make(map[schema.GroupVersionResource][]RowStyler)
	stylersByGVK    = make(map[schema.GroupVersionKind][]RowStyler)
	globalStylers   []RowStyler
	defaultStylers  sync.Once
	unstructuredDec = unstructured.UnstructuredJSONScheme
)

// RegisterRowStylerForGVR installs a styler for the exact GVR.
func RegisterRowStylerForGVR(gvr schema.GroupVersionResource, styler RowStyler) {
	stylerMu.Lock()
	defer stylerMu.Unlock()
	stylersByGVR[gvr] = append(stylersByGVR[gvr], styler)
}

// RegisterRowStylerForGVK installs a styler for the exact GVK.
func RegisterRowStylerForGVK(gvk schema.GroupVersionKind, styler RowStyler) {
	stylerMu.Lock()
	defer stylerMu.Unlock()
	stylersByGVK[gvk] = append(stylersByGVK[gvk], styler)
}

// RegisterGlobalRowStyler installs a styler invoked for every row.
func RegisterGlobalRowStyler(styler RowStyler) {
	stylerMu.Lock()
	defer stylerMu.Unlock()
	globalStylers = append(globalStylers, styler)
}

// applyRowStylers resolves all matching stylers and applies them sequentially.
func applyRowStylers(info RowStyleInfo) *lipgloss.Style {
	defaultStylers.Do(registerDefaultRowStylers)

	if info.BaseStyle == nil {
		info.BaseStyle = WhiteStyle()
	}
	style := info.BaseStyle

	stylerMu.RLock()
	defer stylerMu.RUnlock()
	seq := make([]RowStyler, 0, len(globalStylers)+len(stylersByGVR[info.GVR])+len(stylersByGVK[info.GVK]))
	seq = append(seq, globalStylers...)
	if len(info.GVR.Resource) > 0 {
		if list := stylersByGVR[info.GVR]; len(list) > 0 {
			seq = append(seq, list...)
		}
	}
	if len(info.GVK.Kind) > 0 {
		if list := stylersByGVK[info.GVK]; len(list) > 0 {
			seq = append(seq, list...)
		}
	}
	for _, styler := range seq {
		if styler == nil {
			continue
		}
		info.BaseStyle = style
		if next := styler.Style(info); next != nil {
			style = next
		}
	}
	return style
}

func registerDefaultRowStylers() {
	RegisterGlobalRowStyler(rowStylerFunc(func(info RowStyleInfo) *lipgloss.Style {
		if info.ObjectMeta.DeletionTimestamp != nil && !info.ObjectMeta.DeletionTimestamp.IsZero() {
			return DeletingStyle()
		}
		return info.BaseStyle
	}))
	registerReadyStylers()
	registerJobStyler()
	registerWorkloadStylers()
}

func registerReadyStylers() {
	styler := rowStylerFunc(func(info RowStyleInfo) *lipgloss.Style {
		if info.Unstructured == nil {
			return info.BaseStyle
		}
		status := readinessStatus(info.Unstructured, "Ready")
		if status == conditionFalse {
			return NotReadyStyle()
		}
		return info.BaseStyle
	})

	RegisterRowStylerForGVR(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, styler)
	RegisterRowStylerForGVR(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}, styler)
	RegisterRowStylerForGVR(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, styler)
}

func registerJobStyler() {
	styler := rowStylerFunc(func(info RowStyleInfo) *lipgloss.Style {
		if info.Unstructured == nil {
			return info.BaseStyle
		}
		succeeded, _, _ := unstructured.NestedInt64(info.Unstructured.Object, "status", "succeeded")
		failed, _, _ := unstructured.NestedInt64(info.Unstructured.Object, "status", "failed")
		backoffLimit, _, _ := unstructured.NestedInt64(info.Unstructured.Object, "spec", "backoffLimit")
		if succeeded > 0 {
			return JobSuccessStyle()
		}
		if failed > 0 && backoffLimit > 0 && failed >= backoffLimit {
			return JobFailedStyle()
		}
		return info.BaseStyle
	})
	RegisterRowStylerForGVR(schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, styler)
}

func registerWorkloadStylers() {
	deployStyler := rowStylerFunc(func(info RowStyleInfo) *lipgloss.Style {
		if info.Unstructured == nil {
			return info.BaseStyle
		}
		if deploymentNotReady(info.Unstructured) {
			return NotReadyStyle()
		}
		return info.BaseStyle
	})
	RegisterRowStylerForGVR(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, deployStyler)

	dsStyler := rowStylerFunc(func(info RowStyleInfo) *lipgloss.Style {
		if info.Unstructured == nil {
			return info.BaseStyle
		}
		desired, _, _ := unstructured.NestedInt64(info.Unstructured.Object, "status", "desiredNumberScheduled")
		ready, _, _ := unstructured.NestedInt64(info.Unstructured.Object, "status", "numberReady")
		available, _, _ := unstructured.NestedInt64(info.Unstructured.Object, "status", "numberAvailable")
		if desired > 0 && (ready < desired || available < desired) {
			return NotReadyStyle()
		}
		if readinessStatus(info.Unstructured, "NumberAvailable") == conditionFalse {
			return NotReadyStyle()
		}
		return info.BaseStyle
	})
	RegisterRowStylerForGVR(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, dsStyler)
}

func deploymentNotReady(u *unstructured.Unstructured) bool {
	if u == nil {
		return false
	}
	ready, _, _ := unstructured.NestedInt64(u.Object, "status", "readyReplicas")
	available, _, _ := unstructured.NestedInt64(u.Object, "status", "availableReplicas")
	replicas, _, _ := unstructured.NestedInt64(u.Object, "spec", "replicas")
	if readinessStatus(u, "Available") == conditionFalse {
		return true
	}
	if replicas > 0 {
		if ready < replicas || available < replicas {
			return true
		}
	}
	return false
}

type conditionState string

const (
	conditionTrue  conditionState = "True"
	conditionFalse conditionState = "False"
)

func readinessStatus(u *unstructured.Unstructured, condType string) conditionState {
	if u == nil {
		return ""
	}
	conds, found, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	if !found {
		return ""
	}
	for _, raw := range conds {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t != condType {
			continue
		}
		if s, _ := m["status"].(string); s != "" {
			return conditionState(s)
		}
	}
	return ""
}

func unstructuredFromRow(row *tablecache.Row) *unstructured.Unstructured {
	if row == nil || len(row.Object.Raw) == 0 {
		return nil
	}
	obj, err := runtime.Decode(unstructuredDec, row.Object.Raw)
	if err != nil {
		// try generic map decoding as a fallback
		var data map[string]interface{}
		if err := json.Unmarshal(row.Object.Raw, &data); err != nil {
			return nil
		}
		return &unstructured.Unstructured{Object: data}
	}
	if u, ok := obj.(*unstructured.Unstructured); ok {
		return u
	}
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil
	}
	return &unstructured.Unstructured{Object: data}
}
