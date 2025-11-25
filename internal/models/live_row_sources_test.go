package models

import (
	"context"
	"errors"
	"testing"

	table "github.com/sttts/kc/internal/table"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

}

func TestLiveObjectRowSourceStopsListingAfterForbidden(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	var populateCalls int
	src := newLiveObjectRowSourceWithHooks(
		func(context.Context) ([]table.Row, error) {
			populateCalls++
			return nil, apierrors.NewForbidden(schema.GroupResource{Group: gvr.Group, Resource: gvr.Resource}, "", errors.New("denied"))
		},
		func() {},
		nil,
	)
	src.setTarget(liveSourceTarget{gvr: gvr, namespace: "default"})

	ctx := t.Context()
	if rows := src.Lines(ctx, 0, 5); rows != nil {
		t.Fatalf("expected no rows on forbidden list, got %d", len(rows))
	}
	if populateCalls != 1 {
		t.Fatalf("expected one populate attempt, got %d", populateCalls)
	}

	src.MarkDirty()
	if rows := src.Lines(ctx, 0, 5); rows != nil {
		t.Fatalf("expected no rows after forbidden list retry, got %d", len(rows))
	}
	if populateCalls != 1 {
		t.Fatalf("expected forbidden list to suppress retries, got %d calls", populateCalls)
	}
}

func TestLiveObjectRowSourceStopsWatchRetriesAfterForbidden(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	var populateCalls int
	var watchCalls int
	src := newLiveObjectRowSourceWithHooks(
		func(context.Context) ([]table.Row, error) {
			populateCalls++
			return []table.Row{}, nil
		},
		func() {},
		func(func(), func()) (func(), error) {
			watchCalls++
			return nil, apierrors.NewForbidden(schema.GroupResource{Group: gvr.Group, Resource: gvr.Resource}, "", errors.New("no watch"))
		},
	)
	src.setTarget(liveSourceTarget{gvr: gvr, namespace: "default"})

	ctx := t.Context()
	_ = src.Lines(ctx, 0, 5)
	if watchCalls != 1 {
		t.Fatalf("expected one watch attempt, got %d", watchCalls)
	}
	if populateCalls != 1 {
		t.Fatalf("expected populate to proceed once despite watch forbidden, got %d", populateCalls)
	}

	src.MarkDirty()
	_ = src.Lines(ctx, 0, 5)
	if watchCalls != 1 {
		t.Fatalf("expected watch retries to be suppressed, got %d attempts", watchCalls)
	}
	if populateCalls != 2 {
		t.Fatalf("expected refresh to skip watch and repopulate, got %d populate calls", populateCalls)
	}
}
