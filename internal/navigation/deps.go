package navigation

import (
    table "github.com/sttts/kc/internal/table"
    "github.com/sttts/kc/pkg/resources"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

// Deps bundles the dependencies required by Folders to populate their rows.
// It is immutable and shared across folders created within the same context.
type Deps struct {
    // ResMgr resolves discovery (GVK↔GVR) and optional server-side tables.
    ResMgr *resources.Manager
    // Store provides List/Get over controller-runtime caches.
    Store  resources.StoreProvider
    // CtxName is the human label for the current context (for Folder titles/keys).
    CtxName string
}

// newEmptyList returns an empty table.List ready to be populated.
func newEmptyList() *table.SliceList { return table.NewSliceList(nil) }

