package models

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	table "github.com/sttts/kc/internal/table"
	"github.com/sttts/kc/internal/tablecache"
	"github.com/sttts/kc/pkg/appconfig"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	toolscache "k8s.io/client-go/tools/cache"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// ObjectsFolder provides shared scaffolding for object list folders.
type ObjectsFolder struct {
	*BaseFolder
	gvr                schema.GroupVersionResource
	namespace          string
	rows               *liveObjectRowSource
	objectOrder        string
	hasObjOrder        bool
	verbs              []string
	ageMu              sync.Mutex
	ageHooks           []*ageCellHook
	ageTimer           *time.Timer
	Filter             func(*tablecache.Row) bool
	FilterUnstructured func(*unstructured.Unstructured) bool
}

// NewObjectsFolder constructs an object-list folder with the provided metadata.
func NewObjectsFolder(deps Deps, gvr schema.GroupVersionResource, namespace string, path []string, verbs []string) *ObjectsFolder {
	base := NewBaseFolder(deps, nil, path)
	base.SetColumns([]table.Column{{Title: " Name"}})
	folder := &ObjectsFolder{
		BaseFolder: base,
		gvr:        gvr,
		namespace:  namespace,
		verbs:      append([]string(nil), verbs...),
	}
	rows := newLiveObjectRowSource(folder)
	folder.rows = rows
	base.SetRowSource(rows)
	return folder
}

func (o *ObjectsFolder) populateRows(ctx context.Context) ([]table.Row, error) {
	cfg := o.Deps.AppConfig
	columnsMode := cfg.Objects.Columns
	order := cfg.Objects.Order
	if o.hasObjOrder && o.objectOrder != "" {
		order = o.objectOrder
	}
	if rl, err := o.Deps.Cl.ListRowsByGVR(ctx, o.gvr, o.namespace); err == nil && rl != nil && len(rl.Items) > 0 {
		return o.rowsFromRowList(rl, columnsMode, order), nil
	} else if err != nil && apierrors.IsForbidden(err) {
		return nil, err
	}
	list, err := o.Deps.Cl.ListByGVR(ctx, o.gvr, o.namespace)
	if err != nil {
		return nil, err
	}
	return o.rowsFromList(list, order), nil
}

// GVR exposes the folder's group-version-resource identifier.
func (o *ObjectsFolder) GVR() schema.GroupVersionResource { return o.gvr }

// Namespace returns the namespace when the folder is namespaced, or an empty string when cluster scoped.
func (o *ObjectsFolder) Namespace() string { return o.namespace }

func (o *ObjectsFolder) ObjectListMeta() (schema.GroupVersionResource, string, bool) {
	return o.gvr, o.namespace, true
}

func (o *ObjectsFolder) rowsFromRowList(rl *tablecache.RowList, columnsMode, order string) []table.Row {
	vis := visibleColumns(rl.Columns, columnsMode)
	cols := make([]table.Column, len(vis))
	for i := range vis {
		c := rl.Columns[vis[i]]
		cols[i] = table.Column{Title: c.Name}
	}
	o.SetColumns(cols)
	ageCols := ageColumnIndices(cols)

	idxs := orderRowIndices(rl.Items, order)
	rows := make([]table.Row, 0, len(idxs))
	var hooks []*ageCellHook
	gvStr := o.gvr.GroupVersion().String()
	kind := o.kindString()
	ctor, hasChild := o.childConstructor()

	for _, ii := range idxs {
		rr := &rl.Items[ii]
		if o.Filter != nil && !o.Filter(rr) {
			continue
		}
		name := rowName(rr)
		id := name
		cells := buildCells(rr.Cells, vis, hasChild)
		basePath := append(append([]string{}, o.Path()...), name)
		info := RowStyleInfo{
			GVR:        o.gvr,
			GVK:        rr.TableTarget(),
			ObjectMeta: rr.ObjectMeta,
			BaseStyle:  WhiteStyle(),
		}
		info.Unstructured = unstructuredFromRow(rr)
		style := applyRowStylers(info)
		obj := NewObjectRow(id, cells, basePath, o.gvr, o.namespace, name, style)
		obj.SetResourceVerbs(o.verbs)
		obj.WithViewContent(objectViewContent(o.Deps, o.gvr, o.namespace, name))
		obj.RowItem.details = objectDetails(o.namespace, name, kind, gvStr)
		if len(ageCols) > 0 {
			if hook := newAgeCellHook(ageCols, rr.ObjectMeta.CreationTimestamp.Time); hook != nil {
				obj.RowItem.SetCellsHook(hook.apply)
				hooks = append(hooks, hook)
			}
		}
		if hasChild && ctor != nil {
			ns := o.namespace
			nm := name
			rows = append(rows, NewObjectWithChildItem(obj, func() (Folder, error) {
				return ctor(o.Deps, ns, nm, basePath), nil
			}))
		} else {
			rows = append(rows, obj)
		}
	}
	o.installAgeHooks(hooks)
	return rows
}

