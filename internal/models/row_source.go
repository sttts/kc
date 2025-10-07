package models

import (
	"sync"

	table "github.com/sttts/kc/internal/table"
)

// rowSource supplies row windows for a folder. Implementations may cache rows
// or stream them directly from informer-backed stores.
type rowSource interface {
	Lines(top, num int) []table.Row
	Above(id string, n int) []table.Row
	Below(id string, n int) []table.Row
	Len() int
	Find(id string) (int, table.Row, bool)
	ItemByID(id string) (Item, bool)
	MarkDirty()
}

// sliceRowSource maintains an in-memory snapshot of rows computed on demand via
// a populate callback. It replaces the previous table.SliceList usage while
// keeping cached folders simple for now.
type sliceRowSource struct {
	populate func() ([]table.Row, error)

	mu    sync.Mutex
	rows  []table.Row
	index map[string]int
	items map[string]Item
	dirty bool
	once  sync.Once
}

func newSliceRowSource(populate func() ([]table.Row, error)) *sliceRowSource {
	return &sliceRowSource{populate: populate, dirty: true}
}

func (s *sliceRowSource) setPopulate(populate func() ([]table.Row, error)) {
	s.mu.Lock()
	s.populate = populate
	s.dirty = true
	s.mu.Unlock()
}

func (s *sliceRowSource) ensureLocked() {
	s.once.Do(func() { s.dirty = true })
	if !s.dirty {
		return
	}
	s.dirty = false
	if s.populate == nil {
		s.rows = nil
		s.index = nil
		s.items = nil
		return
	}
	rows, err := s.populate()
	if err != nil {
		// Preserve previous snapshot if populate fails; caller may log separately.
		s.dirty = true
		return
	}
	s.rows = rows
	s.rebuildIndexLocked()
}

func (s *sliceRowSource) rebuildIndexLocked() {
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

func (s *sliceRowSource) Lines(top, num int) []table.Row {
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

func (s *sliceRowSource) Above(id string, n int) []table.Row {
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

func (s *sliceRowSource) Below(id string, n int) []table.Row {
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

func (s *sliceRowSource) Len() int {
	s.mu.Lock()
	s.ensureLocked()
	ln := len(s.rows)
	s.mu.Unlock()
	return ln
}

func (s *sliceRowSource) Find(id string) (int, table.Row, bool) {
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

func (s *sliceRowSource) ItemByID(id string) (Item, bool) {
	s.mu.Lock()
	s.ensureLocked()
	it, ok := s.items[id]
	s.mu.Unlock()
	return it, ok
}

func (s *sliceRowSource) MarkDirty() {
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
}
