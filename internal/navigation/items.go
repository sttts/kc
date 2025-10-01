package navigation

import (
	"context"
	"sync"

	lipgloss "github.com/charmbracelet/lipgloss/v2"
	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	toolscache "k8s.io/client-go/tools/cache"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
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

func (r *RowItem) Details() string { return r.details }
func (r *RowItem) Path() []string  { return append([]string(nil), r.path...) }

// SimpleItem is retained for transitional compatibility; it embeds RowItem and can optionally expose view content.
type SimpleItem struct {
	*RowItem
	viewFn ViewContentFunc
}

var _ Item = (*SimpleItem)(nil)

func NewSimpleItem(id string, cells []string, path []string, style *lipgloss.Style) *SimpleItem {
	return &SimpleItem{RowItem: newRowItem(id, cells, path, style)}
}

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

// ObjectWithChildItem embeds an object row and adds Enter support for child folders.
type ObjectWithChildItem struct {
	*ObjectRow
	enter func() (Folder, error)
}

var _ Enterable = (*ObjectWithChildItem)(nil)

func newObjectWithChildItem(obj *ObjectRow, enter func() (Folder, error)) *ObjectWithChildItem {
	return &ObjectWithChildItem{ObjectRow: obj, enter: enter}
}

func (o *ObjectWithChildItem) Enter() (Folder, error) {
	if o.enter == nil {
		return nil, nil
	}
	return o.enter()
}

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
	enter    func() (Folder, error)
	contexts func() []string
}

var _ Enterable = (*ContextListItem)(nil)
var _ Countable = (*ContextListItem)(nil)

func newContextListItem(id string, cells []string, path []string, style *lipgloss.Style, contextsFn func() []string, enter func() (Folder, error)) *ContextListItem {
	return &ContextListItem{RowItem: newRowItem(id, cells, path, style), enter: enter, contexts: contextsFn}
}

func (c *ContextListItem) Enter() (Folder, error) {
	if c.enter == nil {
		return nil, nil
	}
	return c.enter()
}

func (c *ContextListItem) Count() int {
	if c.contexts == nil {
		return 0
	}
	return len(c.contexts())
}

func (c *ContextListItem) Empty() bool {
	if c.contexts == nil {
		return true
	}
	return len(c.contexts()) == 0
}

// ResourceGroupItem opens the object list for a specific resource and exposes aggregated counts.
type ResourceGroupItem struct {
	*RowItem
	enter     func() (Folder, error)
	deps      Deps
	gvr       schema.GroupVersionResource
	namespace string
	watchable bool

	mu         sync.Mutex
	count      int
	countKnown bool
	empty      bool
	emptyKnown bool
}

var _ Enterable = (*ResourceGroupItem)(nil)
var _ Countable = (*ResourceGroupItem)(nil)

func newResourceGroupItem(deps Deps, gvr schema.GroupVersionResource, namespace, id string, cells []string, path []string, style *lipgloss.Style, watchable bool, enter func() (Folder, error)) *ResourceGroupItem {
	return &ResourceGroupItem{
		RowItem:   newRowItem(id, cells, path, style),
		enter:     enter,
		deps:      deps,
		gvr:       gvr,
		namespace: namespace,
		watchable: watchable,
	}
}

func (r *ResourceGroupItem) Enter() (Folder, error) {
	if r.enter == nil {
		return nil, nil
	}
	return r.enter()
}

func (r *ResourceGroupItem) Count() int {
	if !r.watchable {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.countKnown {
		return r.count
	}
	if r.deps.Ctx != nil {
		logger := crlog.FromContext(r.deps.Ctx)
		logger.Info("initializing informer for resource count", "gvr", r.gvr.String(), "namespace", r.namespace)
	}
	count, ok := r.countFromInformerLocked()
	if ok {
		r.count = count
		r.countKnown = true
		if count == 0 {
			r.empty = true
			r.emptyKnown = true
		}
		return r.count
	}
	return 0
}

func (r *ResourceGroupItem) Empty() bool {
	if !r.watchable {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.emptyKnown {
		return r.empty
	}
	if r.deps.Ctx != nil {
		logger := crlog.FromContext(r.deps.Ctx)
		logger.Info("peeking resource emptiness", "gvr", r.gvr.String(), "namespace", r.namespace)
	}
	empty, ok := r.peekEmptyLocked()
	if ok {
		r.empty = empty
		r.emptyKnown = true
		if empty {
			r.count = 0
			r.countKnown = true
		}
		return r.empty
	}
	return false
}

func (r *ResourceGroupItem) countFromInformerLocked() (int, bool) {
	if r.deps.Cl == nil {
		return 0, false
	}
	ctx := r.deps.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	gvk, err := r.deps.Cl.RESTMapper().KindFor(r.gvr)
	if err != nil {
		return 0, false
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	informer, err := r.deps.Cl.GetCache().GetInformer(ctx, obj, crcache.BlockUntilSynced(true))
	if err != nil {
		return 0, false
	}
	if !informer.HasSynced() {
		toolscache.WaitForCacheSync(ctx.Done(), informer.HasSynced)
	}
	type storeInformer interface {
		GetStore() toolscache.Store
	}
	if si, ok := informer.(storeInformer); ok {
		items := si.GetStore().List()
		if r.namespace == "" {
			return len(items), true
		}
		count := 0
		for _, raw := range items {
			switch o := raw.(type) {
			case crclient.Object:
				if o.GetNamespace() == r.namespace {
					count++
				}
			case *unstructured.Unstructured:
				if o.GetNamespace() == r.namespace {
					count++
				}
			}
		}
		return count, true
	}
	// Fallback: cache-backed list
	ul := &unstructured.UnstructuredList{}
	ul.SetGroupVersionKind(schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind + "List"})
	opts := []crclient.ListOption{}
	if r.namespace != "" {
		opts = append(opts, crclient.InNamespace(r.namespace))
	}
	if err := r.deps.Cl.GetClient().List(ctx, ul, opts...); err != nil {
		return 0, false
	}
	return len(ul.Items), true
}

func (r *ResourceGroupItem) peekEmptyLocked() (bool, bool) {
	if r.deps.Cl == nil {
		return false, false
	}
	ctx := r.deps.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	has, err := r.deps.Cl.HasAnyByGVR(ctx, r.gvr, r.namespace)
	if err != nil {
		return false, false
	}
	return !has, true
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
