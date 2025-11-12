package models

import (
	"strings"

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
	verbs     []string
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

// SetResourceVerbs stores the server-supported verbs for the underlying resource.
func (o *ObjectRow) SetResourceVerbs(verbs []string) {
	o.verbs = append([]string(nil), verbs...)
}

// SupportsVerb reports whether the resource exposes the given verb.
func (o *ObjectRow) SupportsVerb(verb string) bool {
	for _, v := range o.verbs {
		if strings.EqualFold(v, verb) {
			return true
		}
	}
	return false
}
