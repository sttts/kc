package models

import (
	"context"
	"testing"
	"time"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/pkg/appconfig"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestResourceGroupItemUpdatesCountOnInformerEvents(t *testing.T) {
	if testCfg == nil {
		t.Skip("envtest not available")
	}
	cl, err := kccluster.New(testCfg)
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}
	ctx := t.Context()
	go cl.Start(ctx)
	t.Cleanup(cl.Stop)

	ns := "rg-item"
	createNamespace(t, cl, ns)
	createConfigMap(t, cl, ns, "cm-1")

	deps := Deps{
		Cl:        cl,
		Ctx:       ctx,
		AppConfig: appconfig.Default(),
	}
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	item := NewResourceGroupItem(deps, gvr, ns, "id", []string{"/configmaps", "v1", ""}, []string{"namespaces", ns, "configmaps"}, "configmaps", WhiteStyle(), true, nil)

	if count := item.Count(); count != 1 {
		t.Fatalf("initial count = %d, want 1", count)
	}

	createConfigMap(t, cl, ns, "cm-2")
	waitForCount(t, item, 2)
}

func createNamespace(t *testing.T, cl *kccluster.Cluster, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := cl.GetClient().Create(ctx, ns); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	t.Cleanup(func() {
		delCtx, cancelDel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelDel()
		_ = cl.GetClient().Delete(delCtx, ns)
	})
}

func createConfigMap(t *testing.T, cl *kccluster.Cluster, namespace, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{"k": "v"},
	}
	if err := cl.GetClient().Create(ctx, cm); err != nil {
		t.Fatalf("create configmap %s/%s: %v", namespace, name, err)
	}
	t.Cleanup(func() {
		delCtx, cancelDel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelDel()
		_ = cl.GetClient().Delete(delCtx, cm)
	})
}

func waitForCount(t *testing.T, item *ResourceGroupItem, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if count, ok := item.TryCount(); ok && count == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	if count, ok := item.TryCount(); ok {
		t.Fatalf("count = %d, want %d", count, want)
	}
	t.Fatalf("count not ready, wanted %d", want)
}
