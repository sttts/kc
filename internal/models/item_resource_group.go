package models

import (
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	toolscache "k8s.io/client-go/tools/cache"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
)

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
	countOnce  sync.Once
	lastPeek   time.Time
	onChange   func()

	publishedCount      int
	publishedCountKnown bool
	publishedEmpty      bool
	publishedEmptyKnown bool
}

func NewResourceGroupItem(deps Deps, gvr schema.GroupVersionResource, namespace, id string, cells []string, path []string, style *lipgloss.Style, watchable bool, enter func() (Folder, error)) *ResourceGroupItem {
	return &ResourceGroupItem{
		RowItem:   NewRowItem(id, cells, path, style),
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

// ComputeCountAsync triggers Count() on a background goroutine and invokes the
// provided callback once the count is known.
func (r *ResourceGroupItem) ComputeCountAsync(onUpdate func()) {
	if !r.watchable {
		return
	}
	r.countOnce.Do(func() {
		go func() {
			_ = r.Count()
			r.notifyIfChanged(onUpdate)
		}()
	})
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
	logger := crlog.FromContext(r.deps.Ctx)
	logger.Info("initializing informer for resource count", "gvr", r.gvr.String(), "namespace", r.namespace)
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
	cfg := r.deps.AppConfig
	return r.emptyWithin(cfg.Resources.PeekInterval.Duration)
}

func (r *ResourceGroupItem) emptyWithin(interval time.Duration) bool {
	if !r.watchable {
		return true
	}
	r.mu.Lock()
	if r.emptyKnown && !r.lastPeek.IsZero() && time.Since(r.lastPeek) < interval {
		val := r.empty
		r.mu.Unlock()
		return val
	}
	crlog.FromContext(r.deps.Ctx).Info("peeking resource emptiness", "gvr", r.gvr.String(), "namespace", r.namespace)
	empty, ok := r.peekEmptyLocked()
	r.lastPeek = time.Now()
	if ok {
		r.empty = empty
		r.emptyKnown = true
		if empty {
			r.count = 0
			r.countKnown = true
		}
		changed := r.recordPublishedLocked()
		onChange := r.onChange
		val := r.empty
		r.mu.Unlock()
		if changed && onChange != nil {
			onChange()
		}
		return val
	}
	val := r.empty
	r.mu.Unlock()
	return val
}

func (r *ResourceGroupItem) TryCount() (int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.countKnown {
		return 0, false
	}
	return r.count, true
}

func (r *ResourceGroupItem) countFromInformerLocked() (int, bool) {
	ctx := r.deps.Ctx
	gvk, err := r.deps.Cl.RESTMapper().KindFor(r.gvr)
	if err != nil {
		return 0, false
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	informer, err := r.deps.Cl.GetCache().GetInformer(ctx, obj, crcache.BlockUntilSynced(true))
	if err != nil {
		if apierrors.IsMethodNotSupported(err) {
			crlog.FromContext(r.deps.Ctx).Info("resource watch not supported; skipping informer", "gvr", r.gvr.String(), "namespace", r.namespace)
			r.watchable = false
			return 0, true
		}
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
	if err := r.deps.Cl.GetCache().List(ctx, ul, opts...); err != nil {
		return 0, false
	}
	return len(ul.Items), true
}

func (r *ResourceGroupItem) peekEmptyLocked() (bool, bool) {
	ctx := r.deps.Ctx
	has, err := r.deps.Cl.HasAnyByGVR(ctx, r.gvr, r.namespace)
	if err != nil {
		return false, false
	}
	return !has, true
}

func (r *ResourceGroupItem) String() string {
	return fmt.Sprintf("%s/%s", r.gvr.Resource, r.namespace)
}

func (r *ResourceGroupItem) ID() string {
	if r == nil || r.RowItem == nil {
		return ""
	}
	return r.RowItem.ID()
}

func (r *ResourceGroupItem) CopyFrom(other *ResourceGroupItem) {
	if r == nil || other == nil {
		return
	}
	if r.RowItem == nil && other.RowItem != nil {
		r.RowItem = NewRowItem(other.RowItem.ID(), nil, nil, nil)
	}
	if r.RowItem != nil && other.RowItem != nil {
		r.RowItem.copyFrom(other.RowItem)
	}
	r.enter = other.enter
	r.deps = other.deps
	r.gvr = other.gvr
	r.namespace = other.namespace
	r.watchable = other.watchable
}

func (r *ResourceGroupItem) applySpec(spec resourceGroupSpec, deps Deps, created bool) {
	if r == nil {
		return
	}
	if r.RowItem == nil {
		r.RowItem = NewRowItem(spec.id, spec.cells, spec.path, spec.style)
	} else {
		r.RowItem.reset(spec.id, spec.cells, spec.path, spec.style)
	}
	r.enter = spec.enter
	r.deps = deps
	r.gvr = spec.gvr
	r.namespace = spec.namespace
	switch {
	case created:
		r.watchable = spec.watchable
	case !r.watchable:
		// preserve previous disabled state
	case !spec.watchable:
		r.watchable = false
	default:
		r.watchable = spec.watchable
	}
}

func (r *ResourceGroupItem) setCountCell(value string) {
	if r == nil || r.RowItem == nil {
		return
	}
	r.RowItem.SimpleRow.SetColumn(2, value, nil)
}

func (r *ResourceGroupItem) SetOnChange(fn func()) {
	r.mu.Lock()
	r.onChange = fn
	r.mu.Unlock()
}

func (r *ResourceGroupItem) notifyIfChanged(onUpdate func()) {
	r.mu.Lock()
	changed := r.recordPublishedLocked()
	onChange := r.onChange
	r.mu.Unlock()
	if changed {
		if onChange != nil {
			onChange()
		}
		if onUpdate != nil {
			onUpdate()
		}
	}
}

func (r *ResourceGroupItem) recordPublishedLocked() bool {
	changed := false
	if r.countKnown {
		if !r.publishedCountKnown || r.count != r.publishedCount {
			r.publishedCountKnown = true
			r.publishedCount = r.count
			changed = true
		}
	}
	if r.emptyKnown {
		if !r.publishedEmptyKnown || r.empty != r.publishedEmpty {
			r.publishedEmptyKnown = true
			r.publishedEmpty = r.empty
			changed = true
		}
	}
	return changed
}
