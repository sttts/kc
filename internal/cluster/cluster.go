package cluster

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    metamapper "k8s.io/apimachinery/pkg/api/meta"
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
)

// Cluster is a thin extension around controller-runtime's Cluster that exposes
// a self-updating RESTMapper and convenience helpers.
type Cluster struct {
    crcluster.Cluster // embedded; promotes Client/Cache/Start/GetConfig, etc.

    disco      discovery.CachedDiscoveryInterface
    baseMapper metamapper.ResettableRESTMapper
    mapper     metamapper.RESTMapper
    dyn        dynamic.Interface

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
    for _, fn := range opts { fn(o) }

    // controller-runtime cluster using our (to-be) mapper
    // We initialize discovery/mapper lazily in ensureDiscovery() before first use.
    cl, err := crcluster.New(cfg, func(co *crcluster.Options) {
        co.Scheme = o.scheme
    })
    if err != nil { return nil, err }

    ctx, cancel := context.WithCancel(context.Background())
    c := &Cluster{Cluster: cl, cancel: cancel, refresh: o.refresh}
    // Pre-initialize discovery/mapper/dynamic client lazily so methods can be used early.
    _ = c.ensureDiscovery()
    // Kick off background refresh loop with a detached context; start/stop is managed by callers.
    go c.refreshLoop(ctx)
    return c, nil
}

// ensureDiscovery initializes discovery, RESTMapper, and dynamic client lazily.
func (c *Cluster) ensureDiscovery() error {
    if c.mapper != nil && c.baseMapper != nil && c.disco != nil && c.dyn != nil { return nil }
    dc, err := discovery.NewDiscoveryClientForConfig(c.GetConfig())
    if err != nil { return err }
    cached := memory.NewMemCacheClient(dc)
    base := restmapper.NewDeferredDiscoveryRESTMapper(cached)
    expander := restmapper.NewShortcutExpander(base, dc)
    dyn, err := dynamic.NewForConfig(c.GetConfig())
    if err != nil { return err }
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
            if c.disco != nil { c.disco.Invalidate() }
            if c.baseMapper != nil { c.baseMapper.Reset() }
        }
    }
}

// Start delegates to controller-runtime Cluster.Start; it blocks until context is cancelled.
func (c *Cluster) Start(ctx context.Context) error {
    return c.Cluster.Start(ctx)
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
    if err != nil { return nil, err }
    req := rc.Get().Resource(gvr.Resource)
    if namespace != "" { req = req.Namespace(namespace) }
    req.SetHeader("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1, application/json")
    data, err := req.DoRaw(ctx)
    if err != nil { return nil, err }
    var table metav1.Table
    if err := json.Unmarshal(data, &table); err != nil { return nil, err }
    return &table, nil
}

func (c *Cluster) restClientForGV(gv schema.GroupVersion) (*rest.RESTClient, error) {
    cfg := rest.CopyConfig(c.GetConfig())
    cfg.GroupVersion = &gv
    if gv.Group == "" { cfg.APIPath = "/api" } else { cfg.APIPath = "/apis" }
    cfg.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
    return rest.RESTClientFor(cfg)
}
