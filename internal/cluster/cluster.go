package cluster

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	metamapper "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	crcluster "sigs.k8s.io/controller-runtime/pkg/cluster"

	// Table-rendering cache integration
	tablecache "github.com/sttts/kc/internal/tablecache"
)

// Cluster is a thin extension around controller-runtime's Cluster that exposes
// a self-updating RESTMapper and convenience helpers.
type Cluster struct {
	crcluster.Cluster // embedded; promotes Client/Cache/Start/GetConfig, etc.

	disco      discovery.CachedDiscoveryInterface
	baseMapper metamapper.ResettableRESTMapper
	mapper     metamapper.RESTMapper
	dyn        dynamic.Interface

	// tableCache serves Row/RowList objects backed by server-side Table responses.
	tableCache crcache.Cache

	cancel  context.CancelFunc
	refresh time.Duration
}

// Option configures Cluster.
type Option func(*options)
type options struct {
	scheme  *runtime.Scheme
	refresh time.Duration
}

// WithScheme sets the runtime.Scheme used by the controller-runtime cluster.
func WithScheme(s *runtime.Scheme) Option { return func(o *options) { o.scheme = s } }

// WithRefreshInterval sets the discovery/RESTMapper refresh interval (default 30s).
func WithRefreshInterval(d time.Duration) Option { return func(o *options) { o.refresh = d } }

// New creates a new Cluster embedding controller-runtime's Cluster and wiring a cached discovery client
// plus a Resettable RESTMapper.
func New(cfg *rest.Config, opts ...Option) (*Cluster, error) {
	o := &options{scheme: scheme.Scheme, refresh: 30 * time.Second}
	for _, fn := range opts {
		fn(o)
	}

	// controller-runtime cluster using the default cache; we keep it unchanged.
	// We initialize discovery/mapper lazily in ensureDiscovery() before first use.
	cl, err := crcluster.New(cfg, func(co *crcluster.Options) {
		co.Scheme = o.scheme
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := &Cluster{Cluster: cl, cancel: cancel, refresh: o.refresh}
	// Pre-initialize discovery/mapper/dynamic client lazily so methods can be used early.
	_ = c.ensureDiscovery()

	// Build a dedicated table-aware cache for Row/RowList alongside the default cache.
	// Register Row/RowList types in the scheme so the cache can marshal them.
	_ = tablecache.AddToScheme(o.scheme)
	tcache, err := tablecache.NewFromOptions(cfg, crcache.Options{Scheme: o.scheme})
	if err != nil {
		return nil, err
	}
	c.tableCache = tcache

	// Kick off background refresh loop with a detached context; start/stop is managed by callers.
	go c.refreshLoop(ctx)
	return c, nil
}

// ensureDiscovery initializes discovery, RESTMapper, and dynamic client lazily.
func (c *Cluster) ensureDiscovery() error {
	if c.mapper != nil && c.baseMapper != nil && c.disco != nil && c.dyn != nil {
		return nil
	}
	dc, err := discovery.NewDiscoveryClientForConfig(c.GetConfig())
	if err != nil {
		return err
	}
	cached := memory.NewMemCacheClient(dc)
	base := restmapper.NewDeferredDiscoveryRESTMapper(cached)
	expander := restmapper.NewShortcutExpander(base, dc, func(string) {})
	dyn, err := dynamic.NewForConfig(c.GetConfig())
	if err != nil {
		return err
	}
	c.disco = cached
	c.baseMapper = base
	c.mapper = expander
	c.dyn = dyn
	return nil
}

func (c *Cluster) refreshLoop(ctx context.Context) {
	t := time.NewTicker(c.refresh)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// invalidate discovery and reset mapper
			if c.disco != nil {
				c.disco.Invalidate()
			}
			if c.baseMapper != nil {
				c.baseMapper.Reset()
			}
		}
	}
}

// Start delegates to controller-runtime Cluster.Start; it blocks until context is cancelled.
func (c *Cluster) Start(ctx context.Context) error {
	// Start the table cache in parallel to the embedded cluster.
	errCh := make(chan error, 2)
	go func() { errCh <- c.tableCache.Start(ctx) }()
	go func() { errCh <- c.Cluster.Start(ctx) }()
	// Return the first error; the other goroutine will exit on ctx cancel or due to the same failure.
	return <-errCh
}

// Stop cancels internal loops; users should cancel the Start() context as well.
func (c *Cluster) Stop() { c.cancel() }

// RESTMapper exposes the cluster's RESTMapper (with shortcuts).
func (c *Cluster) RESTMapper() metamapper.RESTMapper {
	_ = c.ensureDiscovery()
	return c.mapper
}

// Note: we intentionally do not wrap GetClient/GetCache/etc. from the embedded
// controller-runtime Cluster. Callers should use the embedded methods directly
// (e.g., c.GetClient(), c.GetCache()).

// ListTableByGVR retrieves a server-side Table for the given resource using Accept negotiation.
func (c *Cluster) ListTableByGVR(ctx context.Context, gvr schema.GroupVersionResource, namespace string) (*metav1.Table, error) {
	rc, err := c.restClientForGV(schema.GroupVersion{Group: gvr.Group, Version: gvr.Version})
	if err != nil {
		return nil, err
	}
	req := rc.Get().Resource(gvr.Resource)
	if namespace != "" {
		req = req.Namespace(namespace)
	}
	req.SetHeader("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1, application/json")
	data, err := req.DoRaw(ctx)
	if err != nil {
		return nil, err
	}
	var table metav1.Table
	if err := json.Unmarshal(data, &table); err != nil {
		return nil, err
	}
	return &table, nil
}