func (o *ObjectsFolder) rowsFromList(list *unstructured.UnstructuredList, order string) []table.Row {
	o.installAgeHooks(nil)
	names := make([]string, 0, len(list.Items))
	byName := make(map[string]*unstructured.Unstructured, len(list.Items))
	for i := range list.Items {
		if o.FilterUnstructured != nil && !o.FilterUnstructured(&list.Items[i]) {
			continue
		}
		names = append(names, list.Items[i].GetName())
		byName[list.Items[i].GetName()] = &list.Items[i]
	}
	sort.Strings(names)
	rows := make([]table.Row, 0, len(names))
	gvStr := o.gvr.GroupVersion().String()
	kind := o.kindString()
	ctor, hasChild := o.childConstructor()
	for _, name := range names {
		item := byName[name]
		basePath := append(append([]string{}, o.Path()...), name)
		title := name
		if hasChild {
			title = "/" + strings.TrimPrefix(name, "/")
		}
		meta := metav1.ObjectMeta{
			Name:              name,
			Namespace:         o.namespace,
			UID:               item.GetUID(),
			Labels:            item.GetLabels(),
			Annotations:       item.GetAnnotations(),
			CreationTimestamp: item.GetCreationTimestamp(),
		}
		meta.DeletionTimestamp = item.GetDeletionTimestamp()
		info := RowStyleInfo{
			GVR:          o.gvr,
			GVK:          item.GroupVersionKind(),
			ObjectMeta:   meta,
			Unstructured: item,
			BaseStyle:    WhiteStyle(),
		}
		style := applyRowStylers(info)
		obj := NewObjectRow(name, []string{title}, basePath, o.gvr, o.namespace, name, style)
		obj.SetResourceVerbs(o.verbs)
		obj.WithViewContent(objectViewContent(o.Deps, o.gvr, o.namespace, name))
		obj.RowItem.details = objectDetails(o.namespace, name, kind, gvStr)
		if hasChild && ctor != nil {
			ns := o.namespace
			nm := name
			rows = append(rows, NewObjectWithChildItem(obj, func() (Folder, error) {
				return ctor(o.Deps, ns, nm, basePath), nil
			}))
		} else {
			rows = append(rows, obj)
		}
	}
	return rows
}

func (o *ObjectsFolder) kindString() string {
	if o.Deps.Cl == nil {
		return ""
	}
	if mapper := o.Deps.Cl.RESTMapper(); mapper != nil {
		if k, err := mapper.KindFor(o.gvr); err == nil {
			return k.Kind
		}
	}
	return ""
}

func (o *ObjectsFolder) childConstructor() (ChildConstructor, bool) {
	return ChildFor(o.gvr)
}

// ApplyObjectOrder overrides the object ordering strategy until changed again.
func (o *ObjectsFolder) ApplyObjectOrder(order string) {
	order = normalizeObjectOrder(order)
	if order == "" {
		return
	}
	o.objectOrder = order
	o.hasObjOrder = true
	o.Refresh()
}

func visibleColumns(cols []metav1.TableColumnDefinition, mode string) []int {
	vis := make([]int, 0, len(cols))
	for i, c := range cols {
		if mode == appconfig.ColumnsModeWide || c.Priority == 0 {
			vis = append(vis, i)
		}
	}
	return vis
}

