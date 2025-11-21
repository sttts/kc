package table

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func mkCols(n int, w int) []Column {
	cols := make([]Column, n)
	for i := 0; i < n; i++ {
		cols[i] = Column{Title: fmt.Sprintf("C%02d", i), Width: w}
	}
	return cols
}

func mkRow(id string, cols int) Row {
	r := SimpleRow{ID: id}
	for i := 0; i < cols; i++ {
		s := lipgloss.NewStyle()
		r.SetColumn(i, id, &s)
	}
	return r
}

func mkList(n, cols int) *SliceList {
	rows := make([]Row, 0, n)
	for i := 0; i < n; i++ {
		id := "id-" + pad2(i)
		rows = append(rows, mkRow(id, cols))
	}
	return NewSliceList(rows)
}

func pad2(i int) string { return fmt.Sprintf("%02d", i) }

// Horizontal panning is no longer supported; only Auto/Fit modes remain.

func TestRepositionOnDataChange_NextThenPrev(t *testing.T) {
	cols := mkCols(3, 6)
	list := mkList(5, 3) // ids: id-00..id-04
	ctx := t.Context()
	bt := NewBigTable(cols, list, 60, 10)
	bt.SetMode(ctx, ModeFit)
	bt.Refresh(ctx)
	// Move cursor to index 2 (id-02)
	bt.cursor = 2
	bt.rebuildWindow(ctx)
	id, _ := bt.CurrentID(ctx)
	if id != "id-02" {
		t.Fatalf("want id-02, got %s", id)
	}
	// Remove id-02 -> should move to next (id-03)
	list.RemoveIDs("id-02")
	bt.SetList(ctx, list)
	id, _ = bt.CurrentID(ctx)
	if id != "id-03" {
		t.Fatalf("want id-03 after removal, got %s", id)
	}
	// Remove id-03 too -> should move to next (id-04)
	list.RemoveIDs("id-03")
	bt.SetList(ctx, list)
	id, _ = bt.CurrentID(ctx)
	if id != "id-04" {
		t.Fatalf("expected to land on id-04 after second removal, got %s", id)
	}
}

func TestBigTableVirtualScrollUsesBelow(t *testing.T) {
	ctx := context.Background()
	cols := []Column{{Title: "Name", Width: 10}}
	list := newWindowSpyList(100, 1)
	bt := NewBigTable(cols, list, 60, 10)
	bt.Refresh(ctx)
	vis := bt.bodyRowsHeight()
	if list.windowFetches() != 1 || list.lines[0].num != vis {
		t.Fatalf("expected initial Lines call for %d rows, got %v", vis, list.lines)
	}

	for i := 0; i < vis-1; i++ {
		bt.UpdateWithContext(ctx, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}
	list.resetCalls()

	bt.UpdateWithContext(ctx, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))

	if list.windowFetches() != 0 {
		t.Fatalf("expected no new Lines window calls when scrolling within buffer, got %v", list.lines)
	}
	if len(list.below) != 1 || list.below[0] != 1 {
		t.Fatalf("expected Below call for 1 row, got %v", list.below)
	}
	if bt.top == 0 {
		t.Fatalf("expected top to advance after scrolling past viewport")
	}
}

func TestBigTableVirtualScrollUsesAbove(t *testing.T) {
	ctx := context.Background()
	cols := []Column{{Title: "Name", Width: 10}}
	list := newWindowSpyList(100, 1)
	bt := NewBigTable(cols, list, 60, 10)
	bt.Refresh(ctx)
	vis := bt.bodyRowsHeight()
	for i := 0; i < vis; i++ {
		bt.UpdateWithContext(ctx, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}
	// Align cursor with the topmost visible row to force a window shift when moving up.
	bt.cursor = bt.top
	list.resetCalls()

	bt.UpdateWithContext(ctx, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))

	if list.windowFetches() != 0 {
		t.Fatalf("expected no new Lines window calls when scrolling up, got %v", list.lines)
	}
	if len(list.above) != 1 || list.above[0] != 1 {
		t.Fatalf("expected Above call for 1 row, got %v", list.above)
	}
}

