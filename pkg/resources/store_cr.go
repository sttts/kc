package resources

import (
    "context"
    "fmt"
    "time"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
    kcache "k8s.io/client-go/tools/cache"
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

// Watch streams object changes using the controller-runtime cache informer for PartialObjectMetadata of the target GVK.
func (s *CRReadOnlyStore) Watch(ctx context.Context, key StoreKey) (<-chan Event, context.CancelFunc, error) {
    e, err := s.pool.GetOrCreate(s.key)
    if err != nil { return nil, nil, err }
    // Resolve GVK for the given GVR via the RESTMapper.
    // Prefer the first kind returned.
    var gvk schema.GroupVersionKind
    kinds, err := e.cluster.GetRESTMapper().KindsFor(key.GVR)
    if err != nil || len(kinds) == 0 {
        return nil, nil, fmt.Errorf("map GVR to GVK: %w", err)
    }
    gvk = kinds[0]

    // Build a PartialObjectMetadata with the resolved GVK.
    meta := &metav1.PartialObjectMetadata{}
    meta.SetGroupVersionKind(gvk)
    inf, err := e.cache.GetInformer(ctx, meta)
    if err != nil { return nil, nil, fmt.Errorf("get informer: %w", err) }

    out := make(chan Event, 64)
    stopCtx, cancel := context.WithCancel(ctx)

    // Register handlers and filter by namespace if requested.
    _, err = inf.AddEventHandler(kcache.ResourceEventHandlerFuncs{
        AddFunc: func(obj interface{}) {
            if pom, ok := obj.(*metav1.PartialObjectMetadata); ok {
                if key.Namespace == "" || pom.GetNamespace() == key.Namespace {
                    out <- Event{Type: Added, Object: pomToUnstructured(pom)}
                }
            }
        },
        UpdateFunc: func(oldObj, newObj interface{}) {
            if pom, ok := newObj.(*metav1.PartialObjectMetadata); ok {
                if key.Namespace == "" || pom.GetNamespace() == key.Namespace {
                    out <- Event{Type: Modified, Object: pomToUnstructured(pom)}
                }
            }
        },
        DeleteFunc: func(obj interface{}) {
            if pom, ok := obj.(*metav1.PartialObjectMetadata); ok {
                if key.Namespace == "" || pom.GetNamespace() == key.Namespace {
                    out <- Event{Type: Deleted, Object: pomToUnstructured(pom)}
                }
            }
        },
    })
    if err != nil { cancel(); return nil, nil, fmt.Errorf("add handler: %w", err) }

    // Emit a Synced event once the informer syncs, then keep streaming; close on cancel.
    go func() {
        ticker := time.NewTicker(100 * time.Millisecond)
        defer ticker.Stop()
        syncedSent := false
        for {
            select {
            case <-stopCtx.Done():
                close(out)
                return
            case <-ticker.C:
                if !syncedSent && inf.HasSynced() {
                    out <- Event{Type: Synced}
                    syncedSent = true
                }
            }
        }
    }()

    return out, cancel, nil
}

// pomToUnstructured maps PartialObjectMetadata to a minimal unstructured with metadata.
func pomToUnstructured(pom *metav1.PartialObjectMetadata) *unstructured.Unstructured {
    u := &unstructured.Unstructured{}
    // apiVersion/kind
    if gv := pom.GetObjectKind().GroupVersionKind().GroupVersion().String(); gv != "/" {
        u.SetAPIVersion(gv)
    }
    u.SetKind(pom.GetObjectKind().GroupVersionKind().Kind)
    // metadata
    u.SetName(pom.GetName())
    u.SetNamespace(pom.GetNamespace())
    u.SetUID(pom.GetUID())
    u.SetResourceVersion(pom.GetResourceVersion())
    u.SetGeneration(pom.GetGeneration())
    u.SetCreationTimestamp(pom.GetCreationTimestamp())
    u.SetLabels(pom.GetLabels())
    u.SetAnnotations(pom.GetAnnotations())
    return u
}

// CRStoreProvider adapts a ClusterPool as a StoreProvider.
type CRStoreProvider struct{ pool *ClusterPool; key ClusterKey }

func NewCRStoreProvider(pool *ClusterPool, key ClusterKey) *CRStoreProvider { return &CRStoreProvider{pool: pool, key: key} }

func (p *CRStoreProvider) Store() ReadOnlyStore { return NewCRReadOnlyStore(p.pool, p.key) }