func orderRowIndices(items []tablecache.Row, order string) []int {
	idxs := make([]int, len(items))
	for i := range items {
		idxs[i] = i
	}
	nameOf := func(rr *tablecache.Row) string {
		if rr == nil {
			return ""
		}
		n := rr.Name
		if n == "" && len(rr.Cells) > 0 {
			if s, ok := rr.Cells[0].(string); ok {
				n = strings.TrimPrefix(s, "/")
			}
		}
		return strings.ToLower(n)
	}
	switch order {
	case appconfig.ObjectsOrderNameDesc:
		sort.Slice(idxs, func(i, j int) bool { return nameOf(&items[idxs[i]]) > nameOf(&items[idxs[j]]) })
	case appconfig.ObjectsOrderCreation:
		sort.Slice(idxs, func(i, j int) bool {
			return items[idxs[i]].ObjectMeta.CreationTimestamp.Time.Before(items[idxs[j]].ObjectMeta.CreationTimestamp.Time)
		})
	case appconfig.ObjectsOrderCreationDesc:
		sort.Slice(idxs, func(i, j int) bool {
			return items[idxs[i]].ObjectMeta.CreationTimestamp.Time.After(items[idxs[j]].ObjectMeta.CreationTimestamp.Time)
		})
	default:
		sort.Slice(idxs, func(i, j int) bool { return nameOf(&items[idxs[i]]) < nameOf(&items[idxs[j]]) })
	}
	return idxs
}

func normalizeObjectOrder(order string) string {
	switch strings.ToLower(strings.TrimSpace(order)) {
	case "", appconfig.ObjectsOrderName:
		return appconfig.ObjectsOrderName
	case appconfig.ObjectsOrderNameDesc:
		return appconfig.ObjectsOrderNameDesc
	case appconfig.ObjectsOrderCreation:
		return appconfig.ObjectsOrderCreation
	case appconfig.ObjectsOrderCreationDesc:
		return appconfig.ObjectsOrderCreationDesc
	}
	return appconfig.ObjectsOrderName
}

// NormalizeObjectOrder exposes the canonical form of object ordering for other packages.
func NormalizeObjectOrder(order string) string {
	return normalizeObjectOrder(order)
}

func buildCells(cells []interface{}, vis []int, hasChild bool) []string {
	out := make([]string, len(vis))
	for i := range vis {
		idx := vis[i]
		if idx < len(cells) {
			out[i] = fmt.Sprint(cells[idx])
		}
	}
	if len(out) > 0 && hasChild {
		name := strings.TrimPrefix(out[0], "/")
		out[0] = "/" + name
	}
	return out
}

// rowName extracts the name from a row item, falling back to metadata/name when missing.
func rowName(rr *tablecache.Row) string {
	if rr == nil {
		return ""
	}
	if rr.Name != "" {
		return rr.Name
	}
	if rr.Cells != nil && len(rr.Cells) > 0 {
		if s, ok := rr.Cells[0].(string); ok {
			return strings.TrimPrefix(s, "/")
		}
	}
	return ""
}

func objectDetails(namespace, name, kind, gv string) string {
	if namespace != "" {
		return fmt.Sprintf("%s/%s (%s)", namespace, name, gv)
	}
	return fmt.Sprintf("%s (%s)", name, gv)
}

func ageColumnIndices(cols []table.Column) []int {
	idx := make([]int, 0, len(cols))
	for i := range cols {
		if strings.EqualFold(strings.TrimSpace(cols[i].Title), "age") {
			idx = append(idx, i)
		}
	}
	return idx
}

type liveSourceTarget struct {
	gvr       schema.GroupVersionResource
	namespace string
	name      string
}

// liveObjectRowSource adapts an ObjectsFolder to the rowSource interface while
// keeping rows synced with informer events for the target GVR.
type liveObjectRowSource struct {
	populate        func(context.Context) ([]table.Row, error)
	onFolderDirty   func()
	mu              sync.Mutex
	rows            []table.Row
	index           map[string]int
	items           map[string]Item
	dirty           bool
	once            sync.Once
	watchFactory    func(func(), func()) (func(), error)
	watchCancel     func()
	watchTimer      *time.Timer
	watchTTL        time.Duration
	listBlocked     bool
	watchBlocked    bool
	forbiddenLogged bool
	target          liveSourceTarget
}

