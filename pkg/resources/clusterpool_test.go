package resources

import (
	"testing"
	"time"
)

func TestClusterPoolEviction(t *testing.T) {
	ttl := 50 * time.Millisecond
	p := NewClusterPool(ttl)

	// Insert a fake entry with old lastUsed
	key := ClusterKey{KubeconfigPath: "/dev/null", ContextName: "test"}
	old := time.Now().Add(-2 * ttl)
	e := &clusterEntry{key: key, lastUsed: old, cancel: func() {}}
	p.mu.Lock()
	p.entries[key] = e
	p.mu.Unlock()

	// Evict idle entries
	p.evictIdle()

	p.mu.RLock()
	defer p.mu.RUnlock()
	if _, ok := p.entries[key]; ok {
		t.Fatalf("expected entry to be evicted after TTL, but it remains")
	}
}
