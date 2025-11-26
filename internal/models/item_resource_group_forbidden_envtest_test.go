package models

import (
	"testing"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/pkg/appconfig"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// Verify that forbidden peeks mark the resource group as empty/blocked for a limited user.
func TestResourceGroupItemLimitedAccessBlocksPeeks(t *testing.T) {
	if testCfg == nil {
		t.Skip("envtest not available")
	}

	ctx := t.Context()
	adminCl, err := kccluster.New(testCfg)
	if err != nil {
		t.Fatalf("admin cluster: %v", err)
	}
	go adminCl.Start(ctx)
	t.Cleanup(adminCl.Stop)

	const ns = "limited-access-forbidden"
	createNamespace(t, adminCl, ns)
	createConfigMap(t, adminCl, ns, "cm-visible")

	limitedCfg := rest.CopyConfig(testCfg)
	limitedCfg.Impersonate.UserName = "limited-user"

	limitedCl, err := kccluster.New(limitedCfg)
	if err != nil {
		t.Fatalf("limited cluster: %v", err)
	}
	go limitedCl.Start(ctx)
	t.Cleanup(limitedCl.Stop)

	deps := Deps{
		Cl:        limitedCl,
		Ctx:       ctx,
		AppConfig: appconfig.Default(),
	}
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	item := NewResourceGroupItem(deps, gvr, gvk, ns, "id", []string{"/configmaps", "v1", ""}, []string{"namespaces", ns, "configmaps"}, "configmaps", WhiteStyle(), true, nil)

	if empty := item.Empty(); !empty {
		t.Fatalf("expected limited user to see empty due to forbidden peeks")
	}
	if !item.Forbidden() {
		t.Fatalf("expected peeks to be marked as forbidden")
	}
	if count, ok := item.TryCount(); !ok || count != 0 {
		t.Fatalf("TryCount = (%d, %t), want (0, true)", count, ok)
	}
	if count := item.Count(); count != 0 {
		t.Fatalf("Count = %d, want 0 when peeks are blocked", count)
	}
}
