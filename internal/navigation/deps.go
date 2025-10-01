package navigation

import (
	"context"
	kccluster "github.com/sttts/kc/internal/cluster"
	table "github.com/sttts/kc/internal/table"
)

// Deps bundles the dependencies required by Folders to populate their rows.
// It is immutable and shared across folders created within the same context.
//
// Invariants:
//   - Cl must be non-nil and already started; folders rely on its client/cache.
//   - Ctx must be non-nil; it is passed to informer/client operations and used for logging.
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

	// ViewOptions returns live resource view options (sorting/filtering) that
	// affect how folders populate their rows (e.g., group listing order).
	ViewOptions func() ViewOptions
}

// newEmptyList returns an empty table.List ready to be populated.
func newEmptyList() *table.SliceList { return table.NewSliceList(nil) }

// ViewOptions influences folder population for resources listings.
type ViewOptions struct {
	ShowNonEmptyOnly bool
	// Order: "alpha", "group", or "favorites"
	Order string
	// Favorites: set of resource plural names to prioritize when Order=="favorites"
	Favorites map[string]bool
	// Columns: "normal" (priority 0 only) or "wide" (all server columns)
	Columns string
	// ObjectsOrder controls ordering within object lists:
	// "name", "-name", "creation", "-creation"
	ObjectsOrder string
}
