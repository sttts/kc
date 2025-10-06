package modeltesting

import (
	"strings"

	models "github.com/sttts/kc/internal/models"
	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SliceFolder is a basic Folder backed by a table.SliceList and static columns.
type SliceFolder struct {
	path []string
	cols []table.Column
	list *table.SliceList
	// optional object-list metadata for YAML/F3
	gvr       schema.GroupVersionResource
	namespace string
	hasMeta   bool
}

var _ models.Folder = (*SliceFolder)(nil)

// NewSliceFolder builds a testing folder from rows and columns. The title is
// converted into path segments ("/" -> root, "a/b" -> ["a","b"], etc.).
func NewSliceFolder(title string, cols []table.Column, rows []table.Row) *SliceFolder {
	var path []string
	if title != "" && title != "/" {
		parts := strings.Split(title, "/")
		for _, p := range parts {
			if p != "" {
				path = append(path, p)
			}
		}
	}
	return &SliceFolder{path: path, cols: cols, list: table.NewSliceList(rows)}
}

func (f *SliceFolder) Columns() []table.Column { return f.cols }
func (f *SliceFolder) Path() []string          { return append([]string(nil), f.path...) }

func (f *SliceFolder) ObjectListMeta() (schema.GroupVersionResource, string, bool) {
	if f.hasMeta {
		return f.gvr, f.namespace, true
	}
	return schema.GroupVersionResource{}, "", false
}

func (f *SliceFolder) Lines(top, num int) []table.Row {
	if num <= 0 {
		return nil
	}
	if !f.hasBack() {
		return f.list.Lines(top, num)
	}
	if top <= 0 {
		rows := make([]table.Row, 0, num)
		rows = append(rows, models.BackItem{})
		if num-1 > 0 {
			rows = append(rows, f.list.Lines(0, num-1)...)
		}
		return rows
	}
	return f.list.Lines(top-1, num)
}

func (f *SliceFolder) Above(rowID string, num int) []table.Row {
	if num <= 0 {
		return nil
	}
	if !f.hasBack() || rowID == "__back__" {
		return nil
	}
	return f.list.Above(rowID, num)
}

func (f *SliceFolder) Below(rowID string, num int) []table.Row {
	if num <= 0 {
		return nil
	}
	if f.hasBack() && rowID == "__back__" {
		return f.list.Lines(0, num)
	}
	return f.list.Below(rowID, num)
}

func (f *SliceFolder) Len() int {
	if f.hasBack() {
		return f.list.Len() + 1
	}
	return f.list.Len()
}

func (f *SliceFolder) Find(rowID string) (int, table.Row, bool) {
	if f.hasBack() {
		if rowID == "__back__" {
			return 0, models.BackItem{}, true
		}
		idx, row, ok := f.list.Find(rowID)
		if !ok {
			return -1, nil, false
		}
		return idx + 1, row, true
	}
	return f.list.Find(rowID)
}

func (f *SliceFolder) ItemByID(id string) (models.Item, bool) {
	if id == "" {
		return nil, false
	}
	if f.hasBack() && id == "__back__" {
		return models.BackItem{}, true
	}
	_, row, ok := f.list.Find(id)
	if !ok {
		return nil, false
	}
	it, ok := row.(models.Item)
	return it, ok
}

func (f *SliceFolder) hasBack() bool { return len(f.path) > 0 }
