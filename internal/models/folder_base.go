package models

import (
	"sync"

	table "github.com/sttts/kc/internal/table"
)

// BaseFolder provides shared metadata (dependencies, columns, breadcrumb path)
// and dirty tracking for navigation folders. Concrete folders install a
// rowSource that surfaces their underlying data directly to the table view.
type BaseFolder struct {
	Deps Deps

	columns []table.Column
	path    []string

	mu     sync.Mutex
	dirty  bool
	source rowSource
}

// NewBaseFolder constructs a BaseFolder with the provided dependencies,
// columns, and path. Callers embed it and attach a rowSource via SetRowSource
// or the convenience SetPopulate helper.
func NewBaseFolder(deps Deps, cols []table.Column, path []string) *BaseFolder {
	return &BaseFolder{
		Deps:    deps,
		columns: append([]table.Column(nil), cols...),
		path:    append([]string(nil), path...),
		dirty:   true,
	}
}

// SetColumns replaces the column metadata.
func (b *BaseFolder) SetColumns(cols []table.Column) {
	b.columns = append([]table.Column(nil), cols...)
}

// Refresh marks the folder dirty so the next access repaints rows.
func (b *BaseFolder) Refresh() { b.markDirty() }

// IsDirty reports whether the folder requested a repaint.
func (b *BaseFolder) IsDirty() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dirty
}

// Columns returns the configured columns.
func (b *BaseFolder) Columns() []table.Column { return append([]table.Column(nil), b.columns...) }

// Path returns a copy of the breadcrumb path segments.
func (b *BaseFolder) Path() []string { return append([]string(nil), b.path...) }

// ItemByID returns the navigation item by ID when available.
func (b *BaseFolder) ItemByID(id string) (Item, bool) {
	if id == "" {
		return nil, false
	}
	if b.hasBack() && id == "__back__" {
		return BackItem{}, true
	}
	src := b.rowSource()
	if src == nil {
		return nil, false
	}
	return src.ItemByID(id)
}

// Lines implements table.List with optional back-row support.
func (b *BaseFolder) Lines(top, num int) []table.Row {
	if num <= 0 {
		return nil
	}
	src := b.rowSource()
	if src == nil {
		if b.hasBack() && top <= 0 {
			b.clearDirty()
			return []table.Row{BackItem{}}
		}
		b.clearDirty()
		return nil
	}
	if !b.hasBack() {
		rows := src.Lines(top, num)
		b.clearDirty()
		return rows
	}
	if top <= 0 {
		rows := make([]table.Row, 0, num)
		rows = append(rows, BackItem{})
		if num-1 > 0 {
			rows = append(rows, src.Lines(0, num-1)...)
		}
		b.clearDirty()
		return rows
	}
	rows := src.Lines(top-1, num)
	b.clearDirty()
	return rows
}

// Above implements table.List with back-row handling.
func (b *BaseFolder) Above(id string, n int) []table.Row {
	if n <= 0 {
		return nil
	}
	src := b.rowSource()
	if src == nil {
		return nil
	}
	if !b.hasBack() || id == "__back__" {
		return src.Above(id, n)
	}
	return src.Above(id, n)
}

// Below implements table.List with back-row handling.
func (b *BaseFolder) Below(id string, n int) []table.Row {
	if n <= 0 {
		return nil
	}
	src := b.rowSource()
	if src == nil {
		return nil
	}
	if b.hasBack() && id == "__back__" {
		return src.Lines(0, n)
	}
	return src.Below(id, n)
}

// Len reports the number of rows including the synthetic back row when present.
func (b *BaseFolder) Len() int {
	src := b.rowSource()
	count := 0
	if src != nil {
		count = src.Len()
	}
	if b.hasBack() {
		return count + 1
	}
	return count
}

// Find locates a row by ID, accounting for the back row.
func (b *BaseFolder) Find(id string) (int, table.Row, bool) {
	src := b.rowSource()
	if src == nil {
		return -1, nil, false
	}
	if b.hasBack() {
		if id == "__back__" {
			return 0, BackItem{}, true
		}
		idx, row, ok := src.Find(id)
		if !ok {
			return -1, nil, false
		}
		return idx + 1, row, true
	}
	return src.Find(id)
}

func (b *BaseFolder) hasBack() bool { return len(b.path) > 0 }

func (b *BaseFolder) markDirty() {
	b.mu.Lock()
	b.dirty = true
	src := b.source
	b.mu.Unlock()
	if src != nil {
		src.MarkDirty()
	}
}

func (b *BaseFolder) clearDirty() {
	b.mu.Lock()
	b.dirty = false
	b.mu.Unlock()
}

func (b *BaseFolder) markDirtyFromSource() {
	b.mu.Lock()
	b.dirty = true
	b.mu.Unlock()
}

func (b *BaseFolder) rowSource() rowSource {
	b.mu.Lock()
	src := b.source
	b.mu.Unlock()
	return src
}

// SetRowSource installs the provider responsible for serving rows for this
// folder. Calling this marks the folder dirty so the next access uses the new
// source snapshot.
func (b *BaseFolder) SetRowSource(src rowSource) {
	b.mu.Lock()
	b.source = src
	b.dirty = true
	b.mu.Unlock()
	if src != nil {
		src.MarkDirty()
	}
}

// SetPopulate is a convenience helper for cached folders that still rebuild
// their rows via a populate callback. It wraps the callback with a
// sliceRowSource and installs it.
func (b *BaseFolder) SetPopulate(fn func() ([]table.Row, error)) {
	if fn == nil {
		b.SetRowSource(nil)
		return
	}
	src := newSliceRowSource(fn)
	b.SetRowSource(src)
}
