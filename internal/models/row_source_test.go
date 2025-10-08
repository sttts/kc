package models

import (
	"context"
	"testing"

	table "github.com/sttts/kc/internal/table"
)

func TestSliceRowSourcePopulateWithDirtyDuringPopulate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	src := newSliceRowSource(nil)
	var populateCalls int
	src.setPopulate(func(context.Context) ([]table.Row, error) {
		populateCalls++
		if populateCalls == 1 {
			src.MarkDirty()
		}
		return []table.Row{&table.SimpleRow{ID: "row-1"}}, nil
	})

	idx, row, ok := src.Find(ctx, "row-1")
	if !ok {
		t.Fatalf("expected to find row")
	}
	if idx != 0 {
		t.Fatalf("expected index 0, got %d", idx)
	}
	if id, _, _, exists := row.Columns(); !exists || id != "row-1" {
		t.Fatalf("unexpected row columns; id=%q exists=%v", id, exists)
	}
	if populateCalls != 2 {
		t.Fatalf("expected populate to run twice, got %d", populateCalls)
	}
}
