package navigation

import (
	lipgloss "github.com/charmbracelet/lipgloss/v2"
	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// RowItem is the minimal table-backed item implementation shared by all rows.
type RowItem struct {
	table.SimpleRow
	details string
	path    []string
}

func newRowItem(id string, cells []string, path []string, style *lipgloss.Style) *RowItem {
	if style == nil {
		style = GreenStyle()
	}
	styles := make([]*lipgloss.Style, len(cells))
	for i := range styles {
		styles[i] = style
	}
	return newRowItemStyled(id, cells, path, styles)
}

func newRowItemStyled(id string, cells []string, path []string, styles []*lipgloss.Style) *RowItem {
	cloned := make([]string, len(cells))
	copy(cloned, cells)
	sty := make([]*lipgloss.Style, len(styles))
	copy(sty, styles)
	return &RowItem{
		SimpleRow: table.SimpleRow{ID: id, Cells: cloned, Styles: sty},
		path:      append([]string(nil), path...),
	}
}

func (r *RowItem) Details() string               { return r.details }
func (r *RowItem) WithDetails(d string) *RowItem { r.details = d; return r }
func (r *RowItem) Path() []string                { return append([]string(nil), r.path...) }

// SimpleItem is retained for transitional compatibility; it embeds RowItem and can optionally expose view content.
type SimpleItem struct {
	*RowItem
	viewFn ViewContentFunc
}

var _ Item = (*SimpleItem)(nil)

func NewSimpleItem(id string, cells []string, path []string, style *lipgloss.Style) *SimpleItem {
	return &SimpleItem{RowItem: newRowItem(id, cells, path, style)}
}

func (s *SimpleItem) WithDetails(d string) *SimpleItem { s.RowItem.WithDetails(d); return s }

func (s *SimpleItem) WithViewContent(fn ViewContentFunc) *SimpleItem {
	s.viewFn = fn
	return s
}

func (s *SimpleItem) ViewContent() (string, string, string, string, string, error) {
	if s.viewFn == nil {
		return "", "", "", "", "", ErrNoViewContent
	}
	return s.viewFn()
}

// ObjectRow models a concrete Kubernetes object row.
type ObjectRow struct {
	*RowItem
	gvr       schema.GroupVersionResource
	namespace string
	name      string
	viewFn    ViewContentFunc
}

var _ Item = (*ObjectRow)(nil)
var _ ObjectItem = (*ObjectRow)(nil)

func newObjectRow(id string, cells []string, path []string, gvr schema.GroupVersionResource, namespace, name string, style *lipgloss.Style) *ObjectRow {
	return &ObjectRow{
		RowItem:   newRowItem(id, cells, path, style),
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

// NamespaceItem embeds an ObjectRow and adds Enter support.
type NamespaceItem struct {
	*ObjectRow
	enter func() (Folder, error)
}

var _ Enterable = (*NamespaceItem)(nil)

func newNamespaceItem(obj *ObjectRow, enter func() (Folder, error)) *NamespaceItem {
	return &NamespaceItem{ObjectRow: obj, enter: enter}
}

func (n *NamespaceItem) Enter() (Folder, error) {
	if n.enter == nil {
		return nil, nil
	}
	return n.enter()
}

// PodItem, ConfigMapItem, SecretItem are viewable object rows without extra behaviour.
type PodItem struct{ *ObjectRow }
type ConfigMapItem struct{ *ObjectRow }
type SecretItem struct{ *ObjectRow }

// ContextItem represents a kubeconfig context entry, viewable and enterable.
type ContextItem struct {
	*RowItem
	enter  func() (Folder, error)
	viewFn ViewContentFunc
}

var _ Enterable = (*ContextItem)(nil)

func newContextItem(id string, cells []string, path []string, style *lipgloss.Style, enter func() (Folder, error)) *ContextItem {
	return &ContextItem{RowItem: newRowItem(id, cells, path, style), enter: enter}
}

func (c *ContextItem) Enter() (Folder, error) {
	if c.enter == nil {
		return nil, nil
	}
	return c.enter()
}

func (c *ContextItem) WithViewContent(fn ViewContentFunc) *ContextItem {
	c.viewFn = fn
	return c
}

func (c *ContextItem) ViewContent() (string, string, string, string, string, error) {
	if c.viewFn == nil {
		return "", "", "", "", "", ErrNoViewContent
	}
	return c.viewFn()
}

// ContextListItem lists contexts and is enterable only.
type ContextListItem struct {
	*RowItem
	enter func() (Folder, error)
}

var _ Enterable = (*ContextListItem)(nil)

func newContextListItem(id string, cells []string, path []string, style *lipgloss.Style, enter func() (Folder, error)) *ContextListItem {
	return &ContextListItem{RowItem: newRowItem(id, cells, path, style), enter: enter}
}

func (c *ContextListItem) Enter() (Folder, error) {
	if c.enter == nil {
		return nil, nil
	}
	return c.enter()
}

// ResourceGroupItem opens the object list for a specific resource.
type ResourceGroupItem struct {
	*RowItem
	enter func() (Folder, error)
}

var _ Enterable = (*ResourceGroupItem)(nil)

func newResourceGroupItem(id string, cells []string, path []string, style *lipgloss.Style, enter func() (Folder, error)) *ResourceGroupItem {
	return &ResourceGroupItem{RowItem: newRowItem(id, cells, path, style), enter: enter}
}

func (r *ResourceGroupItem) Enter() (Folder, error) {
	if r.enter == nil {
		return nil, nil
	}
	return r.enter()
}

// ConfigKeyItem exposes a ConfigMap/Secret key value.
type ConfigKeyItem struct {
	*RowItem
	viewFn ViewContentFunc
}

func newConfigKeyItem(id string, cells []string, path []string, style *lipgloss.Style, viewFn ViewContentFunc) *ConfigKeyItem {
	return &ConfigKeyItem{RowItem: newRowItem(id, cells, path, style), viewFn: viewFn}
}

func (c *ConfigKeyItem) ViewContent() (string, string, string, string, string, error) {
	if c.viewFn == nil {
		return "", "", "", "", "", ErrNoViewContent
	}
	return c.viewFn()
}

// ContainerItem shows a pod container or initContainer spec.
type ContainerItem struct {
	*RowItem
	viewFn ViewContentFunc
}

func newContainerItem(id string, cells []string, path []string, style *lipgloss.Style, viewFn ViewContentFunc) *ContainerItem {
	return &ContainerItem{RowItem: newRowItem(id, cells, path, style), viewFn: viewFn}
}

func (c *ContainerItem) ViewContent() (string, string, string, string, string, error) {
	if c.viewFn == nil {
		return "", "", "", "", "", ErrNoViewContent
	}
	return c.viewFn()
}

// BackItem renders the ".." entry and marks a back action.
type BackItem struct{}

var _ Back = (*BackItem)(nil)
var _ Item = (*BackItem)(nil)

func (b BackItem) Columns() (string, []string, []*lipgloss.Style, bool) {
	s := GreenStyle()
	return "__back__", []string{".."}, []*lipgloss.Style{s}, true
}

func (b BackItem) Details() string { return "Back" }
func (b BackItem) Path() []string  { return nil }
