package cluster

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metameta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	toolscache "k8s.io/client-go/tools/cache"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestClusterDiscoveryListenerNotified(t *testing.T) {
	t.Parallel()

	env := &envtest.Environment{}
	cfg, err := env.Start()
	if err != nil || cfg == nil {
		if err != nil {
			if strings.Contains(err.Error(), "operation not permitted") || strings.Contains(err.Error(), "no such file or directory") {
				t.Skipf("envtest unavailable: %v", err)
			}
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

func TestNamespaceScopedInformerExposesStore(t *testing.T) {
	t.Parallel()

	env := &envtest.Environment{}
	cfg, err := env.Start()
	if err != nil || cfg == nil {
		if err != nil {
			if strings.Contains(err.Error(), "operation not permitted") || strings.Contains(err.Error(), "no such file or directory") {
				t.Skipf("envtest unavailable: %v", err)
			}
		}
		t.Fatalf("start envtest: %v", err)
	}
	defer func() { _ = env.Stop() }()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 to scheme: %v", err)
	}

	cl, err := New(cfg, WithScheme(scheme), WithNamespaceScope("ns-a"))
	if err != nil {
		t.Fatalf("namespace-scoped cluster: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = cl.Start(ctx) }()

	createCM := func(ns, name string) {
		t.Helper()
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Data:       map[string]string{"k": "v"},
		}
		if err := cl.GetClient().Create(context.Background(), cm); err != nil {
			t.Fatalf("create configmap %s/%s: %v", ns, name, err)
		}
	}
	// ensure namespaces exist
	for _, ns := range []string{"ns-a", "ns-b"} {
		if err := cl.GetClient().Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}); err != nil && !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("create namespace %s: %v", ns, err)
		}
	}
	createCM("ns-a", "only-a")
	createCM("ns-b", "only-b")

	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	gvk, err := cl.RESTMapper().KindFor(gvr)
	if err != nil {
		t.Fatalf("kindFor: %v", err)
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	informer, err := cl.GetCache().GetInformer(context.Background(), obj, crcache.BlockUntilSynced(true))
	if err != nil {
		t.Fatalf("get informer: %v", err)
	}

	type storeInformer interface{ GetStore() toolscache.Store }
	si, ok := informer.(storeInformer)
	if !ok {
		t.Fatalf("informer %T does not expose GetStore", informer)
	}

	toolscache.WaitForCacheSync(ctx.Done(), informer.HasSynced)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		items := si.GetStore().List()
		foundA := false
		foundB := false
		for _, raw := range items {
			obj, okObj := raw.(metav1.Object)
			if !okObj {
				acc, err := metameta.Accessor(raw)
				if err != nil {
					continue
				}
				obj = acc
			}
			switch obj.GetNamespace() {
			case "ns-a":
				foundA = true
			case "ns-b":
				foundB = true
			}
		}
		if foundA && !foundB {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("expected informer store to contain only ns-a objects; store=%v", si.GetStore().List())
}
