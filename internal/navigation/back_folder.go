package navigation

import (
	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// backFolder wraps a Folder and injects a ".." BackItem as the first row when hasBack is true.
type backFolder struct {
	inner   Folder
	hasBack bool
}

// WithBack returns a Folder that yields a ".." BackItem as first row when hasBack is true.
// When hasBack is false, it returns the original folder unmodified.
func WithBack(f Folder, hasBack bool) Folder {
	if f == nil || !hasBack {
		return f
	}
	return &backFolder{inner: f, hasBack: true}
}

// Folder interface -------------------------------------------------------------

func (b *backFolder) Columns() []table.Column { return b.inner.Columns() }
func (b *backFolder) Title() string           { return b.inner.Title() }
func (b *backFolder) Key() string             { return b.inner.Key() }

func (b *backFolder) ItemByID(id string) (Item, bool) {
	if !b.hasBack {
		if lookup, ok := b.inner.(interface{ ItemByID(string) (Item, bool) }); ok {
			return lookup.ItemByID(id)
		}
		return nil, false
	}
	if id == "__back__" {
		return BackItem{}, true
	}
	if lookup, ok := b.inner.(interface{ ItemByID(string) (Item, bool) }); ok {
		return lookup.ItemByID(id)
	}
	return nil, false
}

// Object list meta passthrough when available
func (b *backFolder) ObjectListMeta() (schema.GroupVersionResource, string, bool) {
	type metaProv interface {
		ObjectListMeta() (schema.GroupVersionResource, string, bool)
	}
	if mp, ok := b.inner.(metaProv); ok {
		return mp.ObjectListMeta()
	}
	return schema.GroupVersionResource{}, "", false
}

// KeyFolder passthrough: if the inner folder exposes key-parent coordinates
// (e.g., ConfigMap/Secret data folders), delegate to it so callers can detect
// KeyFolder even when wrapped with back support.
func (b *backFolder) Parent() (schema.GroupVersionResource, string, string) {
	type keyFolder interface {
		Parent() (schema.GroupVersionResource, string, string)
	}
	if kf, ok := b.inner.(keyFolder); ok {
		return kf.Parent()
	}
	return schema.GroupVersionResource{}, "", ""
}

// table.List implementation ----------------------------------------------------

func (b *backFolder) Len() int {
	if !b.hasBack {
		return b.inner.Len()
	}
	return b.inner.Len() + 1
}

func (b *backFolder) Lines(top, num int) []table.Row {
	if num <= 0 {
		return nil
	}
	if !b.hasBack {
		return b.inner.Lines(top, num)
	}
	if top <= 0 {
		out := make([]table.Row, 0, num)
		out = append(out, BackItem{})
		if num-1 > 0 {
			out = append(out, b.inner.Lines(0, num-1)...)
		}
		return out
	}
	return b.inner.Lines(top-1, num)
}

func (b *backFolder) Above(rowID string, num int) []table.Row {
	if !b.hasBack || num <= 0 {
		return b.inner.Above(rowID, num)
	}
	if rowID == "__back__" {
		return nil
	}
	return b.inner.Above(rowID, num)
}

func (b *backFolder) Below(rowID string, num int) []table.Row {
	if !b.hasBack || num <= 0 {
		return b.inner.Below(rowID, num)
	}
	if rowID == "__back__" {
		return b.inner.Lines(0, num)
	}
	return b.inner.Below(rowID, num)
}

func (b *backFolder) Find(rowID string) (int, table.Row, bool) {
	if !b.hasBack {
		return b.inner.Find(rowID)
	}
	if rowID == "__back__" {
		return 0, BackItem{}, true
	}
	idx, r, ok := b.inner.Find(rowID)
	if !ok {
		return -1, nil, false
	}
	return idx + 1, r, true
}
