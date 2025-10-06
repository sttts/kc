package models

import (
	"context"
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
			if onUpdate != nil {
				onUpdate()
			}
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
	cfg := r.deps.AppConfig
	return r.EmptyWithin(cfg.Resources.PeekInterval.Duration)
}

func (r *ResourceGroupItem) EmptyWithin(interval time.Duration) bool {
	if !r.watchable {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.emptyKnown && !r.lastPeek.IsZero() && time.Since(r.lastPeek) < interval {
		return r.empty
	}
	if r.deps.Ctx != nil {
		crlog.FromContext(r.deps.Ctx).Info("peeking resource emptiness", "gvr", r.gvr.String(), "namespace", r.namespace)
	}
	empty, ok := r.peekEmptyLocked()
	r.lastPeek = time.Now()
	if ok {
		r.empty = empty
		r.emptyKnown = true
		if empty {
			r.count = 0
			r.countKnown = true
		}
		return r.empty
	}
	return r.empty
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
		if apierrors.IsMethodNotSupported(err) {
			if r.deps.Ctx != nil {
				crlog.FromContext(r.deps.Ctx).Info("resource watch not supported; skipping informer", "gvr", r.gvr.String(), "namespace", r.namespace)
			}
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