func TestBigTableVirtualScrollFallbackOnSelect(t *testing.T) {
	ctx := context.Background()
	cols := []Column{{Title: "Name", Width: 10}}
	list := newWindowSpyList(500, 1)
	bt := NewBigTable(cols, list, 60, 10)
	bt.Refresh(ctx)
	list.resetCalls()

	if !bt.Select(ctx, "row-00300") {
		t.Fatalf("Select should succeed")
	}

	vis := bt.bodyRowsHeight()
	if call, ok := list.lastWindowCall(); !ok || call.num != vis {
		t.Fatalf("expected fresh Lines fetch for %d rows after select, got %v", vis, list.lines)
	}
	if len(list.below) != 0 || len(list.above) != 0 {
		t.Fatalf("expected no incremental calls on select jump, got above=%v below=%v", list.above, list.below)
	}
}

func TestBigTableVirtualScrollLargeList(t *testing.T) {
	ctx := context.Background()
	cols := []Column{{Title: "Name", Width: 10}}
	list := newWindowSpyList(20000, 1)
	bt := NewBigTable(cols, list, 80, 18)
	bt.Refresh(ctx)

	vis := bt.bodyRowsHeight()
	steps := 500
	for i := 0; i < steps; i++ {
		bt.UpdateWithContext(ctx, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}

	if wf := list.windowFetches(); wf != 1 {
		t.Fatalf("expected a single window fetch for large scroll, got %d (%v)", wf, list.lines)
	}

	expectedBelow := max(0, steps-(vis-1))
	if len(list.below) != expectedBelow {
		t.Fatalf("expected %d incremental Below fetches, got %d", expectedBelow, len(list.below))
	}

	if bt.top <= 0 {
		t.Fatalf("expected table top to advance after scrolling many rows")
	}
}

func TestBigTableSelectionClampsOnRemoval(t *testing.T) {
	ctx := context.Background()
	cols := mkCols(2, 6)
	list := mkList(4, 2)
	bt := NewBigTable(cols, list, 40, 8)
	bt.Refresh(ctx)

	if !bt.Select(ctx, "id-02") {
		t.Fatalf("expected to select id-02")
	}
	if id, ok := bt.CurrentID(ctx); !ok || id != "id-02" {
		t.Fatalf("cursor id = %q, want id-02", id)
	}

	list.RemoveIDs("id-02")
	bt.SetList(ctx, list)
	if id, ok := bt.CurrentID(ctx); !ok || id != "id-03" {
		t.Fatalf("cursor id after removal = %q, want id-03", id)
	}
}

func TestBigTableMultiSelectMarks(t *testing.T) {
	ctx := context.Background()
	cols := mkCols(1, 6)
	list := mkList(3, 1)
	bt := NewBigTable(cols, list, 30, 8)
	bt.Refresh(ctx)

	bt.Mark(ctx, "id-01", true)
	bt.Mark(ctx, "id-02", true)
	bt.Mark(ctx, "id-01", false)
	ids := bt.SelectedIDs()
	if len(ids) != 1 || ids[0] != "id-02" {
		t.Fatalf("selected IDs = %v, want [id-02]", ids)
	}
	bt.ClearMarks()
	if ids := bt.SelectedIDs(); len(ids) != 0 {
		t.Fatalf("expected marks cleared, got %v", ids)
	}
}

func TestBigTablePreservesMarksAfterSetList(t *testing.T) {
	ctx := context.Background()
	cols := mkCols(1, 6)
	list := mkList(3, 1)
	bt := NewBigTable(cols, list, 30, 8)
	bt.Refresh(ctx)

	bt.Mark(ctx, "id-00", true)
	bt.Mark(ctx, "id-01", true)

	list.RemoveIDs("id-01")
	bt.SetList(ctx, list)

	ids := bt.SelectedIDs()
	if len(ids) != 1 || ids[0] != "id-00" {
		t.Fatalf("expected only id-00 mark, got %v", ids)
	}
}

type windowSpyList struct {
	rows  []Row
	index map[string]int

	lines []lineCall
	above []int
	below []int
}

type lineCall struct {
	top int
	num int
}

func newWindowSpyList(n, cols int) *windowSpyList {
	rows := make([]Row, n)
	index := make(map[string]int, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("row-%05d", i)
		row := mkRow(id, cols)
		rows[i] = row
		index[id] = i
	}
	return &windowSpyList{rows: rows, index: index}
}

func (s *windowSpyList) Lines(_ context.Context, top, num int) []Row {
	s.lines = append(s.lines, lineCall{top: top, num: num})
	if num <= 0 || top >= len(s.rows) {
		return nil
	}
	if top < 0 {
		top = 0
	}
	end := top + num
	if end > len(s.rows) {
		end = len(s.rows)
	}
	out := make([]Row, end-top)
	copy(out, s.rows[top:end])
	return out
}

func (s *windowSpyList) Above(_ context.Context, rowID string, num int) []Row {
	s.above = append(s.above, num)
	idx, ok := s.index[rowID]
	if !ok || num <= 0 {
		return nil
	}
	start := idx - num
	if start < 0 {
		start = 0
	}
	out := make([]Row, idx-start)
	copy(out, s.rows[start:idx])
	return out
}

func (s *windowSpyList) Below(_ context.Context, rowID string, num int) []Row {
	s.below = append(s.below, num)
	idx, ok := s.index[rowID]
	if !ok || num <= 0 {
		return nil
	}
	start := idx + 1
	if start >= len(s.rows) {
		return nil
	}
	end := start + num
	if end > len(s.rows) {
		end = len(s.rows)
	}
	out := make([]Row, end-start)
	copy(out, s.rows[start:end])
	return out
}

func (s *windowSpyList) Len(context.Context) int { return len(s.rows) }

func (s *windowSpyList) Find(_ context.Context, rowID string) (int, Row, bool) {
	idx, ok := s.index[rowID]
	if !ok {
		return -1, nil, false
	}
	return idx, s.rows[idx], true
}

func (s *windowSpyList) resetCalls() {
	s.lines = nil
	s.above = nil
	s.below = nil
}
func (s *windowSpyList) windowFetches() int {
	count := 0
	for _, call := range s.lines {
		if call.num > 1 {
			count++
		}
	}
	return count
}

func (s *windowSpyList) lastWindowCall() (lineCall, bool) {
	for i := len(s.lines) - 1; i >= 0; i-- {
		if s.lines[i].num > 1 {
			return s.lines[i], true
		}
	}
	return lineCall{}, false
}

type countingList struct {
	rows  []Row
	calls int
}

func (c *countingList) Lines(ctx context.Context, top, num int) []Row {
	c.calls++
	return append([]Row(nil), c.rows...)
}

func (c *countingList) Above(context.Context, string, int) []Row { return nil }

func (c *countingList) Below(context.Context, string, int) []Row { return nil }

func (c *countingList) Len(context.Context) int { return len(c.rows) }

func (c *countingList) Find(context.Context, string) (int, Row, bool) {
	if len(c.rows) == 0 {
		return -1, nil, false
	}
	return 0, c.rows[0], true
}

func (c *countingList) reset() { c.calls = 0 }

func TestBigTableSetSizeNoOp(t *testing.T) {
	ctx := context.Background()
	row := SimpleRow{ID: "a"}
	style := lipgloss.NewStyle()
	row.SetColumn(0, "a", &style)
	list := &countingList{rows: []Row{row}}

	bt := NewBigTable([]Column{{Title: "A"}}, list, 40, 10)
	bt.SetList(ctx, list)
	list.reset()

	bt.SetSize(ctx, 80, 20)
	if list.calls == 0 {
		t.Fatalf("expected SetSize to rebuild window when size changes")
	}
	list.reset()
	bt.SetSize(ctx, 80, 20)
	if list.calls != 0 {
		t.Fatalf("expected SetSize to no-op when size unchanged, calls=%d", list.calls)
	}
}
