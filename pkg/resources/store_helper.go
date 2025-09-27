package resources

import (
    "time"

    "github.com/sschimanski/kc/pkg/kubeconfig"
)

// NewStoreProviderForContext creates a StoreProvider bound to the given kubeconfig context.
// It also returns the underlying ClusterPool so callers can manage its lifecycle (e.g., Stop on shutdown).
// The pool keeps clusters alive for the provided TTL to avoid cache thrashing when switching contexts.
func NewStoreProviderForContext(ctx *kubeconfig.Context, ttl time.Duration) (StoreProvider, *ClusterPool) {
    pool := NewClusterPool(ttl)
    pool.Start()
    key := ClusterKey{KubeconfigPath: ctx.Kubeconfig.Path, ContextName: ctx.Name}
    return NewCRStoreProvider(pool, key), pool
}

