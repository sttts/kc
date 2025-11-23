package models

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
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
	lastError  time.Time
	onChange   func()

	publishedCount      int
	publishedCountKnown bool
	publishedEmpty      bool
	publishedEmptyKnown bool
	recounting          bool // true while an async count recomputation is in flight
	watchOnce           sync.Once
	nextPeekScheduled   bool
}

func NewResourceGroupItem(deps Deps, gvr schema.GroupVersionResource, namespace, id string, cells []string, path []string, detail string, style *lipgloss.Style, watchable bool, enter func() (Folder, error)) *ResourceGroupItem {
	row := NewRowItem(id, cells, path, style)
	row.details = detail
	return &ResourceGroupItem{
		RowItem:   row,
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
	logger.Info("ensuring informer for resource count", "gvr", r.gvr.String(), "namespace", r.namespace)
	count, ok := r.countFromInformerLocked()
	if ok {
		r.count = count
		r.countKnown = true
		if count == 0 {
			r.empty = true
			r.emptyKnown = true
		} else {
			r.empty = false
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
	if time.Since(r.lastError) < interval {
		// Back off when the last peek errored; skip hitting the API until interval elapses.
		r.mu.Unlock()
		return r.empty
	}
	crlog.FromContext(r.deps.Ctx).Info("peeking resource emptiness", "gvr", r.gvr.String(), "namespace", r.namespace)
	empty, ok := r.peekEmptyLocked()
	prevEmpty := r.emptyKnown && r.empty
	r.lastPeek = time.Now()
	if ok {
		r.empty = empty
		r.emptyKnown = true
		needRecount := false
		if empty {
			r.count = 0
			r.countKnown = true
		} else if prevEmpty {
			r.countKnown = false
			r.empty = false
			r.emptyKnown = true
			needRecount = true
		}
		changed := r.recordPublishedLocked()
		onChange := r.onChange
		val := r.empty
		r.mu.Unlock()
		if changed && onChange != nil {
			onChange()
		}
		if needRecount {
			go r.scheduleRecount()
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
			crlog.FromContext(r.deps.Ctx).Info("resource watch not supported; skipping informer", "gvr", r.gvr.String(), "namespace", r.namespace)
			r.watchable = false
			return 0, true
		}
		return 0, false
	}
	r.ensureInformerHandler(informer)
	if !informer.HasSynced() {
		toolscache.WaitForCacheSync(ctx.Done(), informer.HasSynced)
	}
	type storeInformer interface{ GetStore() toolscache.Store }
	type indexerInformer interface{ GetIndexer() toolscache.Indexer }
	switch {
	case func() bool { _, ok := informer.(storeInformer); return ok }():
		items := informer.(storeInformer).GetStore().List()
		return r.countObjects(items), true
	case func() bool { _, ok := informer.(indexerInformer); return ok }():
		items := informer.(indexerInformer).GetIndexer().List()
		return r.countObjects(items), true
	default:
		// Fallback to a lightweight client peek if informer/store not available.
		hasAny := true
		if ok, err := r.deps.Cl.HasAnyByGVR(ctx, r.gvr, r.namespace); err == nil {
			hasAny = ok
		} else {
			crlog.FromContext(r.deps.Ctx).Error(err, "hasAny peek failed", "gvr", r.gvr.String(), "namespace", r.namespace)
			r.lastPeek = time.Now()
			r.lastError = time.Now()
			r.scheduleNextPeekLocked()
			return 0, false
		}
		if !hasAny {
			r.scheduleNextPeekLocked()
			return 0, true
		}
		return r.countViaClient(ctx)
	}
}

func (r *ResourceGroupItem) countObjects(items []interface{}) int {
	if items == nil {
		return 0
	}
	if r.namespace == "" {
		return len(items)
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
	return count
}

func (r *ResourceGroupItem) countViaClient(ctx context.Context) (int, bool) {
	list, err := r.deps.Cl.ListByGVR(ctx, r.gvr, r.namespace)
	if err != nil || list == nil {
		if err != nil {
			crlog.FromContext(r.deps.Ctx).Error(err, "list fallback failed", "gvr", r.gvr.String(), "namespace", r.namespace)
		}
		return 0, false
	}
	return len(list.Items), true
}

func (r *ResourceGroupItem) peekEmptyLocked() (bool, bool) {
	ctx := r.deps.Ctx
	has, err := r.deps.Cl.HasAnyByGVR(ctx, r.gvr, r.namespace)
	if err != nil {
		r.lastPeek = time.Now()
		r.lastError = time.Now()
		r.scheduleNextPeekLocked()
		crlog.FromContext(r.deps.Ctx).Error(err, "peek failed", "gvr", r.gvr.String(), "namespace", r.namespace)
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
	r.RowItem.details = spec.detail
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

func (r *ResourceGroupItem) ensureInformerHandler(informer crcache.Informer) {
	if informer == nil {
		return
	}
	r.watchOnce.Do(func() {
		crlog.FromContext(r.deps.Ctx).Info("resource group registering informer handler", "gvr", r.gvr.String(), "namespace", r.namespace)
		_, err := informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				r.onInformerEvent(obj)
			},
			UpdateFunc: func(_, newObj interface{}) {
				r.onInformerEvent(newObj)
			},
			DeleteFunc: func(obj interface{}) {
				r.onInformerEvent(obj)
			},
		})
		if err != nil {
			crlog.FromContext(r.deps.Ctx).Error(err, "register informer handler", "gvr", r.gvr.String(), "namespace", r.namespace)
		}
	})
}

func (r *ResourceGroupItem) onInformerEvent(obj interface{}) {
	log := crlog.FromContext(r.deps.Ctx)
	log.Info("resource group informer event", "gvr", r.gvr.String(), "namespace", r.namespace)
	if r.namespace != "" {
		if acc, ok := accessorForEvent(obj); ok && acc != nil {
			if acc.GetNamespace() != r.namespace {
				log.Info("resource group event ignored due to namespace mismatch", "eventNamespace", acc.GetNamespace())
				return
			}
		}
	}
	r.scheduleRecount()
}

func (r *ResourceGroupItem) scheduleRecount() {
	r.mu.Lock()
	if r.recounting {
		r.mu.Unlock()
		return
	}
	r.recounting = true
	r.nextPeekScheduled = false
	r.countKnown = false
	r.emptyKnown = false
	r.lastPeek = time.Time{}
	r.mu.Unlock()

	go func() {
		defer func() {
			r.mu.Lock()
			r.recounting = false
			r.mu.Unlock()
		}()
		log := crlog.FromContext(r.deps.Ctx)
		log.Info("resource group recount started", "gvr", r.gvr.String(), "namespace", r.namespace)
		_ = r.Count()
		ctx := r.deps.Ctx
		if ctx == nil {
			ctx = context.Background()
		}
		hasAny, err := r.deps.Cl.HasAnyByGVR(ctx, r.gvr, r.namespace)
		if err != nil {
			log.Error(err, "hasAny peek failed", "gvr", r.gvr.String(), "namespace", r.namespace)
		}
		r.mu.Lock()
		if err == nil {
			log.Info("hasAny peek", "gvr", r.gvr.String(), "namespace", r.namespace, "hasAny", hasAny)
			r.empty = !hasAny
			r.emptyKnown = true
			r.lastPeek = time.Now()
		}
		count := r.count
		countKnown := r.countKnown
		onChange := r.onChange
		r.mu.Unlock()
		if countKnown {
			r.setCountCell(fmt.Sprintf("%d", count))
		}
		r.notifyIfChanged(nil)
		if onChange != nil {
			log.Info("resource group recount triggering refresh", "gvr", r.gvr.String(), "namespace", r.namespace, "count", count)
			onChange()
		}
		log.Info("resource group recount finished", "gvr", r.gvr.String(), "namespace", r.namespace, "count", count, "countKnown", countKnown)
	}()
}

func (r *ResourceGroupItem) scheduleNextPeek() {
	r.mu.Lock()
	ctx, delay, ok := r.nextPeekScheduleLocked()
	r.mu.Unlock()
	if !ok {
		return
	}
	go r.runPeekTimer(ctx, delay)
}

func (r *ResourceGroupItem) scheduleNextPeekLocked() {
	ctx, delay, ok := r.nextPeekScheduleLocked()
	if !ok {
		return
	}
	go r.runPeekTimer(ctx, delay)
}

func (r *ResourceGroupItem) nextPeekScheduleLocked() (context.Context, time.Duration, bool) {
	if r.nextPeekScheduled {
		return nil, 0, false
	}
	r.nextPeekScheduled = true
	interval := r.peekInterval()
	ctx := r.deps.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return ctx, interval, true
}

func (r *ResourceGroupItem) runPeekTimer(ctx context.Context, delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		r.scheduleRecount()
	case <-ctx.Done():
	}
}

func (r *ResourceGroupItem) peekInterval() time.Duration {
	cfg := r.deps.AppConfig
	interval := 10 * time.Second
	if cfg != nil && cfg.Resources.PeekInterval.Duration > 0 {
		interval = cfg.Resources.PeekInterval.Duration
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if jitterBase := interval / 5; jitterBase > 0 {
		interval += time.Duration(rand.Int63n(int64(jitterBase)))
	}
	// If the last attempt errored, stretch the interval to avoid hammering broken APIs.
	if time.Since(r.lastError) < interval {
		interval += interval
	}
	return interval
}
