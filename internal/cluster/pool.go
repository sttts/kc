package cluster

import (
    "context"
    "fmt"
    "sync"
    "time"

    "k8s.io/client-go/tools/clientcmd"
    crcluster "sigs.k8s.io/controller-runtime/pkg/cluster"
)

// Key identifies a controller-runtime Cluster by kubeconfig path and context name.
type Key struct {
    KubeconfigPath string
    ContextName    string
}

type entry struct {
    key      Key
    cl       crcluster.Cluster
    cancel   context.CancelFunc
    lastUsed time.Time
    ready    sync.Once
    err      error
}

// Pool manages controller-runtime clusters per kubeconfig+context with idle eviction.
type Pool struct {
    mu      sync.RWMutex
    ttl     time.Duration
    closing chan struct{}
    started bool
    items   map[Key]*entry
}

func NewPool(ttl time.Duration) *Pool { return &Pool{ttl: ttl, closing: make(chan struct{}), items: map[Key]*entry{}} }

func (p *Pool) Start() {
    p.mu.Lock()
    if p.started { p.mu.Unlock(); return }
    p.started = true
    p.mu.Unlock()
    go p.evictLoop()
}

func (p *Pool) Stop() {
    close(p.closing)
    p.mu.Lock(); defer p.mu.Unlock()
    for k, e := range p.items { e.cancel(); delete(p.items, k) }
}

// Get returns a running controller-runtime Cluster for the key, starting it if needed.
func (p *Pool) Get(k Key) (crcluster.Cluster, error) {
    p.mu.Lock()
    if e, ok := p.items[k]; ok {
        e.lastUsed = time.Now(); p.mu.Unlock(); return e.cl, nil
    }
    cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
        &clientcmd.ClientConfigLoadingRules{ExplicitPath: k.KubeconfigPath},
        &clientcmd.ConfigOverrides{CurrentContext: k.ContextName},
    ).ClientConfig()
    if err != nil { p.mu.Unlock(); return nil, fmt.Errorf("client config: %w", err) }
    cl, err := crcluster.New(cfg, func(o *crcluster.Options) {})
    if err != nil { p.mu.Unlock(); return nil, fmt.Errorf("cluster: %w", err) }
    ctx, cancel := context.WithCancel(context.TODO())
    e := &entry{key: k, cl: cl, cancel: cancel, lastUsed: time.Now()}
    p.items[k] = e
    p.mu.Unlock()
    go func(){ e.ready.Do(func(){ e.err = cl.Start(ctx) }) }()
    return cl, nil
}

func (p *Pool) Touch(k Key) { p.mu.Lock(); if e, ok := p.items[k]; ok { e.lastUsed = time.Now() }; p.mu.Unlock() }

func (p *Pool) evictLoop() {
    t := time.NewTicker(30 * time.Second); defer t.Stop()
    for {
        select {
        case <-p.closing:
            return
        case <-t.C:
            p.evictIdle()
        }
    }
}

func (p *Pool) evictIdle() {
    cutoff := time.Now().Add(-p.ttl)
    p.mu.Lock(); defer p.mu.Unlock()
    for k, e := range p.items {
        if e.lastUsed.Before(cutoff) { e.cancel(); delete(p.items, k) }
    }
}

