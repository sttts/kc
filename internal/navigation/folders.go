package navigation

import (
    table "github.com/sttts/kc/internal/table"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

// SliceFolder is a basic Folder backed by a table.SliceList and static columns.
type SliceFolder struct {
    title string
    key   string
    cols  []table.Column
    list  *table.SliceList
}

var _ Folder = (*SliceFolder)(nil)

// NewSliceFolder builds a Folder from rows and columns with title/key metadata.
func NewSliceFolder(title, key string, cols []table.Column, rows []table.Row) *SliceFolder {
    return &SliceFolder{title: title, key: key, cols: cols, list: table.NewSliceList(rows)}
}

// Folder interface implementation -------------------------------------------------

func (f *SliceFolder) Columns() []table.Column { return f.cols }
func (f *SliceFolder) Title() string          { return f.title }
func (f *SliceFolder) Key() string            { return f.key }

func (f *SliceFolder) Lines(top, num int) []table.Row { return f.list.Lines(top, num) }
func (f *SliceFolder) Above(rowID string, num int) []table.Row { return f.list.Above(rowID, num) }
func (f *SliceFolder) Below(rowID string, num int) []table.Row { return f.list.Below(rowID, num) }
func (f *SliceFolder) Len() int { return f.list.Len() }
func (f *SliceFolder) Find(rowID string) (int, table.Row, bool) { return f.list.Find(rowID) }

// Constructors for common folders -------------------------------------------------

// NewContextsFolder lists available contexts. Key is simply "contexts".
func NewContextsFolder(rows []table.Row) *SliceFolder {
    return NewSliceFolder("contexts", "contexts", []table.Column{{Title: " Name"}}, rows)
}

// NewNamespacesFolder lists namespaces for a given context.
func NewNamespacesFolder(contextName string, rows []table.Row) *SliceFolder {
    key := contextName + "/namespaces"
    return NewSliceFolder("namespaces", key, []table.Column{{Title: " Name"}}, rows)
}

// NewGroupFolder lists objects for a GVR (namespaced or cluster-scoped).
func NewGroupFolder(contextName string, gvr schema.GroupVersionResource, namespace, title string, rows []table.Row) *SliceFolder {
    key := contextName + "/" + gvr.String()
    if namespace != "" { key = contextName + "/namespaces/" + namespace + "/" + gvr.Resource }
    // Typical columns: Name, Group (dim), Count (right-aligned) – callers choose.
    return NewSliceFolder(title, key, []table.Column{{Title: " Name"}}, rows)
}

// NewObjectsFolder lists concrete objects of the given GVR in a namespace.
func NewObjectsFolder(contextName string, gvr schema.GroupVersionResource, namespace string, rows []table.Row) *SliceFolder {
    title := gvr.Resource
    key := contextName + "/" + gvr.String()
    if namespace != "" { key = contextName + "/namespaces/" + namespace + "/" + gvr.Resource }
    return NewSliceFolder(title, key, []table.Column{{Title: " Name"}}, rows)
}
