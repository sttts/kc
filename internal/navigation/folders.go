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
    // optional object-list metadata for YAML/F3
    gvr       schema.GroupVersionResource
    namespace string
    hasMeta   bool
}

var _ Folder = (*SliceFolder)(nil)

// Note: legacy alias types and specialized constructors have been removed in
// favor of concrete folders in folders_new.go. This file now only provides
// a generic SliceFolder used in tests and simple cases.

// NewSliceFolder builds a Folder from rows and columns with title/key metadata.
func NewSliceFolder(title, key string, cols []table.Column, rows []table.Row) *SliceFolder {
    return &SliceFolder{title: title, key: key, cols: cols, list: table.NewSliceList(rows)}
}

// Folder interface implementation -------------------------------------------------

func (f *SliceFolder) Columns() []table.Column { return f.cols }
func (f *SliceFolder) Title() string          { return f.title }
func (f *SliceFolder) Key() string            { return f.key }

// ObjectListMeta returns GVR/namespace when this folder represents a concrete
// object listing. ok=false if not applicable.
func (f *SliceFolder) ObjectListMeta() (schema.GroupVersionResource, string, bool) {
    if f.hasMeta { return f.gvr, f.namespace, true }
    return schema.GroupVersionResource{}, "", false
}

func (f *SliceFolder) Lines(top, num int) []table.Row { return f.list.Lines(top, num) }
func (f *SliceFolder) Above(rowID string, num int) []table.Row { return f.list.Above(rowID, num) }
func (f *SliceFolder) Below(rowID string, num int) []table.Row { return f.list.Below(rowID, num) }
func (f *SliceFolder) Len() int { return f.list.Len() }
func (f *SliceFolder) Find(rowID string) (int, table.Row, bool) { return f.list.Find(rowID) }

// Constructors removed (see note above).
