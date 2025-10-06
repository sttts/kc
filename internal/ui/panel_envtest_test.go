package ui

import (
	"context"
	"testing"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/internal/models"
	"github.com/sttts/kc/pkg/appconfig"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestFooterShowsGroupVersionForPods(t *testing.T) {
	if testCfg == nil {
		t.Skip("envtest not available")
	}
	cl, err := kccluster.New(testCfg)
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}
	ctx := context.TODO()
	go cl.Start(ctx)

	cfg := appconfig.Default()
	cfg.Resources.ShowNonEmptyOnly = false
	cfg.Resources.Columns = "normal"
	cfg.Resources.Order = appconfig.OrderAlpha
	cfg.Objects.Order = "name"
	cfg.Objects.Columns = "normal"

	deps := models.Deps{
		Cl:         cl,
		Ctx:        ctx,
		CtxName:    "env",
		KubeConfig: clientcmdapi.Config{CurrentContext: "env", Contexts: map[string]*clientcmdapi.Context{"env": &clientcmdapi.Context{}}},
		AppConfig:  cfg,
	}
	folder := models.NewNamespacedResourcesFolder(deps, "default", []string{"namespaces", "default"})

	p := NewPanel("")
	p.UseFolder(true)
	p.SetFolder(folder, false)
	_ = p.ViewContentOnlyFocused(false)

	rows := folder.Lines(0, folder.Len())
	idx := -1
	for i := range rows {
		_, cells, _, _ := rows[i].Columns()
		if len(cells) > 0 && cells[0] == "/pods" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Skip("/pods not present in groups (env may not expose pods)")
	}
	if idx >= len(rows) {
		t.Fatalf("pods row index out of range")
	}
	_, cells, _, _ := rows[idx].Columns()
	if len(cells) < 2 || cells[1] != "v1" {
		t.Fatalf("expected group column 'v1' for pods, got %+v", cells)
	}
}
