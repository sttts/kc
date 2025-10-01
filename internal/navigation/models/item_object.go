package models

import (
	"github.com/charmbracelet/lipgloss/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ObjectRow models a concrete Kubernetes object row.
type ObjectRow struct {
	*RowItem
	gvr       schema.GroupVersionResource
	namespace string
	name      string
	viewFn    ViewContentFunc
}

func NewObjectRow(id string, cells []string, path []string, gvr schema.GroupVersionResource, namespace, name string, style *lipgloss.Style) *ObjectRow {
	return &ObjectRow{
		RowItem:   NewRowItem(id, cells, path, style),
		gvr:       gvr,
		namespace: namespace,
		name:      name,
	}
}

func (o *ObjectRow) GVR() schema.GroupVersionResource { return o.gvr }
func (o *ObjectRow) Namespace() string                { return o.namespace }
func (o *ObjectRow) Name() string                     { return o.name }

func (o *ObjectRow) WithViewContent(fn ViewContentFunc) *ObjectRow {
	o.viewFn = fn
	return o
}

func (o *ObjectRow) ViewContent() (string, string, string, string, string, error) {
	if o.viewFn == nil {
		return "", "", "", "", "", ErrNoViewContent
	}
	return o.viewFn()
}
