package cluster

import (
	"testing"
	"time"
)

func TestPoolEvictIdleRemovesExpiredEntries(t *testing.T) {
	t.Parallel()

	p := NewPool(10 * time.Millisecond)
	key := Key{KubeconfigPath: "/tmp/kubeconfig", ContextName: "ctx"}

	evicted := 0
	p.mu.Lock()
	p.items[key] = &entry{
		key:      key,
		lastUsed: time.Now().Add(-time.Minute),
		cancel:   func() { evicted++ },
	}
	p.mu.Unlock()

	p.evictIdle()

	p.mu.RLock()
	_, exists := p.items[key]
	p.mu.RUnlock()

	if exists {
		t.Fatalf("expected expired entry to be evicted")
	}
	if evicted != 1 {
		t.Fatalf("expected cancel to be called once, got %d", evicted)
	}
}

func TestPoolEvictIdleKeepsRecentEntries(t *testing.T) {
	t.Parallel()

	p := NewPool(50 * time.Millisecond)
	key := Key{KubeconfigPath: "/tmp/kubeconfig", ContextName: "ctx"}

	p.mu.Lock()
	p.items[key] = &entry{
		key:      key,
		lastUsed: time.Now(),
		cancel:   func() { t.Fatalf("unexpected cancel") },
	}
	p.mu.Unlock()

	p.evictIdle()

	p.mu.RLock()
	_, exists := p.items[key]
	p.mu.RUnlock()

	if !exists {
		t.Fatalf("expected recent entry to remain in pool")
	}
}
