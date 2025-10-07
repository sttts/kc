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
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilduration "k8s.io/apimachinery/pkg/util/duration"
	toolscache "k8s.io/client-go/tools/cache"
)

// ObjectsFolder provides shared scaffolding for object list folders.
type ObjectsFolder struct {
	*BaseFolder
	gvr       schema.GroupVersionResource
	namespace string
	rows      *liveObjectRowSource
}

// NewObjectsFolder constructs an object-list folder with the provided metadata.
func NewObjectsFolder(deps Deps, gvr schema.GroupVersionResource, namespace string, path []string) *ObjectsFolder {
	base := NewBaseFolder(deps, nil, path)
	base.SetColumns([]table.Column{{Title: " Name"}})
	folder := &ObjectsFolder{
		BaseFolder: base,
		gvr:        gvr,
		namespace:  namespace,
	}
	rows := newLiveObjectRowSource(folder)
	folder.rows = rows
	base.SetRowSource(rows)
	return folder
}

func (o *ObjectsFolder) populateRows() ([]table.Row, error) {
	cfg := o.Deps.AppConfig
	columnsMode := cfg.Objects.Columns
	order := cfg.Objects.Order
	if rl, err := o.Deps.Cl.ListRowsByGVR(o.Deps.Ctx, o.gvr, o.namespace); err == nil && rl != nil && len(rl.Items) > 0 {
		return o.rowsFromRowList(rl, columnsMode, order), nil
	}
	list, err := o.Deps.Cl.ListByGVR(o.Deps.Ctx, o.gvr, o.namespace)
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
	cols := make([]table.Column, len(vis)+1)
	for i := range vis {
		c := rl.Columns[vis[i]]
		cols[i] = table.Column{Title: c.Name}
	}
	cols[len(cols)-1] = table.Column{Title: "Age"}
	o.SetColumns(cols)

	idxs := orderRowIndices(rl.Items, order)
	rows := make([]table.Row, 0, len(idxs))
	nameStyle := WhiteStyle()
	gvStr := o.gvr.GroupVersion().String()
	kind := o.kindString()
	ctor, hasChild := o.childConstructor()

	for _, ii := range idxs {
		rr := &rl.Items[ii]
		name := rowName(rr)
		id := name
		cells := buildCells(rr.Cells, vis, hasChild)
		age := ""
		if !rr.ObjectMeta.CreationTimestamp.IsZero() {
			age = utilduration.HumanDuration(time.Since(rr.ObjectMeta.CreationTimestamp.Time))
		}
		cells[len(cells)-1] = age
		basePath := append(append([]string{}, o.Path()...), name)
		obj := NewObjectRow(id, cells, basePath, o.gvr, o.namespace, name, nameStyle)
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

func (o *ObjectsFolder) rowsFromList(list *unstructured.UnstructuredList, order string) []table.Row {
	names := make([]string, 0, len(list.Items))
	for i := range list.Items {
		names = append(names, list.Items[i].GetName())
	}
	sort.Strings(names)
	rows := make([]table.Row, 0, len(names))
	nameStyle := WhiteStyle()
	gvStr := o.gvr.GroupVersion().String()
	kind := o.kindString()
	ctor, hasChild := o.childConstructor()
	for _, name := range names {
		basePath := append(append([]string{}, o.Path()...), name)
		title := name
		if hasChild {
			title = "/" + name
		}
		obj := NewObjectRow(name, []string{title}, basePath, o.gvr, o.namespace, name, nameStyle)
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

func buildCells(cells []interface{}, vis []int, hasChild bool) []string {
	out := make([]string, len(vis)+1)
	for i := range vis {
		idx := vis[i]
		if idx < len(cells) {
			out[i] = fmt.Sprint(cells[idx])
		}
	}
	if len(out) > 0 && hasChild {
		out[0] = "/" + strings.TrimPrefix(out[0], "/")
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

// liveObjectRowSource adapts an ObjectsFolder to the rowSource interface while
// keeping rows synced with informer events for the target GVR.
type liveObjectRowSource struct {
	populate      func() ([]table.Row, error)
	onFolderDirty func()
	mu            sync.Mutex
	rows          []table.Row
	index         map[string]int
	items         map[string]Item
	dirty         bool
	once          sync.Once
}

func newLiveObjectRowSource(owner *ObjectsFolder) *liveObjectRowSource {
	return newLiveObjectRowSourceWithHooks(
		func() ([]table.Row, error) { return owner.populateRows() },
		func() { owner.BaseFolder.markDirtyFromSource() },
		func(cb func()) { startInformerForObjectsFolder(owner, cb) },
	)
}

func newLiveObjectRowSourceWithHooks(populate func() ([]table.Row, error), onDirty func(), startInformer func(func())) *liveObjectRowSource {
	src := &liveObjectRowSource{
		populate:      populate,
		onFolderDirty: onDirty,
		dirty:         true,
	}
	if startInformer != nil {
		startInformer(src.MarkDirty)
	}
	return src
}

func startInformerForObjectsFolder(owner *ObjectsFolder, onEvent func()) {
	if owner == nil {
		return
	}
	startInformerForResource(owner.Deps, owner.gvr, owner.namespace, "", onEvent)
}

func (s *liveObjectRowSource) ensureLocked() {
	s.once.Do(func() { s.dirty = true })
	if !s.dirty {
		return
	}
	rows, err := s.populate()
	if err != nil {
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

func (s *liveObjectRowSource) Lines(top, num int) []table.Row {
	if num <= 0 {
		return nil
	}
	s.mu.Lock()
	s.ensureLocked()
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

func (s *liveObjectRowSource) Above(id string, n int) []table.Row {
	if n <= 0 {
		return nil
	}
	s.mu.Lock()
	s.ensureLocked()
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

func (s *liveObjectRowSource) Below(id string, n int) []table.Row {
	if n <= 0 {
		return nil
	}
	s.mu.Lock()
	s.ensureLocked()
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

func (s *liveObjectRowSource) Len() int {
	s.mu.Lock()
	s.ensureLocked()
	ln := len(s.rows)
	s.mu.Unlock()
	return ln
}

func (s *liveObjectRowSource) Find(id string) (int, table.Row, bool) {
	s.mu.Lock()
	s.ensureLocked()
	idx, ok := s.index[id]
	if !ok || idx < 0 || idx >= len(s.rows) {
		s.mu.Unlock()
		return -1, nil, false
	}
	row := s.rows[idx]
	s.mu.Unlock()
	return idx, row, true
}

func (s *liveObjectRowSource) ItemByID(id string) (Item, bool) {
	s.mu.Lock()
	s.ensureLocked()
	it, ok := s.items[id]
	s.mu.Unlock()
	return it, ok
}

func (s *liveObjectRowSource) MarkDirty() {
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
	if s.onFolderDirty != nil {
		s.onFolderDirty()
	}
}

func newLiveKeyRowSource(deps Deps, gvr schema.GroupVersionResource, namespace, name string, populate func() ([]table.Row, error), onDirty func()) *liveObjectRowSource {
	return newLiveObjectRowSourceWithHooks(populate, onDirty, func(cb func()) {
		startInformerForResource(deps, gvr, namespace, name, cb)
	})
}

func startInformerForResource(deps Deps, gvr schema.GroupVersionResource, namespace, name string, onEvent func()) {
	if onEvent == nil || deps.Cl == nil {
		return
	}
	ctx := deps.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	gvk, err := deps.Cl.RESTMapper().KindFor(gvr)
	if err != nil {
		return
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	informer, err := deps.Cl.GetCache().GetInformer(ctx, obj)
	if err != nil {
		return
	}
	matches := func(evt interface{}) bool {
		accessor, ok := accessorForEvent(evt)
		if !ok {
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
	_, _ = informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if matches(obj) {
				onEvent()
			}
		},
		UpdateFunc: func(_, newObj interface{}) {
			if matches(newObj) {
				onEvent()
			}
		},
		DeleteFunc: func(obj interface{}) {
			if matches(obj) {
				onEvent()
			}
		},
	})
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
