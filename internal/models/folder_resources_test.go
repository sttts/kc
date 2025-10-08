package models

import (
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
