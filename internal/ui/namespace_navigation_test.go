package ui

import (
	"testing"

	"github.com/sttts/kc/internal/models"
	navui "github.com/sttts/kc/internal/navigation"
	"github.com/sttts/kc/pkg/appconfig"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestForceNamespaceNavigationLeavesFullNamespacesFolder(t *testing.T) {
	t.Parallel()

	app := NewApp()
	app.leftNav = navui.NewNavigator(nil)
	app.rightNav = navui.NewNavigator(nil)

	cfgLeft := appconfig.Default()
	cfgRight := appconfig.Default()
	depsLeft := models.Deps{
		Ctx:        t.Context(),
		AppConfig:  cfgLeft,
		KubeConfig: clientcmdapi.Config{Contexts: map[string]*clientcmdapi.Context{}},
	}
	depsRight := models.Deps{
		Ctx:        t.Context(),
		AppConfig:  cfgRight,
		KubeConfig: clientcmdapi.Config{Contexts: map[string]*clientcmdapi.Context{}},
	}

	if !app.forceNamespaceNavigation("default", depsLeft, depsRight) {
		t.Fatalf("forceNamespaceNavigation returned false")
	}

	// Simulate going back from /namespaces/<ns>/.. to /namespaces.
	app.leftNav.Back()
	folder := app.leftNav.Current()
	if folder == nil {
		t.Fatalf("expected namespaces folder on navigation stack")
	}
	if _, ok := folder.(*models.ClusterObjectsFolder); !ok {
		t.Fatalf("expected cluster objects folder, got %T", folder)
	}
}
