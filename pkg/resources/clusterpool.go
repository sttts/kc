package resources

import (
    "context"
    "fmt"
    "sync"
    "time"

    "k8s.io/client-go/tools/clientcmd"
    "sigs.k8s.io/controller-runtime/pkg/cache"
    "sigs.k8s.io/controller-runtime/pkg/cluster"
)

// ClusterKey identifies a controller-runtime Cluster by kubeconfig path and context name.
type ClusterKey struct {
    KubeconfigPath string
    ContextName    string
}

// clusterEntry holds a running cluster and metadata for lifecycle management.
type clusterEntry struct {
    key       ClusterKey
    cluster   cluster.Cluster
    cache     cache.Cache
    cancel    context.CancelFunc
    lastUsed  time.Time
    readyOnce sync.Once
    readyErr  error
}

// ClusterPool manages controller-runtime clusters per kubeconfig+context with idle eviction.
type ClusterPool struct {
    mu       sync.RWMutex
    entries  map[ClusterKey]*clusterEntry
    ttl      time.Duration
    closing  chan struct{}
    started  bool
}

// NewClusterPool creates a pool with the given idle TTL (e.g., 2 * time.Minute).
func NewClusterPool(ttl time.Duration) *ClusterPool {
    return &ClusterPool{entries: make(map[ClusterKey]*clusterEntry), ttl: ttl, closing: make(chan struct{})}
}

// Start launches the eviction loop. Call once.
func (p *ClusterPool) Start() {
    p.mu.Lock()
    if p.started {
        p.mu.Unlock(); return
    }
    p.started = true
    p.mu.Unlock()
    go p.evictLoop()
}

// Stop stops the eviction loop and tears down all clusters.
func (p *ClusterPool) Stop() {
    close(p.closing)
    p.mu.Lock()
    defer p.mu.Unlock()
    for k, e := range p.entries {
        e.cancel()
        delete(p.entries, k)
    }
}

// GetOrCreate returns a running cluster entry for the key, starting it if absent.
func (p *ClusterPool) GetOrCreate(key ClusterKey) (*clusterEntry, error) {
    p.mu.Lock()
    if e, ok := p.entries[key]; ok {
        e.lastUsed = time.Now()
        p.mu.Unlock()
        return e, nil
    }
    // Build rest.Config from kubeconfig+context.
    cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
        &clientcmd.ClientConfigLoadingRules{ExplicitPath: key.KubeconfigPath},
        &clientcmd.ConfigOverrides{CurrentContext: key.ContextName},
    ).ClientConfig()
    if err != nil {
        p.mu.Unlock()
        return nil, fmt.Errorf("build client config: %w", err)
    }
    // Create cluster
    c, err := cluster.New(cfg, func(o *cluster.Options) {})
    if err != nil {
        p.mu.Unlock()
        return nil, fmt.Errorf("create cluster: %w", err)
    }
    ctx, cancel := context.WithCancel(context.Background())
    e := &clusterEntry{key: key, cluster: c, cache: c.GetCache(), cancel: cancel, lastUsed: time.Now()}
    p.entries[key] = e
    p.mu.Unlock()

    // Start asynchronously; cache sync can be awaited by callers if needed.
    go func() {
        e.readyOnce.Do(func() {
            if err := c.Start(ctx); err != nil {
                e.readyErr = err
                return
            }
        })
    }()

    return e, nil
}

// Touch updates lastUsed for an entry (call after operations).
func (p *ClusterPool) Touch(e *clusterEntry) { p.mu.Lock(); e.lastUsed = time.Now(); p.mu.Unlock() }

func (p *ClusterPool) evictLoop() {
    ticker := time.NewTicker(time.Second * 30)
    defer ticker.Stop()
    for {
        select {
        case <-p.closing:
            return
        case <-ticker.C:
            p.evictIdle()
        }
    }
}

func (p *ClusterPool) evictIdle() {
    cutoff := time.Now().Add(-p.ttl)
    p.mu.Lock()
    defer p.mu.Unlock()
    for k, e := range p.entries {
        if e.lastUsed.Before(cutoff) {
            e.cancel()
            delete(p.entries, k)
        }
    }
}

