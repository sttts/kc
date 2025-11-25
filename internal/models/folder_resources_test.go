package models

import (
	"strings"
	"testing"

	"github.com/sttts/kc/pkg/appconfig"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestResourcesFolderFinalizeMarksDirtyOnlyOnChange(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	cfg := appconfig.Default()
	cfg.Resources.ShowNonEmptyOnly = false

	deps := Deps{
		AppConfig: cfg,
		Ctx:       ctx,
	}

	base := NewBaseFolder(deps, nil, nil)
	folder := NewResourcesFolder(base)
	folder.BaseFolder.clearDirty()

	spec := resourceGroupSpec{
		id:        "g/v/res",
		cells:     []string{"res", "g/v", ""},
		path:      []string{"res"},
		detail:    "res (g/v)",
		gvr:       schema.GroupVersionResource{Group: "g", Version: "v", Resource: "res"},
		namespace: "",
		watchable: false,
	}

	folder.finalize(ctx, []resourceGroupSpec{spec})
	if !folder.BaseFolder.IsDirty() {
		t.Fatalf("expected base folder to be marked dirty on first finalize")
	}

	folder.BaseFolder.clearDirty()

	folder.finalize(ctx, []resourceGroupSpec{spec})
	if folder.BaseFolder.IsDirty() {
		t.Fatalf("expected base folder to remain clean when nothing changed")
	}

	folder.BaseFolder.clearDirty()

	updated := spec
	updated.cells = []string{"res*", "g/v", ""}
	folder.finalize(ctx, []resourceGroupSpec{updated})
	if !folder.BaseFolder.IsDirty() {
		t.Fatalf("expected base folder to be marked dirty when spec changes")
	}

	folder.BaseFolder.clearDirty()

	folder.finalize(ctx, nil)
	if !folder.BaseFolder.IsDirty() {
		t.Fatalf("expected base folder to be marked dirty when specs cleared")
	}
}

func TestResourcesFolderFinalizeVisibilityChanges(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	cfg := appconfig.Default()
	cfg.Resources.ShowNonEmptyOnly = true

	deps := Deps{
		AppConfig: cfg,
		Ctx:       ctx,
	}

	base := NewBaseFolder(deps, nil, nil)
	folder := NewResourcesFolder(base)
	folder.BaseFolder.clearDirty()

	spec := resourceGroupSpec{
		id:        "g/v/res",
		cells:     []string{"res", "g/v", ""},
		path:      []string{"res"},
		detail:    "res (g/v)",
		gvr:       schema.GroupVersionResource{Group: "g", Version: "v", Resource: "res"},
		namespace: "",
		watchable: false,
	}

	// First run: resource is filtered (watchable false => Empty() == true). Should mark dirty once.
	folder.finalize(ctx, []resourceGroupSpec{spec})
	if !folder.BaseFolder.IsDirty() {
		t.Fatalf("expected initial finalize to mark dirty even when filtered")
	}

	folder.BaseFolder.clearDirty()

	// Second run with identical data should not toggle dirty again.
	folder.finalize(ctx, []resourceGroupSpec{spec})
	if folder.BaseFolder.IsDirty() {
		t.Fatalf("expected finalize to remain clean when filtered result unchanged")
	}
}

func TestResourcesFolderToggleShowNonEmpty(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	cfg := appconfig.Default()
	cfg.Resources.ShowNonEmptyOnly = true

	deps := Deps{
		AppConfig: cfg,
		Ctx:       ctx,
	}

	base := NewBaseFolder(deps, nil, nil)
	folder := NewResourcesFolder(base)
	spec := resourceGroupSpec{
		id:        "g/v/res",
		cells:     []string{"res", "g/v", ""},
		path:      []string{"res"},
		detail:    "res (g/v)",
		gvr:       schema.GroupVersionResource{Group: "g", Version: "v", Resource: "res"},
		namespace: "",
		watchable: false,
	}

	rows := folder.finalize(ctx, []resourceGroupSpec{spec})
	if len(rows) != 0 {
		t.Fatalf("expected no rows when showNonEmptyOnly is true")
	}

	cfg.Resources.ShowNonEmptyOnly = false
	folder.ApplyResourceViewOptions(false, cfg.Resources.Order, cfg.Resources.Favorites)
	rows = folder.finalize(ctx, []resourceGroupSpec{spec})
	if len(rows) != 1 {
		t.Fatalf("expected resource to be visible when showNonEmptyOnly is false; got %d rows", len(rows))
	}
}

func TestResourcesFolderMarksDirtyOnOrderChange(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	cfg := appconfig.Default()
	cfg.Resources.ShowNonEmptyOnly = false

	deps := Deps{
		AppConfig: cfg,
		Ctx:       ctx,
	}

	base := NewBaseFolder(deps, nil, nil)
	folder := NewResourcesFolder(base)

	specA := resourceGroupSpec{
		id:     "g/v/a",
		cells:  []string{"a", "g/v", ""},
		path:   []string{"a"},
		detail: "a (g/v)",
	}
	specB := resourceGroupSpec{
		id:     "g/v/b",
		cells:  []string{"b", "g/v", ""},
		path:   []string{"b"},
		detail: "b (g/v)",
	}

	// Initial populate: should mark dirty.
	folder.BaseFolder.clearDirty()
	folder.finalize(ctx, []resourceGroupSpec{specA, specB})
	if !folder.BaseFolder.IsDirty() {
		t.Fatalf("expected dirty after initial finalize")
	}

	// Clear dirty flag and reorder specs (simulate order change).
	folder.BaseFolder.clearDirty()
	folder.finalize(ctx, []resourceGroupSpec{specB, specA})
	if !folder.BaseFolder.IsDirty() {
		t.Fatalf("expected dirty when resource order changes")
	}
}

func TestResourcesFolderHidesForbiddenResources(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	cfg := appconfig.Default()
	cfg.Resources.ShowNonEmptyOnly = false

	deps := Deps{
		AppConfig: cfg,
		Ctx:       ctx,
	}

	base := NewBaseFolder(deps, nil, nil)
	folder := NewResourcesFolder(base)

	spec := resourceGroupSpec{
		id:        "g/v/res",
		cells:     []string{"res", "g/v", ""},
		path:      []string{"res"},
		detail:    "res (g/v)",
		gvr:       schema.GroupVersionResource{Group: "g", Version: "v", Resource: "res"},
		namespace: "",
		watchable: false,
	}

	// First pass to initialize the item.
	rows := folder.finalize(ctx, []resourceGroupSpec{spec})
	if len(rows) != 1 {
		t.Fatalf("expected resource to be visible initially, got %d rows", len(rows))
	}

	item := folder.items[spec.id]
	if item == nil {
		t.Fatalf("expected resource item to be tracked")
	}
	item.mu.Lock()
	item.peekBlocked = true
	item.mu.Unlock()

	rows = folder.finalize(ctx, []resourceGroupSpec{spec})
	if len(rows) != 0 {
		t.Fatalf("expected forbidden resource to be hidden, got %d rows", len(rows))
	}
}

func TestSortResourceEntriesOrders(t *testing.T) {
	t.Parallel()

	sample := []resourceEntry{
		{info: ResourceInfo{Resource: "certificates", GVK: schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1"}}, gvr: schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}},
		{info: ResourceInfo{Resource: "endpointslices", GVK: schema.GroupVersionKind{Group: "discovery.k8s.io", Version: "v1"}}, gvr: schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}},
		{info: ResourceInfo{Resource: "pods", GVK: schema.GroupVersionKind{Group: "", Version: "v1"}}, gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}},
	}

	check := func(order appconfig.ResourcesViewOrder, favorites []string, want []string) {
		entries := make([]resourceEntry, len(sample))
		copy(entries, sample)
		sortResourceEntries(entries, order, favoritesMap(favorites))
		got := make([]string, len(entries))
		for i, e := range entries {
			got[i] = e.info.Resource
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("order %s mismatch: got %v want %v", order, got, want)
		}
	}

	check(appconfig.OrderAlpha, nil, []string{"certificates", "endpointslices", "pods"})
	check(appconfig.OrderGroup, nil, []string{"pods", "certificates", "endpointslices"})
	check(appconfig.OrderFavorites, []string{"pods", "certificates"}, []string{"certificates", "pods", "endpointslices"})
}
