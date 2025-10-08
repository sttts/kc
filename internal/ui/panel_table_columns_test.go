package ui

import (
	"context"
	"testing"

	"github.com/sttts/kc/internal/models"
	table "github.com/sttts/kc/internal/table"
)

// fakeFolder is a minimal Folder implementation for testing column handling.
type fakeFolder struct {
	cols []table.Column
	list *table.SliceList
	path []string
}

// table.List implementation
func (f *fakeFolder) Lines(ctx context.Context, top, num int) []table.Row {
	return f.list.Lines(ctx, top, num)
}
func (f *fakeFolder) Above(ctx context.Context, id string, n int) []table.Row {
	return f.list.Above(ctx, id, n)
}
func (f *fakeFolder) Below(ctx context.Context, id string, n int) []table.Row {
	return f.list.Below(ctx, id, n)
}
func (f *fakeFolder) Len(ctx context.Context) int { return f.list.Len(ctx) }
func (f *fakeFolder) Find(ctx context.Context, id string) (int, table.Row, bool) {
	return f.list.Find(ctx, id)
}

// Folder metadata
func (f *fakeFolder) Columns() []table.Column                              { return f.cols }
func (f *fakeFolder) Path() []string                                       { return append([]string(nil), f.path...) }
func (f *fakeFolder) Key() string                                          { return "fake" }
func (f *fakeFolder) ItemByID(context.Context, string) (models.Item, bool) { return nil, false }

func newFakeFolder(cols []string, rows [][]string) *fakeFolder {
	tc := make([]table.Column, len(cols))
	for i := range cols {
		tc[i] = table.Column{Title: cols[i]}
	}
	tr := make([]table.Row, 0, len(rows))
	for i := range rows {
		tr = append(tr, table.SimpleRow{ID: rows[i][0], Cells: rows[i]})
	}
	return &fakeFolder{cols: tc, list: table.NewSliceList(tr), path: []string{"fake"}}
}

func TestPanelSetFolderUsesServerColumns(t *testing.T) {
	// Two columns present initially
	ff := newFakeFolder([]string{"Name", "Ready"}, [][]string{{"a", "0/1"}, {"b", "0/1"}})
	p := NewPanel("")
	p.UseFolder(true)
	p.SetDimensions(80, 20)
	p.SetFolder(ff, false)
	if p.bt == nil {
		t.Fatalf("bigtable not initialized")
	}
	if len(p.lastColTitles) < 2 {
		t.Fatalf("expected >=2 columns, got %v", p.lastColTitles)
	}
}

func TestPanelRefreshFolderRebuildsOnColumnChange(t *testing.T) {
	// Start with single column
	ff := newFakeFolder([]string{"Name"}, [][]string{{"a"}, {"b"}})
	p := NewPanel("")
	p.UseFolder(true)
	p.SetDimensions(80, 20)
	p.SetFolder(ff, false)
	if len(p.lastColTitles) != 1 {
		t.Fatalf("expected 1 column initially, got %d", len(p.lastColTitles))
	}
	// Change folder columns to simulate server-side table columns arriving
	ff.cols = []table.Column{{Title: "Name"}, {Title: "Ready"}, {Title: "Status"}}
	// Trigger refresh; Panel should compare and rebuild
	p.RefreshFolder()
	if want, got := 3, len(p.lastColTitles); got != want {
		t.Fatalf("expected %d columns after refresh, got %d (%v)", want, got, p.lastColTitles)
	}
}
