package cluster

import (
	"strings"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestClusterDiscoveryListenerNotified(t *testing.T) {
	t.Parallel()

	env := &envtest.Environment{}
	cfg, err := env.Start()
	if err != nil || cfg == nil {
		if err != nil && strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("envtest unavailable: %v", err)
		}
		t.Fatalf("start envtest: %v", err)
	}
	defer func() { _ = env.Stop() }()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 to scheme: %v", err)
	}

	cl, err := New(cfg, WithScheme(scheme))
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}

	var hits atomic.Int32
	cancel := cl.AddDiscoveryListener(func() {
		hits.Add(1)
	})

	cl.RefreshDiscovery()
	got := hits.Load()
	if got == 0 {
		t.Fatalf("expected discovery listener to fire at least once")
	}

	cancel()
	cl.RefreshDiscovery()
	if hits.Load() != got {
		t.Fatalf("listener fired after cancel: before=%d after=%d", got, hits.Load())
	}
}
