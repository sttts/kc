package ui

import (
	"context"
	"testing"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/pkg/kubeconfig"
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
