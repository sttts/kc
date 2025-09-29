package navigation

import (
    "context"
    table "github.com/sttts/kc/internal/table"
    kccluster "github.com/sttts/kc/internal/cluster"
)

// Deps bundles the dependencies required by Folders to populate their rows.
// It is immutable and shared across folders created within the same context.
type Deps struct {
    // Cluster provides client/cache, RESTMapper and discovery helpers.
    Cl *kccluster.Cluster
    // Ctx is the context for all cluster operations.
    Ctx context.Context
    // CtxName is the human label for the current context (for Folder titles/keys).
    CtxName string
    // ListContexts returns available context names (optional; used by root Contexts folder).
    ListContexts func() []string
    // EnterContext returns a Folder for the selected context (optional).
    // Typically returns a ContextRootFolder bound to the new context's cluster.
    // basePath is the absolute path segments to the target (e.g., ["contexts", name]).
    EnterContext func(name string, basePath []string) (Folder, error)
}

// newEmptyList returns an empty table.List ready to be populated.
func newEmptyList() *table.SliceList { return table.NewSliceList(nil) }