func (c *Cluster) restClientForGV(gv schema.GroupVersion) (*rest.RESTClient, error) {
	cfg := rest.CopyConfig(c.GetConfig())
	cfg.GroupVersion = &gv
	if gv.Group == "" {
		cfg.APIPath = "/api"
	} else {
		cfg.APIPath = "/apis"
	}
	cfg.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	return rest.RESTClientFor(cfg)
}

// -----------------------------------------------------------------------------
// Table-aware helpers using the tablecache-backed cache

// ListRowsByGVR lists resources as server-side table rows using the cache.
// It returns a RowList whose Columns describe the headers and whose Items carry
// the raw cell values. Callers can render cells or fall back when the server
// does not support Tables.
func (c *Cluster) ListRowsByGVR(ctx context.Context, gvr schema.GroupVersionResource, namespace string) (*tablecache.RowList, error) {
	// Resolve Kind for the Row target GVK
	_ = c.ensureDiscovery()
	gvk, err := c.RESTMapper().KindFor(gvr)
	if err != nil {
		return nil, err
	}
	rows := tablecache.NewRowList(gvk)
	if namespace != "" {
		if err := c.tableCache.List(ctx, rows, crclient.InNamespace(namespace)); err != nil {
			return nil, err
		}
	} else {
		if err := c.tableCache.List(ctx, rows); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

// GetRowByGVR fetches a single object rendered as a server-side table row.
// It extracts the row via the cache using the Row target GVK.
func (c *Cluster) GetRowByGVR(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*tablecache.Row, error) {
	_ = c.ensureDiscovery()
	gvk, err := c.RESTMapper().KindFor(gvr)
	if err != nil {
		return nil, err
	}
	row := tablecache.NewRow(gvk)
	key := crclient.ObjectKey{Namespace: namespace, Name: name}
	if err := c.tableCache.Get(ctx, key, row); err != nil {
		return nil, err
	}
	return row, nil
}

// Helpers ---------------------------------------------------------------------

// GVKToGVR maps a Kind to its resource using the RESTMapper.
func (c *Cluster) GVKToGVR(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	_ = c.ensureDiscovery()
	m, err := c.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	return m.Resource, nil
}

// ListByGVR lists objects using the cache-backed client and returns an UnstructuredList.
func (c *Cluster) ListByGVR(ctx context.Context, gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
	_ = c.ensureDiscovery()
	k, err := c.RESTMapper().KindFor(gvr)
	if err != nil {
		return nil, err
	}
	ul := &unstructured.UnstructuredList{}
	ul.SetGroupVersionKind(schema.GroupVersionKind{Group: k.Group, Version: k.Version, Kind: k.Kind + "List"})
	if namespace != "" {
		if err := c.GetClient().List(ctx, ul, crclient.InNamespace(namespace)); err != nil {
			return nil, err
		}
	} else {
		if err := c.GetClient().List(ctx, ul); err != nil {
			return nil, err
		}
	}
	return ul, nil
}

// HasAnyByGVR performs a lightweight peek (limit=1) to determine if at least one object exists for the GVR.
// It avoids starting informers so callers can filter empty resource groups cheaply.
func (c *Cluster) HasAnyByGVR(ctx context.Context, gvr schema.GroupVersionResource, namespace string) (bool, error) {
	if err := c.ensureDiscovery(); err != nil {
		return false, err
	}
	res := c.dyn.Resource(gvr)
	var iface dynamic.ResourceInterface
	if namespace != "" {
		iface = res.Namespace(namespace)
	} else {
		iface = res
	}
	list, err := iface.List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return false, err
	}
	return len(list.Items) > 0, nil
}

// GetByGVR fetches one object as Unstructured using the cache-backed client.
func (c *Cluster) GetByGVR(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	_ = c.ensureDiscovery()
	k, err := c.RESTMapper().KindFor(gvr)
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(k)
	key := crclient.ObjectKey{Namespace: namespace, Name: name}
	if err := c.GetClient().Get(ctx, key, u); err != nil {
		return nil, err
	}
	return u, nil
}

// ResourceInfo describes a discoverable API resource kind.
type ResourceInfo struct {
	GVK        schema.GroupVersionKind
	Resource   string
	Namespaced bool
	Verbs      []string
}

// GetResourceInfos returns API resource infos via discovery.
func (c *Cluster) GetResourceInfos() ([]ResourceInfo, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(c.GetConfig())
	if err != nil {
		return nil, err
	}
	lists, err := dc.ServerPreferredResources()
	if err != nil {
		return nil, err
	}
	var out []ResourceInfo
	for _, l := range lists {
		gv, err := schema.ParseGroupVersion(l.GroupVersion)
		if err != nil {
			continue
		}
		for _, ar := range l.APIResources {
			if ar.Name == "" || ar.Kind == "" {
				continue
			}
			if strings.Contains(ar.Name, "/") {
				continue
			}
			out = append(out, ResourceInfo{
				GVK:        schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: ar.Kind},
				Resource:   ar.Name,
				Namespaced: ar.Namespaced,
				Verbs:      ar.Verbs,
			})
		}
	}
	return out, nil
}
