package models

import (
	"context"
	"strings"
	"testing"
	"time"

	kccluster "github.com/sttts/kc/internal/cluster"
	table "github.com/sttts/kc/internal/table"
	kctesting "github.com/sttts/kc/internal/testing"
	"github.com/sttts/kc/pkg/appconfig"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestFoldersProduceExpectedRows(t *testing.T) {
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
	_ = corev1.AddToScheme(scheme)
	cli, err := crclient.New(cfg, crclient.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx := t.Context()

	seedData(t, cli)

	cl, err := kccluster.New(cfg)
	if err != nil {
		t.Fatalf("kccluster: %v", err)
	}
	go cl.Start(ctx)

	panelCfg := appconfig.Default()
	panelCfg.Resources.ShowNonEmptyOnly = false
	panelCfg.Resources.Order = appconfig.OrderAlpha
	panelCfg.Resources.Columns = "normal"
	panelCfg.Objects.Order = "name"
	panelCfg.Objects.Columns = "normal"

	deps := Deps{
		Cl:         cl,
		Ctx:        ctx,
		CtxName:    "envtest",
		KubeConfig: clientcmdapi.Config{CurrentContext: "envtest", Contexts: map[string]*clientcmdapi.Context{"envtest": &clientcmdapi.Context{}}},
		AppConfig:  panelCfg,
	}

	root := NewRootFolder(deps, nil)
	waitFolder(t, root)
	assertRows(t, "root", root, map[string][]string{
		"namespaces": {"/namespaces", "v1"},
		"/v1/nodes":  {"/nodes", "v1"},
	})

	groupsPath := []string{"namespaces", "testns"}
	groups := NewNamespacedResourcesFolder(deps, "testns", groupsPath)
	waitFolder(t, groups)
	assertRows(t, "namespaced-groups", groups, map[string][]string{
		"testns//v1/configmaps": {"/configmaps", "v1"},
		"testns//v1/secrets":    {"/secrets", "v1"},
	})

	gvrCM := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	objsPath := []string{"namespaces", "testns", gvrCM.Resource}
	objs := NewNamespacedObjectsFolder(deps, gvrCM, "testns", objsPath, nil)
	waitFolder(t, objs)
	assertRows(t, "configmap-objects", objs, map[string][]string{
		"cm1": {"/cm1"},
	})

	keysPath := []string{"namespaces", "testns", "configmaps", "cm1"}
	keys := NewConfigMapKeysFolder(deps, keysPath, "testns", "cm1")
	waitFolder(t, keys)
	assertRows(t, "configmap-keys", keys, map[string][]string{
		"a": {"a"},
		"b": {"b"},
	})

	secPath := []string{"namespaces", "testns", "secrets", "sec1"}
	secs := NewSecretKeysFolder(deps, secPath, "testns", "sec1")
	waitFolder(t, secs)
	assertRows(t, "secret-keys", secs, map[string][]string{
		"x": {"x"},
		"y": {"y"},
	})
}

func seedData(t *testing.T, cli crclient.Client) {
	t.Helper()
	mustCreate(t, cli, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "testns"}})
	mustCreate(t, cli, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "testns"}, Data: map[string]string{"a": "A", "b": "B"}})
	mustCreate(t, cli, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec1", Namespace: "testns"}, Data: map[string][]byte{"x": []byte("xx"), "y": []byte("yy")}})
	mustCreate(t, cli, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}})
}

func mustCreate(t *testing.T, cli crclient.Client, obj crclient.Object) {
	t.Helper()
	if err := cli.Create(t.Context(), obj); err != nil {
		t.Fatalf("create %T: %v", obj, err)
	}
}

func waitFolder(t *testing.T, f interface{ Len(context.Context) int }) {
	t.Helper()
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool {
		return f.Len(t.Context()) > 0
	})
}

func assertRows(t *testing.T, name string, f interface {
	Len(context.Context) int
	Lines(context.Context, int, int) []table.Row
}, expected map[string][]string) {
	t.Helper()
	ctx := t.Context()
	count := f.Len(ctx)
	rows := f.Lines(ctx, 0, count)
	got := make(map[string][]string, len(rows))
	for _, r := range rows {
		id, cells, _, ok := r.Columns()
		if !ok {
			continue
		}
		got[id] = append([]string(nil), cells...)
	}
	if len(got) < len(expected) {
		t.Fatalf("%s: expected at least %d rows, got %d (%v)", name, len(expected), len(got), got)
	}
	for id, cells := range expected {
		gc, ok := got[id]
		if !ok {
			t.Fatalf("%s: missing row %q", name, id)
		}
		if len(gc) < len(cells) {
			t.Fatalf("%s: row %q expected at least %d cells, got %v", name, id, len(cells), gc)
		}
		for i := range cells {
			if gc[i] != cells[i] {
				t.Fatalf("%s: row %q expected %v got %v", name, id, cells, gc)
			}
		}
	}
}

func TestDiscoveryRefreshAddsCRD(t *testing.T) {
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

	cl, err := kccluster.New(cfg)
	if err != nil {
		t.Fatalf("kccluster: %v", err)
	}

	ctx := t.Context()
	go cl.Start(ctx)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "crd-ns"}}
	mustCreate(t, cl.GetClient(), ns)

	deps := Deps{
		Cl:      cl,
		Ctx:     ctx,
		CtxName: "envtest",
		KubeConfig: clientcmdapi.Config{
			CurrentContext: "envtest",
			Contexts:       map[string]*clientcmdapi.Context{"envtest": {Namespace: ns.Name}},
		},
		AppConfig: appconfig.Default(),
	}
	deps.AppConfig.Resources.ShowNonEmptyOnly = false

	folder := NewNamespacedResourcesFolder(deps, ns.Name, []string{"namespaces", ns.Name})
	folder.Refresh()
	ctxFolder := t.Context()
	waitFolder(t, folder)

	apiext, err := apiextclient.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("apiext client: %v", err)
	}

	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "widgets.example.com"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "widgets",
				Singular: "widget",
				Kind:     "Widget",
				ListKind: "WidgetList",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    "v1",
				Served:  true,
				Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]apiextensionsv1.JSONSchemaProps{
									"replicas": {Type: "integer"},
								},
							},
						},
					},
				},
			}},
		},
	}

	if _, err := apiext.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, crd, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create CRD: %v", err)
	}

	kctesting.Eventually(t, 10*time.Second, 100*time.Millisecond, func() bool {
		latest, err := apiext.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		for _, cond := range latest.Status.Conditions {
			if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
				return true
			}
		}
		return false
	})

	cl.RefreshDiscovery()
	folder.Refresh()

	kctesting.Eventually(t, 10*time.Second, 100*time.Millisecond, func() bool {
		count := folder.Len(ctxFolder)
		rows := folder.Lines(ctxFolder, 0, count)
		for _, r := range rows {
			id, _, _, ok := r.Columns()
			if ok && strings.Contains(id, "widgets") {
				return true
			}
		}
		folder.Refresh()
		return false
	})
}
