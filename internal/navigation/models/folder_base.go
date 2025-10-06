package models

import (
	"sync"

	table "github.com/sttts/kc/internal/table"
)

// PopulateFunc rebuilds the rows for a folder. Returning nil keeps the
// previously cached rows.
type PopulateFunc func(*BaseFolder) ([]table.Row, error)

// BaseFolder is a reusable foundation for navigation folders. It manages lazy
// row population, breadcrumb metadata, and optional ".." back-row support.
// Concrete folders embed it and provide their own populate function.
type BaseFolder struct {
	Deps Deps

	columns  []table.Column
	path     []string
	populate PopulateFunc

	once sync.Once
	mu   sync.Mutex

	dirty bool
	back  bool

	list  *table.SliceList
	items map[string]Item
}

// NewBaseFolder constructs a BaseFolder with the provided dependencies,
// columns, path, key, and populate function. Callers may later adjust these
// values via the setter helpers if needed.
func NewBaseFolder(deps Deps, cols []table.Column, path []string, populate PopulateFunc) *BaseFolder {
	return &BaseFolder{
		Deps:     deps,
		columns:  append([]table.Column(nil), cols...),
		path:     append([]string(nil), path...),
		populate: populate,
	}
}

// SetColumns replaces the column metadata.
func (b *BaseFolder) SetColumns(cols []table.Column) {
	b.columns = append([]table.Column(nil), cols...)
}

// SetPopulate assigns the populate callback used to rebuild rows lazily.
func (b *BaseFolder) SetPopulate(fn PopulateFunc) { b.populate = fn }

// EnableBack toggles whether the folder should expose a synthetic ".." row.
func (b *BaseFolder) EnableBack(enable bool) { b.back = enable }

// Refresh marks the folder dirty so the next access repopulates rows.
func (b *BaseFolder) Refresh() { b.markDirty() }

// IsDirty reports whether the folder requires a refresh.
func (b *BaseFolder) IsDirty() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dirty
}

// Columns returns the configured columns.
func (b *BaseFolder) Columns() []table.Column { return append([]table.Column(nil), b.columns...) }

// Path returns a copy of the breadcrumb path segments.
func (b *BaseFolder) Path() []string { return append([]string(nil), b.path...) }

// ItemByID returns the cached navigation item by ID when available.
func (b *BaseFolder) ItemByID(id string) (Item, bool) {
	if id == "" {
		return nil, false
	}
	b.ensure()
	if b.back && id == "__back__" {
		return BackItem{}, true
	}
	if b.items == nil {
		return nil, false
	}
	it, ok := b.items[id]
	return it, ok
}

// Lines implements table.List with optional back-row support.
func (b *BaseFolder) Lines(top, num int) []table.Row {
	if num <= 0 {
		return nil
	}
	b.ensure()
	if !b.back {
		return b.list.Lines(top, num)
	}
	if top <= 0 {
		rows := make([]table.Row, 0, num)
		rows = append(rows, BackItem{})
		if num-1 > 0 {
			rows = append(rows, b.list.Lines(0, num-1)...)
		}
		return rows
	}
	return b.list.Lines(top-1, num)
}

// Above implements table.List with back-row handling.
func (b *BaseFolder) Above(id string, n int) []table.Row {
	if n <= 0 {
		return nil
	}
	b.ensure()
	if !b.back || id == "__back__" {
		return nil
	}
	return b.list.Above(id, n)
}

// Below implements table.List with back-row handling.
func (b *BaseFolder) Below(id string, n int) []table.Row {
	if n <= 0 {
		return nil
	}
	b.ensure()
	if b.back && id == "__back__" {
		return b.list.Lines(0, n)
	}
	return b.list.Below(id, n)
}

// Len reports the number of rows including the synthetic back row.
func (b *BaseFolder) Len() int {
	b.ensure()
	if b.back {
		return b.list.Len() + 1
	}
	return b.list.Len()
}

// Find locates a row by ID, accounting for the back row.
func (b *BaseFolder) Find(id string) (int, table.Row, bool) {
	b.ensure()
	if b.back {
		if id == "__back__" {
			return 0, BackItem{}, true
		}
		idx, row, ok := b.list.Find(id)
		if !ok {
			return -1, nil, false
		}
		return idx + 1, row, true
	}
	return b.list.Find(id)
}

// setRows replaces the underlying row list and rebuilds the ID cache.
func (b *BaseFolder) setRows(rows []table.Row) {
	b.list = table.NewSliceList(rows)
	if b.items == nil {
		b.items = make(map[string]Item, len(rows))
	} else {
		for k := range b.items {
			delete(b.items, k)
		}
	}
	for _, row := range rows {
		if item, ok := row.(Item); ok {
			if id, _, _, okCols := row.Columns(); okCols {
				b.items[id] = item
			}
		}
	}
}

// ensure lazy-populates rows when required.
func (b *BaseFolder) ensure() {
	b.once.Do(func() {
		if b.list == nil {
			b.list = table.NewSliceList(nil)
		}
		b.dirty = true
	})
	if !b.dirty {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.dirty {
		return
	}
	if b.populate != nil {
		if rows, err := b.populate(b); err == nil && rows != nil {
			b.setRows(rows)
		}
	}
	if b.list == nil {
		b.list = table.NewSliceList(nil)
	}
	b.dirty = false
}

func (b *BaseFolder) markDirty() {
	b.mu.Lock()
	b.dirty = true
	b.mu.Unlock()
}
