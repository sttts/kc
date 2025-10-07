package models

import (
	"fmt"
	"sort"
	"strings"
	"time"

	table "github.com/sttts/kc/internal/table"
	"github.com/sttts/kc/internal/tablecache"
	"github.com/sttts/kc/pkg/appconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilduration "k8s.io/apimachinery/pkg/util/duration"
)

// ObjectsFolder provides shared scaffolding for object list folders.
type ObjectsFolder struct {
	*BaseFolder
	gvr       schema.GroupVersionResource
	namespace string // empty means cluster scope
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
	base.SetPopulate(folder.populateRows)
	return folder
}

// GVR exposes the folder's group-version-resource identifier.
func (o *ObjectsFolder) GVR() schema.GroupVersionResource { return o.gvr }

// Namespace returns the namespace when the folder is namespaced, or an empty string when cluster scoped.
func (o *ObjectsFolder) Namespace() string { return o.namespace }

func (o *ObjectsFolder) ObjectListMeta() (schema.GroupVersionResource, string, bool) {
	return o.gvr, o.namespace, true
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