func newLiveObjectRowSource(owner *ObjectsFolder) *liveObjectRowSource {
	rows := newLiveObjectRowSourceWithHooks(
		func(ctx context.Context) ([]table.Row, error) { return owner.populateRows(ctx) },
		owner.BaseFolder.markDirtyFromSource,
		func(onEvent func(), onStop func()) (func(), error) {
			return startInformerForObjectsFolder(owner, onEvent, onStop)
		},
	)
	rows.setTarget(liveSourceTarget{gvr: owner.gvr, namespace: owner.namespace})
	rows.watchTTL = watchDuration(owner.Deps)
	return rows
}

func newLiveObjectRowSourceWithHooks(
	populate func(context.Context) ([]table.Row, error),
	onDirty func(),
	startInformer func(func(), func()) (func(), error),
) *liveObjectRowSource {
	src := &liveObjectRowSource{
		populate:      populate,
		onFolderDirty: onDirty,
		dirty:         true,
		watchFactory:  startInformer,
	}
	return src
}

func (s *liveObjectRowSource) setTarget(target liveSourceTarget) {
	s.mu.Lock()
	s.target = target
	s.mu.Unlock()
}

func startInformerForObjectsFolder(owner *ObjectsFolder, onEvent func(), onStop func()) (func(), error) {
	if owner == nil {
		return nil, nil
	}
	return startInformerForResource(owner.Deps, owner.gvr, owner.namespace, "", onEvent, onStop)
}

func (s *liveObjectRowSource) logForbiddenLocked(ctx context.Context, stage string, err error) {
	if s.forbiddenLogged {
		return
	}
	log := ctrllog.FromContext(ctx).WithName("liveObjectRowSource")
	if !s.target.gvr.Empty() {
		log = log.WithValues(
			"gvr", s.target.gvr.String(),
			"namespace", s.target.namespace,
			"name", s.target.name,
		)
	}
	if err != nil {
		log.Info("suppressing list/watch retries after forbidden", "stage", stage, "error", err.Error())
	} else {
		log.Info("suppressing list/watch retries after forbidden", "stage", stage)
	}
	s.forbiddenLogged = true
}

func (s *liveObjectRowSource) blockWatchLocked(ctx context.Context, stage string, err error) {
	if s.watchBlocked {
		return
	}
	if s.watchTimer != nil {
		s.watchTimer.Stop()
		s.watchTimer = nil
	}
	s.watchCancel = nil
	s.watchFactory = nil
	s.watchBlocked = true
	s.logForbiddenLocked(ctx, stage, err)
}

func (s *liveObjectRowSource) blockListLocked(ctx context.Context, err error) {
	if s.listBlocked {
		return
	}
	s.listBlocked = true
	s.dirty = false
	s.logForbiddenLocked(ctx, "list", err)
	s.blockWatchLocked(ctx, "list", nil)
}

func (s *liveObjectRowSource) ensureWatcherLocked(ctx context.Context) {
	if s.watchFactory == nil || s.watchCancel != nil || s.watchBlocked {
		return
	}
	factory := s.watchFactory
	s.mu.Unlock()
	cancel, err := factory(s.MarkDirty, s.watchStopped)
	s.mu.Lock()
	if err != nil {
		if apierrors.IsForbidden(err) {
			s.blockWatchLocked(ctx, "watch", err)
			return
		}
		ctrllog.FromContext(ctx).WithName("liveObjectRowSource").Error(err, "failed to start resource watch")
		return
	}
	if cancel != nil {
		s.watchCancel = cancel
		s.scheduleWatchTimeoutLocked()
	}
}

func (s *liveObjectRowSource) touchWatchLocked() {
	if s.watchTTL <= 0 || s.watchCancel == nil || s.watchBlocked {
		return
	}
	s.scheduleWatchTimeoutLocked()
}

func (s *liveObjectRowSource) scheduleWatchTimeoutLocked() {
	if s.watchTTL <= 0 || s.watchCancel == nil {
		if s.watchTimer != nil {
			s.watchTimer.Stop()
			s.watchTimer = nil
		}
		return
	}
	cancel := s.watchCancel
	if s.watchTimer != nil {
		s.watchTimer.Stop()
	}
	s.watchTimer = time.AfterFunc(s.watchTTL, func() {
		cancel()
	})
}

func (s *liveObjectRowSource) watchStopped() {
	s.mu.Lock()
	if s.watchTimer != nil {
		s.watchTimer.Stop()
		s.watchTimer = nil
	}
	s.watchCancel = nil
	s.mu.Unlock()
}

