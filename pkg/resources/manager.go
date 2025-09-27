package resources

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
    "sync"
    "time"

    "github.com/sttts/kc/pkg/handlers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/client-go/discovery"
    "k8s.io/client-go/discovery/cached/memory"
    "k8s.io/client-go/dynamic"
    "k8s.io/client-go/rest"
    "k8s.io/client-go/restmapper"
    "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

// Manager manages Kubernetes resources using controller-runtime
type Manager struct {
	cluster  cluster.Cluster
	client   client.Client
	cache    cache.Cache
	handlers map[schema.GroupVersionKind]handlers.ResourceHandler
	mutex    sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	// lazy-initialized helpers for discovery and dynamic operations
	discoOnce sync.Once
	disco     discovery.CachedDiscoveryInterface
	mapper    *restmapper.DeferredDiscoveryRESTMapper
	dyn       dynamic.Interface
}

// NewManager creates a new resource manager
func NewManager(config *rest.Config) (*Manager, error) {
	// Create a cluster
	c, err := cluster.New(config, func(o *cluster.Options) {
		// Configure cluster options if needed
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		cluster:  c,
		client:   c.GetClient(),
		cache:    c.GetCache(),
		handlers: make(map[schema.GroupVersionKind]handlers.ResourceHandler),
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// Start starts the manager
func (m *Manager) Start() error {
    // Start the cluster asynchronously; Start blocks until stop.
    go func() {
        // Error is captured only for logging; callers should operate even if cache isn't fully warm yet.
        _ = m.cluster.Start(m.ctx)
    }()

    // Best-effort: wait briefly for cache sync. If no informers are registered yet, this will return immediately.
    synced := make(chan struct{})
    go func() {
        m.cache.WaitForCacheSync(m.ctx)
        close(synced)
    }()
    select {
    case <-synced:
    case <-time.After(2 * time.Second):
        // Proceed without blocking; cache will continue warming in background.
    }

    // Start discovery refresh loop (invalidate cached discovery and reset RESTMapper periodically)
    go m.discoveryRefresher(30 * time.Second)
    return nil
}

// Stop stops the manager
func (m *Manager) Stop() {
    m.cancel()
}

// discoveryRefresher periodically invalidates discovery and resets the RESTMapper to pick up CRDs and API changes.
func (m *Manager) discoveryRefresher(interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-m.ctx.Done():
            return
        case <-ticker.C:
            // Lazily initialize discovery if needed, then invalidate caches.
            if err := m.ensureDiscovery(); err == nil {
                if m.disco != nil {
                    m.disco.Invalidate()
                }
                if m.mapper != nil {
                    m.mapper.Reset()
                }
            }
        }
    }
}

// Client returns the client
func (m *Manager) Client() client.Client {
	return m.client
}

// Cache returns the cache
func (m *Manager) Cache() cache.Cache {
	return m.cache
}

// Cluster returns the cluster
func (m *Manager) Cluster() cluster.Cluster {
	return m.cluster
}

// ensureDiscovery initializes discovery, rest mapper, and dynamic client lazily
func (m *Manager) ensureDiscovery() error {
	var initErr error
	m.discoOnce.Do(func() {
		dc, err := discovery.NewDiscoveryClientForConfig(m.cluster.GetConfig())
		if err != nil {
			initErr = fmt.Errorf("failed to create discovery client: %w", err)
			return
		}
		m.disco = memory.NewMemCacheClient(dc)
		m.mapper = restmapper.NewDeferredDiscoveryRESTMapper(m.disco)
		m.dyn, err = dynamic.NewForConfig(m.cluster.GetConfig())
		if err != nil {
			initErr = fmt.Errorf("failed to create dynamic client: %w", err)
			return
		}
	})
	return initErr
}

// GVKToGVR resolves a GroupVersionKind to a GroupVersionResource using discovery
func (m *Manager) GVKToGVR(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	if err := m.ensureDiscovery(); err != nil {
		return schema.GroupVersionResource{}, err
	}
	mapping, err := m.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("failed to map %s to GVR: %w", gvk.String(), err)
	}
	return mapping.Resource, nil
}

// ListByGVK lists resources generically using unstructured objects for the given GVK
func (m *Manager) ListByGVK(gvk schema.GroupVersionKind, namespace string) (*unstructured.UnstructuredList, error) {
	if err := m.ensureDiscovery(); err != nil {
		return nil, err
	}
	gvr, err := m.GVKToGVR(gvk)
	if err != nil {
		return nil, err
	}
	var ri dynamic.ResourceInterface
	if namespace != "" {
		ri = m.dyn.Resource(gvr).Namespace(namespace)
	} else {
		ri = m.dyn.Resource(gvr)
	}
	list, err := ri.List(m.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list %s: %w", gvk.String(), err)
	}
	return list, nil
}

// ListNamespaces lists namespaces generically without relying on typed clients
func (m *Manager) ListNamespaces() (*unstructured.UnstructuredList, error) {
	// Core v1 Namespace
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
	return m.ListByGVK(gvk, "")
}

// RegisterHandler registers a resource handler
func (m *Manager) RegisterHandler(gvk schema.GroupVersionKind, handler handlers.ResourceHandler) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.handlers[gvk] = handler
}

