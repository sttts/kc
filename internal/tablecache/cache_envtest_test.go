package tablecache

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCacheReturnsRows(t *testing.T) {
	if testCfg == nil {
		t.Skip("envtest not available")
	}

	ctx, cancel := context.WithCancel(testCtx)
	defer cancel()

	cacheOpts := cache.Options{Scheme: testScheme}
	rowCache, err := New(testCfg, Options{Options: cacheOpts})
	if err != nil {
		t.Fatalf("create table cache: %v", err)
	}

	go func() {
		if err := rowCache.Start(ctx); err != nil {
			t.Fatalf("start cache: %v", err)
		}
	}()

	if ok := rowCache.WaitForCacheSync(ctx); !ok {
		t.Fatalf("cache did not sync")
	}

	cl, err := client.New(testCfg, client.Options{Scheme: testScheme})
	if err != nil {
		t.Fatalf("build client: %v", err)
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tablecache"}}
	if err := cl.Create(ctx, ns); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	pod := &corev1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: "tablecache-pod", Namespace: ns.Name},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "pause", Image: "pause"}}},
	}
	if err := cl.Create(ctx, pod); err != nil {
		t.Fatalf("create pod: %v", err)
	}

	podGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
	rows := NewRowList(podGVK)
	rows.SetTableTarget(podGVK)

	deadline := time.After(10 * time.Second)
	for {
		err := rowCache.List(ctx, rows, client.InNamespace(ns.Name))
		if err == nil && len(rows.Items) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("cache never returned rows: %v", err)
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}

	var found bool
	for _, row := range rows.Items {
		if row.Name == pod.Name {
			found = true
			if row.Namespace != pod.Namespace {
				t.Fatalf("row namespace = %s, want %s", row.Namespace, pod.Namespace)
			}
		}
	}
	if !found {
		t.Fatalf("pod row not found in cache list")
	}

	single := NewRow(podGVK)
	single.SetTableTarget(podGVK)

	if err := rowCache.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, single); err != nil {
		t.Fatalf("cache get: %v", err)
	}
	if single.Name != pod.Name || single.Namespace != pod.Namespace {
		t.Fatalf("cache get returned unexpected row metadata")
	}
}