func (s *liveObjectRowSource) ensureLocked(ctx context.Context) {
	s.once.Do(func() { s.dirty = true })
	if s.listBlocked {
		return
	}
	s.ensureWatcherLocked(ctx)
	s.touchWatchLocked()
	if !s.dirty {
		return
	}
	rows, err := s.populate(ctx)
	if err != nil {
		if apierrors.IsForbidden(err) {
			s.blockListLocked(ctx, err)
			return
		}
		// keep dirty so we retry next time
		s.dirty = true
		return
	}
	s.rows = rows
	s.rebuildIndexLocked()
	s.dirty = false
}

func (s *liveObjectRowSource) rebuildIndexLocked() {
	s.index = make(map[string]int, len(s.rows))
	s.items = make(map[string]Item, len(s.rows))
	for i, row := range s.rows {
		if row == nil {
			continue
		}
		id, _, _, ok := row.Columns()
		if !ok {
			continue
		}
		s.index[id] = i
		if item, ok := row.(Item); ok {
			s.items[id] = item
		}
	}
}

func (s *liveObjectRowSource) Lines(ctx context.Context, top, num int) []table.Row {
	if num <= 0 {
		return nil
	}
	s.mu.Lock()
	s.ensureLocked(ctx)
	rows := s.rows
	s.mu.Unlock()
	if len(rows) == 0 || top >= len(rows) {
		return nil
	}
	if top < 0 {
		top = 0
	}
	end := top + num
	if end > len(rows) {
		end = len(rows)
	}
	return rows[top:end]
}

func (s *liveObjectRowSource) Above(ctx context.Context, id string, n int) []table.Row {
	if n <= 0 {
		return nil
	}
	s.mu.Lock()
	s.ensureLocked(ctx)
	idx, ok := s.index[id]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	start := idx - n
	if start < 0 {
		start = 0
	}
	rows := append([]table.Row(nil), s.rows[start:idx]...)
	s.mu.Unlock()
	if len(rows) == 0 {
		return nil
	}
	return rows
}

func (s *liveObjectRowSource) Below(ctx context.Context, id string, n int) []table.Row {
	if n <= 0 {
		return nil
	}
	s.mu.Lock()
	s.ensureLocked(ctx)
	idx, ok := s.index[id]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	start := idx + 1
	if start >= len(s.rows) {
		s.mu.Unlock()
		return nil
	}
	end := start + n
	if end > len(s.rows) {
		end = len(s.rows)
	}
	rows := append([]table.Row(nil), s.rows[start:end]...)
	s.mu.Unlock()
	return rows
}

func (s *liveObjectRowSource) Len(ctx context.Context) int {
	s.mu.Lock()
	s.ensureLocked(ctx)
	ln := len(s.rows)
	s.mu.Unlock()
	return ln
}

func (s *liveObjectRowSource) Find(ctx context.Context, id string) (int, table.Row, bool) {
	s.mu.Lock()
	s.ensureLocked(ctx)
	idx, ok := s.index[id]
	if !ok || idx < 0 || idx >= len(s.rows) {
		s.mu.Unlock()
		return -1, nil, false
	}
	row := s.rows[idx]
	s.mu.Unlock()
	return idx, row, true
}

func (s *liveObjectRowSource) ItemByID(ctx context.Context, id string) (Item, bool) {
	s.mu.Lock()
	s.ensureLocked(ctx)
	it, ok := s.items[id]
	s.mu.Unlock()
	return it, ok
}

func (s *liveObjectRowSource) MarkDirty() {
	s.mu.Lock()
	if s.listBlocked {
		s.mu.Unlock()
		return
	}
	s.dirty = true
	s.mu.Unlock()
	if s.onFolderDirty != nil {
		s.onFolderDirty()
	}
}

func newLiveKeyRowSource(deps Deps, gvr schema.GroupVersionResource, namespace, name string, populate func(context.Context) ([]table.Row, error), onDirty func()) *liveObjectRowSource {
	rows := newLiveObjectRowSourceWithHooks(populate, onDirty, func(onEvent func(), onStop func()) (func(), error) {
		return startInformerForResource(deps, gvr, namespace, name, onEvent, onStop)
	})
	rows.setTarget(liveSourceTarget{gvr: gvr, namespace: namespace, name: name})
	rows.watchTTL = watchDuration(deps)
	return rows
}

