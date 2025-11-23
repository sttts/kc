package cluster

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

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

	// storageCounts caches apiserver_storage_objects counts keyed by GVR.
	storageMu      sync.RWMutex
	storageCounts  map[schema.GroupResource]int
	storageFetched time.Time

	cancel  context.CancelFunc
	refresh time.Duration

	discoMu        sync.RWMutex
	discoListeners map[int]func()
	discoSeq       int
}

// Option configures Cluster.
type Option func(*options)
type options struct {
	scheme    *runtime.Scheme
	refresh   time.Duration
	namespace string
}

// WithScheme sets the runtime.Scheme used by the controller-runtime cluster.
func WithScheme(s *runtime.Scheme) Option { return func(o *options) { o.scheme = s } }

// WithRefreshInterval sets the discovery/RESTMapper refresh interval (default 30s).
func WithRefreshInterval(d time.Duration) Option { return func(o *options) { o.refresh = d } }

// WithNamespaceScope restricts the controller-runtime cache to a single namespace.
func WithNamespaceScope(ns string) Option {
	return func(o *options) {
		o.namespace = strings.TrimSpace(ns)
	}
}

// New creates a new Cluster embedding controller-runtime's Cluster and wiring a cached discovery client
// plus a Resettable RESTMapper.
func New(cfg *rest.Config, opts ...Option) (*Cluster, error) {
	o := &options{scheme: scheme.Scheme, refresh: 30 * time.Second}
	for _, fn := range opts {
		fn(o)
	}

	// Increase client-side QPS/Burst so the cache-backed client can issue frequent LIST calls without throttling.
	if cfg.QPS == 0 {
		cfg.QPS = 50
	}
	if cfg.Burst == 0 {
		cfg.Burst = 100
	}

	// controller-runtime cluster using the default cache; we keep it unchanged.
	// We initialize discovery/mapper lazily in ensureDiscovery() before first use.
	cacheHTTPClient, err := namespaceHTTPClient(cfg, o.namespace)
	if err != nil {
		return nil, err
	}

	cl, err := crcluster.New(cfg, func(co *crcluster.Options) {
		co.Scheme = o.scheme
		if cacheHTTPClient != nil {
			co.Cache.HTTPClient = cacheHTTPClient
		}
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
	tableCacheOpts := crcache.Options{Scheme: o.scheme}
	if cacheHTTPClient != nil {
		tableCacheOpts.HTTPClient = cacheHTTPClient
	}
	tcache, err := tablecache.NewFromOptions(cfg, tableCacheOpts)
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
			c.RefreshDiscovery()
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

// Dynamic returns the shared dynamic client backing the cluster.
func (c *Cluster) Dynamic() dynamic.Interface {
	_ = c.ensureDiscovery()
	return c.dyn
}

// DiscoveryClient exposes the cached discovery client backing the cluster.
func (c *Cluster) DiscoveryClient() discovery.CachedDiscoveryInterface {
	_ = c.ensureDiscovery()
	return c.disco
}

// RefreshDiscovery invalidates cached discovery information and notifies listeners.
func (c *Cluster) RefreshDiscovery() {
	if err := c.ensureDiscovery(); err != nil {
		return
	}
	if c.disco != nil {
		c.disco.Invalidate()
	}
	if c.baseMapper != nil {
		c.baseMapper.Reset()
	}
	c.notifyDiscoveryListeners()
}

// AddDiscoveryListener registers a callback invoked whenever discovery is refreshed.
// The returned function removes the listener when invoked.
func (c *Cluster) AddDiscoveryListener(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	c.discoMu.Lock()
	if c.discoListeners == nil {
		c.discoListeners = make(map[int]func())
	}
	id := c.discoSeq
	c.discoSeq++
	c.discoListeners[id] = fn
	c.discoMu.Unlock()
	return func() {
		c.discoMu.Lock()
		delete(c.discoListeners, id)
		c.discoMu.Unlock()
	}
}

func (c *Cluster) notifyDiscoveryListeners() {
	c.discoMu.RLock()
	if len(c.discoListeners) == 0 {
		c.discoMu.RUnlock()
		return
	}
	listeners := make([]func(), 0, len(c.discoListeners))
	for _, fn := range c.discoListeners {
		listeners = append(listeners, fn)
	}
	c.discoMu.RUnlock()
	for _, fn := range listeners {
		func() {
			defer func() { _ = recover() }()
			fn()
		}()
	}
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

func namespaceHTTPClient(cfg *rest.Config, namespace string) (*http.Client, error) {
	if namespace == "" {
		return nil, nil
	}
	copyCfg := rest.CopyConfig(cfg)
	transport, err := rest.TransportFor(copyCfg)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport: newNamespaceRoundTripper(namespace, transport),
		Timeout:   copyCfg.Timeout,
	}, nil
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
	log := ctrllog.FromContext(ctx).WithName("cluster")
	log.Info("client list", "gvr", gvr.String(), "namespace", namespace)
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
	log := ctrllog.FromContext(ctx).WithName("cluster")
	log.Info("client hasAny peek", "gvr", gvr.String(), "namespace", namespace, "limit", 1)
	res := c.dyn.Resource(gvr)
	var iface dynamic.ResourceInterface
	if namespace != "" {
		iface = res.Namespace(namespace)
	} else {
		iface = res
	}
	// Leave ResourceVersion empty to avoid forcing a quorum read; the apiserver may serve from cache.
	list, err := iface.List(ctx, metav1.ListOptions{Limit: 1, ResourceVersion: ""})
	if err != nil {
		return false, err
	}
	return len(list.Items) > 0, nil
}

// GetByGVR fetches one object as Unstructured using the cache-backed client.
func (c *Cluster) GetByGVR(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	_ = c.ensureDiscovery()
	log := ctrllog.FromContext(ctx).WithName("cluster")
	log.Info("client get", "gvr", gvr.String(), "namespace", namespace, "name", name)
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

// StorageCount returns the number of objects for a GVR based on apiserver_storage_objects metrics.
// When metrics are unavailable or stale, ok=false and callers should fall back to client peeks.
// When metrics are available and the GVR is missing from the metrics, count=0 and ok=true.
func (c *Cluster) StorageCount(ctx context.Context, gvr schema.GroupVersionResource, maxAge time.Duration) (int, bool) {
	if !c.refreshStorageMetrics(ctx, maxAge) {
		return 0, false
	}
	c.storageMu.RLock()
	defer c.storageMu.RUnlock()
	if c.storageCounts == nil {
		return 0, false
	}
	if cnt, ok := c.storageCounts[gvr.GroupResource()]; ok {
		return cnt, true
	}
	// Metrics present but GVR absent → treat as zero.
	return 0, true
}

func (c *Cluster) refreshStorageMetrics(ctx context.Context, maxAge time.Duration) bool {
	c.storageMu.RLock()
	ageOk := !c.storageFetched.IsZero() && time.Since(c.storageFetched) < maxAge && c.storageCounts != nil
	c.storageMu.RUnlock()
	if ageOk {
		return true
	}
	counts, err := c.fetchStorageMetrics(ctx)
	if err != nil {
		return false
	}
	c.storageMu.Lock()
	c.storageCounts = counts
	c.storageFetched = time.Now()
	c.storageMu.Unlock()
	return true
}

func (c *Cluster) fetchStorageMetrics(ctx context.Context) (map[schema.GroupResource]int, error) {
	cfg := rest.CopyConfig(c.GetConfig())
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, err
	}
	url := strings.TrimSuffix(cfg.Host, "/") + "/metrics"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/plain")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics: unexpected status %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	const prefix = "apiserver_storage_objects{"
	counts := make(map[schema.GroupResource]int)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rbrace := strings.Index(line, "}")
		if rbrace <= len(prefix) {
			continue
		}
		labelStr := line[len(prefix):rbrace]
		valStr := strings.TrimSpace(line[rbrace+1:])
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		labels := make(map[string]string)
		for _, kv := range strings.Split(labelStr, ",") {
			parts := strings.SplitN(strings.TrimSpace(kv), "=", 2)
			if len(parts) != 2 {
				continue
			}
			labels[strings.TrimSpace(parts[0])] = strings.Trim(parts[1], `"`)
		}
		group := labels["group"]
		resource := labels["resource"]
		if resource == "" {
			continue
		}
		gr := schema.GroupResource{Group: group, Resource: resource}
		counts[gr] = int(val)
	}
	return counts, scanner.Err()
}

// ResourceInfo describes a discoverable API resource kind.
type ResourceInfo struct {
	GVK        schema.GroupVersionKind
	Resource   string
	Namespaced bool
	Verbs      []string
	Categories []string
}

// GetResourceInfos returns API resource infos via discovery.
func (c *Cluster) GetResourceInfos() ([]ResourceInfo, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(c.GetConfig())
	if err != nil {
		return nil, err
	}
	lists, err := dc.ServerPreferredResources()
	if err != nil {
		// Allow partial discovery results when some aggregated APIs fail (e.g., metrics.k8s.io).
		if len(lists) == 0 {
			return nil, err
		}
		if _, ok := err.(*discovery.ErrGroupDiscoveryFailed); !ok {
			return nil, err
		}
		// For partial failures, continue with the successful lists and clear the error so callers
		// can proceed (metrics server down should not block the UI).
		err = nil
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
				Verbs:      append([]string(nil), ar.Verbs...),
				Categories: append([]string(nil), ar.Categories...),
			})
		}
	}
	return out, nil
}
