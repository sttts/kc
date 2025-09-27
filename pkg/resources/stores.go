package resources

import (
    "context"

    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

// StoreKey identifies a concrete resource collection in the cache.
// It intentionally uses native Kubernetes identity (GVR + namespace).
type StoreKey struct {
    GVR       schema.GroupVersionResource
    Namespace string // empty for cluster-scoped
}

// EventType is a minimal event indicator for list/watch updates.
type EventType string

const (
    Added    EventType = "Added"
    Modified EventType = "Modified"
    Deleted  EventType = "Deleted"
    Synced   EventType = "Synced" // initial list complete
)

// Event conveys an object change for UI consumption using unstructured objects.
type Event struct {
    Type   EventType
    Object *unstructured.Unstructured
}

// ReadOnlyStore exposes informer-backed access without re-wrapping client-go/controller-runtime.
// Implementations MUST be thin adapters over controller-runtime Cluster/Cache informers
// (start/stop on demand), one Cluster per kubeconfig+context. Avoid custom informer factories.
type ReadOnlyStore interface {
    // List returns the latest snapshot from cache.
    List(ctx context.Context, key StoreKey) (*unstructured.UnstructuredList, error)
    // Get returns a single object by name.
    Get(ctx context.Context, key StoreKey, name string) (*unstructured.Unstructured, error)
    // Watch streams object-level changes for the given key.
    // The cancel function must stop delivery and release resources.
    Watch(ctx context.Context, key StoreKey) (<-chan Event, context.CancelFunc, error)
}

// StoreProvider provides access to a ReadOnlyStore bound to a cluster/context.
// Higher layers (navigation/UI) remain agnostic of how informers are realized.
type StoreProvider interface {
    Store() ReadOnlyStore
}