func startInformerForResource(deps Deps, gvr schema.GroupVersionResource, namespace, name string, onEvent func(), onStop func()) (func(), error) {
	if deps.Cl == nil {
		return nil, nil
	}
	ctx := deps.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	log := ctrllog.FromContext(ctx).WithName("resourceWatch").WithValues("gvr", gvr.String(), "namespace", namespace, "name", name)
	mapper := deps.Cl.RESTMapper()
	if mapper == nil {
		return nil, fmt.Errorf("rest mapper unavailable")
	}
	gvk, err := mapper.KindFor(gvr)
	if err != nil {
		return nil, err
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	informer, err := deps.Cl.GetCache().GetInformer(ctx, obj, crcache.BlockUntilSynced(true))
	if err != nil || informer == nil {
		return nil, err
	}
	type handlerReg interface {
		Remove() error
	}
	reg, err := informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if !matchesTarget(obj, namespace, name) {
				return
			}
			if onEvent != nil {
				onEvent()
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if !matchesTarget(newObj, namespace, name) {
				return
			}
			if onEvent != nil {
				onEvent()
			}
		},
		DeleteFunc: func(obj interface{}) {
			if !matchesTarget(obj, namespace, name) {
				return
			}
			if onEvent != nil {
				onEvent()
			}
		},
	})
	if err != nil {
		log.Error(err, "failed to add informer handler")
		return nil, err
	}
	if informer.HasSynced() && onEvent != nil {
		onEvent()
	}
	cancel := func() {
		if h, ok := reg.(handlerReg); ok {
			_ = h.Remove()
		}
		if onStop != nil {
			onStop()
		}
	}
	return cancel, nil
}

func watchDuration(deps Deps) time.Duration {
	if deps.AppConfig != nil {
		if dur := deps.AppConfig.Kubernetes.Clusters.TTL.Duration; dur > 0 {
			return dur
		}
	}
	return 2 * time.Minute
}

func accessorForEvent(obj interface{}) (metav1.Object, bool) {
	switch o := obj.(type) {
	case toolscache.DeletedFinalStateUnknown:
		return accessorForEvent(o.Obj)
	default:
		accessor, err := meta.Accessor(o)
		if err != nil {
			return nil, false
		}
		return accessor, true
	}
}

func matchesTarget(obj interface{}, namespace, name string) bool {
	accessor, ok := accessorForEvent(obj)
	if !ok || accessor == nil {
		return false
	}
	if namespace != "" && accessor.GetNamespace() != namespace {
		return false
	}
	if name != "" && accessor.GetName() != name {
		return false
	}
	return true
}

func (o *ObjectsFolder) installAgeHooks(hooks []*ageCellHook) {
	o.ageMu.Lock()
	defer o.ageMu.Unlock()
	if len(hooks) == 0 {
		o.ageHooks = nil
		if o.ageTimer != nil {
			o.ageTimer.Stop()
			o.ageTimer = nil
		}
		return
	}
	o.ageHooks = hooks
	interval := o.nextAgeIntervalLocked(time.Now())
	o.scheduleAgeTimerLocked(interval)
}

func (o *ObjectsFolder) nextAgeIntervalLocked(now time.Time) time.Duration {
	if len(o.ageHooks) == 0 {
		return 0
	}
	var interval time.Duration
	for _, hook := range o.ageHooks {
		if hook == nil {
			continue
		}
		if d := hook.nextInterval(now); d > 0 {
			if interval == 0 || d < interval {
				interval = d
			}
		}
	}
	return interval
}

func (o *ObjectsFolder) scheduleAgeTimerLocked(interval time.Duration) {
	if o.ageTimer != nil {
		o.ageTimer.Stop()
		o.ageTimer = nil
	}
	if interval <= 0 {
		return
	}
	o.ageTimer = time.AfterFunc(interval, o.ageTick)
}

func (o *ObjectsFolder) ageTick() {
	select {
	case <-o.Deps.Ctx.Done():
		return
	default:
	}
	o.BaseFolder.markDirtyFromSource()
	o.ageMu.Lock()
	defer o.ageMu.Unlock()
	o.ageTimer = nil
	interval := o.nextAgeIntervalLocked(time.Now())
	o.scheduleAgeTimerLocked(interval)
}
