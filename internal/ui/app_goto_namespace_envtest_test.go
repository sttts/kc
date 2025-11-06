package ui

import (
	"context"
	"testing"
	"time"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/pkg/kubeconfig"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGoToNamespaceEmptyStaysClusterScope(t *testing.T) {
	if testCfg == nil {
		t.Skip("envtest not available")
	}
	cl, err := kccluster.New(testCfg)
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}
	ctx := t.Context()
	go cl.Start(ctx)
	t.Cleanup(func() {
		cl.Stop()
	})

	app := NewApp()
	app.cancel()
	app.ctx, app.cancel = context.WithCancel(ctx)
	t.Cleanup(app.cancel)
	app.cl = cl
	app.currentCtx = &kubeconfig.Context{Name: "env"}

	app.goToNamespace("")

	if app.leftNav == nil || app.rightNav == nil {
		t.Fatalf("expected navigators to be initialized")
	}
	if got := app.leftPanel.GetCurrentPath(); got != "/" {
		t.Fatalf("left panel path = %q, want /", got)
	}
	if got := app.rightPanel.GetCurrentPath(); got != "/" {
		t.Fatalf("right panel path = %q, want /", got)
	}
	if app.leftNav.HasBack() {
		t.Fatalf("expected left navigator to have no back history")
	}
	if app.rightNav.HasBack() {
		t.Fatalf("expected right navigator to have no back history")
	}
}

func TestGoToNamespaceExistingNavigatesPanels(t *testing.T) {
	if testCfg == nil {
		t.Skip("envtest not available")
	}
	cl, err := kccluster.New(testCfg)
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}
	ctx := t.Context()
	go cl.Start(ctx)
	t.Cleanup(func() {
		cl.Stop()
	})

	nsName := "autonav"
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
	if err := cl.GetClient().Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = cl.GetClient().Delete(cleanupCtx, namespace)
	})

	app := NewApp()
	app.cancel()
	app.ctx, app.cancel = context.WithCancel(ctx)
	t.Cleanup(app.cancel)
	app.cl = cl
	app.currentCtx = &kubeconfig.Context{Name: "env", Namespace: nsName}

	waitCtx, cancelWait := context.WithTimeout(ctx, 5*time.Second)
	defer cancelWait()
	for {
		if app.namespaceExists(nsName) {
			break
		}
		select {
		case <-time.After(50 * time.Millisecond):
		case <-waitCtx.Done():
			t.Fatalf("namespace %q not observed: %v", nsName, waitCtx.Err())
		}
	}

	app.goToNamespace(nsName)

	wantPath := "/namespaces/" + nsName
	if got := app.leftPanel.GetCurrentPath(); got != wantPath {
		t.Fatalf("left panel path = %q, want %q", got, wantPath)
	}
	if got := app.rightPanel.GetCurrentPath(); got != wantPath {
		t.Fatalf("right panel path = %q, want %q", got, wantPath)
	}
}
