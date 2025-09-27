package resources

import (
    "context"
    "fmt"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/client-go/dynamic"
)

// CRReadOnlyStore implements ReadOnlyStore using controller-runtime clusters from a ClusterPool.
// List uses a dynamic client for simplicity; Watch will be implemented via cache informers.
type CRReadOnlyStore struct {
    pool *ClusterPool
    key  ClusterKey
}

func NewCRReadOnlyStore(pool *ClusterPool, key ClusterKey) *CRReadOnlyStore {
    return &CRReadOnlyStore{pool: pool, key: key}
}

// dyn returns a dynamic client for the cluster key.
func (s *CRReadOnlyStore) dyn() (dynamic.Interface, error) {
    e, err := s.pool.GetOrCreate(s.key)
    if err != nil { return nil, err }
    // NOTE: We need a dynamic client. controller-runtime cluster does not expose it directly; create one from the cluster config.
    dc, err := dynamic.NewForConfig(e.cluster.GetConfig())
    if err != nil { return nil, fmt.Errorf("dynamic client: %w", err) }
    s.pool.Touch(e)
    return dc, nil
}

// List returns a snapshot of objects from the API server or cache.
func (s *CRReadOnlyStore) List(ctx context.Context, key StoreKey) (*unstructured.UnstructuredList, error) {
    // For now, use dynamic client. A future iteration can switch to cache.List once types are registered in the scheme.
    dc, err := s.dyn()
    if err != nil { return nil, err }
    var ri dynamic.ResourceInterface
    if key.Namespace != "" { ri = dc.Resource(key.GVR).Namespace(key.Namespace) } else { ri = dc.Resource(key.GVR) }
    lst, err := ri.List(ctx, metav1.ListOptions{})
    if err != nil { return nil, err }
    return lst, nil
}

// Watch is a placeholder; it will be implemented via cache informers delivering events.
func (s *CRReadOnlyStore) Watch(ctx context.Context, key StoreKey) (<-chan Event, context.CancelFunc, error) {
    ch := make(chan Event)
    cancel := func() { close(ch) }
    return ch, cancel, fmt.Errorf("watch not implemented yet")
}

// CRStoreProvider adapts a ClusterPool as a StoreProvider.
type CRStoreProvider struct{ pool *ClusterPool; key ClusterKey }

func NewCRStoreProvider(pool *ClusterPool, key ClusterKey) *CRStoreProvider { return &CRStoreProvider{pool: pool, key: key} }

func (p *CRStoreProvider) Store() ReadOnlyStore { return NewCRReadOnlyStore(p.pool, p.key) }