// GetHandler returns the handler for a specific GVK
func (m *Manager) GetHandler(gvk schema.GroupVersionKind) (handlers.ResourceHandler, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	handler, exists := m.handlers[gvk]
	if !exists {
		return nil, fmt.Errorf("no handler registered for GVK %v", gvk)
	}

	return handler, nil
}

// GetSupportedResources returns a list of supported resource types from the cluster
func (m *Manager) GetSupportedResources() ([]schema.GroupVersionKind, error) {
	// Create a discovery client
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(m.cluster.GetConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	// Get all API resources
	apiResources, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return nil, fmt.Errorf("failed to get server resources: %w", err)
	}

	var gvks []schema.GroupVersionKind
	for _, apiResourceList := range apiResources {
		gv, err := schema.ParseGroupVersion(apiResourceList.GroupVersion)
		if err != nil {
			continue // Skip invalid group versions
		}

		for _, apiResource := range apiResourceList.APIResources {
			// Skip subresources and non-resource types
			if isSubresource(apiResource.Name) || isNonResourceType(apiResource.Kind) {
				continue
			}

			gvk := schema.GroupVersionKind{
				Group:   gv.Group,
				Version: gv.Version,
				Kind:    apiResource.Kind,
			}
			gvks = append(gvks, gvk)
		}
	}

	return gvks, nil
}

// GetResourceInfos returns API resource infos including GVK, plural resource name, and namespaced flag.
func (m *Manager) GetResourceInfos() ([]ResourceInfo, error) {
    discoveryClient, err := discovery.NewDiscoveryClientForConfig(m.cluster.GetConfig())
    if err != nil {
        return nil, fmt.Errorf("failed to create discovery client: %w", err)
    }
    apiResources, err := discoveryClient.ServerPreferredResources()
    if err != nil {
        return nil, fmt.Errorf("failed to get server resources: %w", err)
    }
    var infos []ResourceInfo
    for _, apiResourceList := range apiResources {
        gv, err := schema.ParseGroupVersion(apiResourceList.GroupVersion)
        if err != nil { continue }
        for _, apiResource := range apiResourceList.APIResources {
            if isSubresource(apiResource.Name) || isNonResourceType(apiResource.Kind) { continue }
            infos = append(infos, ResourceInfo{
                GVK: schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: apiResource.Kind},
                Resource: apiResource.Name, // plural
                Namespaced: apiResource.Namespaced,
            })
        }
    }
    return infos, nil
}

// restClientForGV constructs a REST client for a specific GroupVersion using the cluster config.
func (m *Manager) restClientForGV(gv schema.GroupVersion) (*rest.RESTClient, error) {
    cfg := rest.CopyConfig(m.cluster.GetConfig())
    cfg.GroupVersion = &gv
    if gv.Group == "" {
        cfg.APIPath = "/api"
    } else {
        cfg.APIPath = "/apis"
    }
    cfg.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
    return rest.RESTClientFor(cfg)
}

// ListTableByGVR retrieves a server-side Table for the given resource using Accept negotiation.
// Falls back to a JSON decode of the response into metav1.Table.
func (m *Manager) ListTableByGVR(ctx context.Context, gvr schema.GroupVersionResource, namespace string) (*metav1.Table, error) {
    rc, err := m.restClientForGV(schema.GroupVersion{Group: gvr.Group, Version: gvr.Version})
    if err != nil {
        return nil, fmt.Errorf("rest client: %w", err)
    }
    req := rc.Get().Resource(gvr.Resource)
    if namespace != "" {
        req = req.Namespace(namespace)
    }
    // Ask for Table with JSON fallback per K8s API content negotiation.
    req.SetHeader("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1, application/json")
    data, err := req.DoRaw(ctx)
    if err != nil {
        return nil, fmt.Errorf("list table: %w", err)
    }
    var table metav1.Table
    if err := json.Unmarshal(data, &table); err != nil {
        return nil, fmt.Errorf("decode table: %w", err)
    }
    return &table, nil
}

// isSubresource checks if a resource name indicates a subresource
func isSubresource(name string) bool {
	// Subresources typically contain a slash (e.g., "pods/log", "pods/status")
	return strings.Contains(name, "/")
}

// isNonResourceType checks if a kind represents a non-resource type
func isNonResourceType(kind string) bool {
	nonResourceTypes := map[string]bool{
		"Status":                    true,
		"List":                      true,
		"WatchEvent":                true,
		"APIGroup":                  true,
		"APIVersion":                true,
		"APIResourceList":           true,
		"CreateOptions":             true,
		"UpdateOptions":             true,
		"DeleteOptions":             true,
		"PatchOptions":              true,
		"GetOptions":                true,
		"Table":                     true,
		"PartialObjectMetadata":     true,
		"PartialObjectMetadataList": true,
	}

	return nonResourceTypes[kind]
}
// ResourceInfo describes a discoverable API resource kind.
type ResourceInfo struct {
    GVK        schema.GroupVersionKind
    Resource   string // plural resource name (e.g., pods)
    Namespaced bool
}
