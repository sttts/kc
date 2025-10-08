package models

import (
	"context"
	"testing"

	table "github.com/sttts/kc/internal/table"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
)

func TestNewLiveKeyRowSourceRefresh(t *testing.T) {
	rowsSet := [][]table.Row{
		{
			NewSimpleItem("cm/key1", []string{"key1"}, []string{"cm", "key1"}, WhiteStyle()),
			NewSimpleItem("cm/key2", []string{"key2"}, []string{"cm", "key2"}, WhiteStyle()),
		},
		{
			NewSimpleItem("cm/key3", []string{"key3"}, []string{"cm", "key3"}, WhiteStyle()),
		},
	}

	idx := 0
	populateCalls := 0
	dirtyCalls := 0

	src := newLiveKeyRowSource(
		Deps{},
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"},
		"default",
		"example",
		func(context.Context) ([]table.Row, error) {
			populateCalls++
			return rowsSet[idx], nil
		},
		func() { dirtyCalls++ },
	)

	ctx := t.Context()
	first := src.Lines(ctx, 0, 10)
	if len(first) != len(rowsSet[0]) {
		t.Fatalf("expected %d rows, got %d", len(rowsSet[0]), len(first))
	}
	if populateCalls != 1 {
		t.Fatalf("expected one populate call, got %d", populateCalls)
	}

	idx = 1
	src.MarkDirty()
	if dirtyCalls != 1 {
		t.Fatalf("expected dirty callback once, got %d", dirtyCalls)
	}

	second := src.Lines(ctx, 0, 10)
	if len(second) != len(rowsSet[1]) {
		t.Fatalf("expected %d rows after refresh, got %d", len(rowsSet[1]), len(second))
	}
	if populateCalls != 2 {
		t.Fatalf("expected populate to refresh, got %d calls", populateCalls)
	}
}

func TestPodRowSourcesRefresh(t *testing.T) {
	rowsSet := [][]table.Row{
		{
			NewSimpleItem("section/containers", []string{"/containers"}, []string{"pod", "containers"}, WhiteStyle()),
			NewSimpleItem("section/init", []string{"/init"}, []string{"pod", "init"}, WhiteStyle()),
		},
		{
			NewSimpleItem("section/containers", []string{"/containers"}, []string{"pod", "containers"}, WhiteStyle()),
		},
	}

	idx := 0
	populate := func(context.Context) ([]table.Row, error) {
		return rowsSet[idx], nil
	}
	ctx := t.Context()

	// Section row source
	sec := newPodSectionRowSource(Deps{}, "default", "pod", func(ctx context.Context) ([]table.Row, error) {
		return populate(ctx)
	}, func() {})
	if len(sec.Lines(ctx, 0, 10)) != len(rowsSet[0]) {
		t.Fatalf("unexpected section rows")
	}

	idx = 1
	sec.MarkDirty()
	if len(sec.Lines(ctx, 0, 10)) != len(rowsSet[1]) {
		t.Fatalf("section rows did not refresh")
	}

	// Container list row source
	idx = 0
	lst := newPodContainerRowSource(Deps{}, "default", "pod", containerKindPrimary, func(ctx context.Context) ([]table.Row, error) {
		return populate(ctx)
	}, func() {})
	if len(lst.Lines(ctx, 0, 10)) != len(rowsSet[0]) {
		t.Fatalf("unexpected list rows")
	}
	idx = 1
	lst.MarkDirty()
	if len(lst.Lines(ctx, 0, 10)) != len(rowsSet[1]) {
		t.Fatalf("list rows did not refresh")
	}

	// Pod logs row source simply wraps populate
	lg := newPodContainerLogRowSource(Deps{}, "default", "pod", "containers", func(context.Context) ([]table.Row, error) {
		return rowsSet[1], nil
	}, func() {})
	if len(lg.Lines(ctx, 0, 10)) != len(rowsSet[1]) {
		t.Fatalf("unexpected log rows")
	}
}
