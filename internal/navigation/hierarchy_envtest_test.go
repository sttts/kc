package navigation

import (
	"context"
	"strings"
	"testing"
	"time"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/internal/models"
	kctesting "github.com/sttts/kc/internal/testing"
	"github.com/sttts/kc/pkg/appconfig"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func pathString(f models.Folder) string {
	if f == nil {
		return ""
	}
	path := f.Path()
	if len(path) == 0 {
		return "/"
	}
	return strings.Join(path, "/")
}

func hierarchyConfig() *appconfig.Config {
	cfg := appconfig.Default()
	cfg.Resources.ShowNonEmptyOnly = false
	cfg.Resources.Order = appconfig.OrderAlpha
	cfg.Resources.Columns = "normal"
	cfg.Objects.Order = "name"
	cfg.Objects.Columns = "normal"
	return cfg
}

func hierarchyDeps(cl *kccluster.Cluster, ctx context.Context, name string) models.Deps {
	contexts := map[string]*clientcmdapi.Context{}
	if name != "" {
		contexts[name] = &clientcmdapi.Context{}
	}
	return models.Deps{
		Cl:         cl,
		Ctx:        ctx,
		CtxName:    name,
		KubeConfig: clientcmdapi.Config{CurrentContext: name, Contexts: contexts},
		AppConfig:  hierarchyConfig(),
	}
}

func TestHierarchyEnvtest(t *testing.T) {
	t.Parallel()
	// Start envtest API server
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil || cfg == nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() { _ = testEnv.Stop() }()

	// Seed objects via a typed client
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	cli, err := crclient.New(cfg, crclient.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx := context.TODO()
	// Namespace testns
	if err := cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "testns"}}); err != nil {
		t.Fatalf("create ns: %v", err)
	}
	// ConfigMap and Secret in testns
	if err := cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "testns"}, Data: map[string]string{"a": "A", "b": "B"}}); err != nil {
		t.Fatalf("create cm1: %v", err)
	}
	if err := cli.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec1", Namespace: "testns"}, Data: map[string][]byte{"x": []byte("xx"), "y": []byte("yy")}}); err != nil {
		t.Fatalf("create sec1: %v", err)
	}
	// Node n1 (cluster-scoped)
	if err := cli.Create(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}); err != nil {
		t.Fatalf("create node: %v", err)
	}

	// Build kc cluster and start with ctx
	cl, err := kccluster.New(cfg)
	if err != nil {
		t.Fatalf("kccluster: %v", err)
	}
	go cl.Start(ctx)

	deps := hierarchyDeps(cl, ctx, "envtest")

	// 1) Root
	root := models.NewRootFolder(deps, nil)
	// Wait until namespaces are visible
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return root.Len(ctx) > 0 })
	count := root.Len(ctx)
	rows := root.Lines(ctx, 0, count)
	foundNamespaces := false
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 && cells[0] == "/namespaces" {
			foundNamespaces = true
			break
		}
	}
	if !foundNamespaces {
		t.Fatalf("root: /namespaces not found")
	}

	// 2) Enter /namespaces
	nsFolder := models.NewClusterObjectsFolder(deps, schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, []string{"namespaces"})
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return nsFolder.Len(ctx) > 0 })
	count = nsFolder.Len(ctx)
	rows = nsFolder.Lines(ctx, 0, count)
	foundTestns := false
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 && cells[0] == "/testns" {
			foundTestns = true
			break
		}
	}
	if !foundTestns {
		t.Fatalf("namespaces: /testns not found")
	}

	// 2b) Context root behaves like cluster root for this context
	ctxRoot := models.NewContextRootFolder(hierarchyDeps(cl, ctx, deps.CtxName), []string{"contexts", deps.CtxName})
	if got := strings.Join(ctxRoot.Path(), "/"); got != "contexts/"+deps.CtxName {
		t.Fatalf("context root path: got %q", got)
	}
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return ctxRoot.Len(ctx) > 0 })
	count = ctxRoot.Len(ctx)
	rows = ctxRoot.Lines(ctx, 0, count)
	hasNamespaces := false
	hasNodes := false
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 {
			if cells[0] == "/namespaces" {
				hasNamespaces = true
			}
			if cells[0] == "/nodes" {
				hasNodes = true
			}
		}
	}
	if !hasNamespaces {
		t.Fatalf("context root: /namespaces not found")
	}
	if !hasNodes {
		t.Fatalf("context root: /nodes not found")
	}

	// 3) Enter groups for testns
	grp := models.NewNamespacedResourcesFolder(deps, "testns", []string{"namespaces", "testns"})
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return grp.Len(ctx) > 0 })
	count = grp.Len(ctx)
	rows = grp.Lines(ctx, 0, count)
	hasCM, hasSec := false, false
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 {
			if cells[0] == "/configmaps" {
				hasCM = true
			}
			if cells[0] == "/secrets" {
				hasSec = true
			}
		}
	}
	if !hasCM || !hasSec {
		t.Fatalf("groups: expected /configmaps and /secrets, got %+v", rows)
	}

	// 4) Enter objects: configmaps
	gvrCM := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	objs := models.NewNamespacedObjectsFolder(deps, gvrCM, "testns", []string{"namespaces", "testns", gvrCM.Resource})
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return objs.Len(ctx) > 0 })
	count = objs.Len(ctx)
	rows = objs.Lines(ctx, 0, count)
	foundCM1 := false
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 && cells[0] == "/cm1" {
			foundCM1 = true
			break
		}
	}
	if !foundCM1 {
		t.Fatalf("objects: cm1 not found")
	}

	// 5) Enter cm1 keys
	keys := models.NewConfigMapKeysFolder(deps, []string{"namespaces", "testns", "configmaps", "cm1"}, "testns", "cm1")
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return keys.Len(ctx) >= 2 })
	count = keys.Len(ctx)
	rows = keys.Lines(ctx, 0, count)
	hasA, hasB := false, false
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 {
			if cells[0] == "a" {
				hasA = true
			}
			if cells[0] == "b" {
				hasB = true
			}
		}
	}
	if !hasA || !hasB {
		t.Fatalf("cm1 keys: expected a and b")
	}

	// 6) Cluster-scoped objects: nodes
	gvrNodes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
	nodes := models.NewClusterObjectsFolder(deps, gvrNodes, []string{"nodes"})
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return nodes.Len(ctx) > 0 })
	count = nodes.Len(ctx)
	rows = nodes.Lines(ctx, 0, count)
	foundN1 := false
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 && cells[0] == "n1" {
			foundN1 = true
			break
		}
	}
	if !foundN1 {
		t.Fatalf("nodes: n1 not found")
	}
}

// TestContextNamespaceWalk verifies that under /contexts/<ctx> we can walk into
// /namespaces, then a namespace, then a group and into concrete objects below.
func TestContextNamespaceWalk(t *testing.T) {
	t.Parallel()
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil || cfg == nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() { _ = testEnv.Stop() }()

	// Seed: ns testns, cm1 (a,b)
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	cli, err := crclient.New(cfg, crclient.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx := context.TODO()
	if err := cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "testns"}}); err != nil {
		t.Fatalf("create ns: %v", err)
	}
	if err := cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "testns"}, Data: map[string]string{"a": "A", "b": "B"}}); err != nil {
		t.Fatalf("create cm1: %v", err)
	}

	cl, err := kccluster.New(cfg)
	if err != nil {
		t.Fatalf("kccluster: %v", err)
	}
	go cl.Start(ctx)
	deps := hierarchyDeps(cl, ctx, "envtest")

	// Context root
	ctxRoot := models.NewContextRootFolder(deps, []string{"contexts", deps.CtxName})
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return ctxRoot.Len(ctx) > 0 })
	// Enter /namespaces
	var nsFolder models.Folder
	count := ctxRoot.Len(ctx)
	rows := ctxRoot.Lines(ctx, 0, count)
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 && cells[0] == "/namespaces" {
			if e, ok := r.(models.Enterable); ok {
				f, err := e.Enter()
				if err == nil {
					nsFolder = f
				}
				break
			}
		}
	}
	if nsFolder == nil {
		t.Fatalf("enter namespaces from context root failed")
	}
	// Wait for namespace
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return nsFolder.Len(ctx) > 0 })
	// Enter testns
	var grp models.Folder
	count = nsFolder.Len(ctx)
	rows = nsFolder.Lines(ctx, 0, count)
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 && cells[0] == "/testns" {
			if e, ok := r.(models.Enterable); ok {
				f, err := e.Enter()
				if err == nil {
					grp = f
				}
				break
			}
		}
	}
	if grp == nil {
		t.Fatalf("enter groups for testns failed")
	}
	// Enter configmaps group; verify proper "/" prefix and core group displayed as "v1"
	var objs models.Folder
	count = grp.Len(ctx)
	rows = grp.Lines(ctx, 0, count)
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 1 && cells[0] == "/configmaps" {
			if cells[1] != "v1" {
				t.Fatalf("expected group column 'v1' for core resources, got %q", cells[1])
			}
			if e, ok := r.(models.Enterable); ok {
				f, err := e.Enter()
				if err == nil {
					objs = f
				}
				break
			}
		}
	}
	if objs == nil {
		t.Fatalf("enter configmaps objects failed")
	}
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return objs.Len(ctx) > 0 })
	// Enter cm1 keys
	var keys models.Folder
	count = objs.Len(ctx)
	rows = objs.Lines(ctx, 0, count)
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 && cells[0] == "/cm1" {
			if e, ok := r.(models.Enterable); ok {
				f, err := e.Enter()
				if err == nil {
					keys = f
				}
				break
			}
		}
	}
	if keys == nil {
		t.Fatalf("enter cm1 keys failed")
	}
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return keys.Len(ctx) >= 2 })
	count = keys.Len(ctx)
	rows = keys.Lines(ctx, 0, count)
	seen := map[string]bool{}
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 {
			seen[cells[0]] = true
		}
	}
	if !seen["a"] || !seen["b"] {
		t.Fatalf("cm1 keys: expected a and b, got %+v", seen)
	}
}

// TestStartupSelectionRestore simulates the app's startup navigation
// (root -> namespaces -> namespaced groups) and verifies that going back
// restores the previous selection IDs (namespace, then "/namespaces").
func TestStartupSelectionRestore(t *testing.T) {
	t.Parallel()
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil || cfg == nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() { _ = testEnv.Stop() }()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	cli, err := crclient.New(cfg, crclient.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx := context.TODO()
	if err := cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "testns"}}); err != nil {
		t.Fatalf("create ns: %v", err)
	}

	cl, err := kccluster.New(cfg)
	if err != nil {
		t.Fatalf("kccluster: %v", err)
	}
	go cl.Start(ctx)
	deps := hierarchyDeps(cl, ctx, "envtest")

	root := models.NewContextRootFolder(deps, []string{"contexts", deps.CtxName})
	nav := NewNavigator(root)
	// Simulate app startup sequence
	nav.SetSelectionID("namespaces")
	nav.Push(models.NewClusterObjectsFolder(deps, schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, []string{"namespaces"}))
	nav.SetSelectionID("testns")
	nav.Push(models.NewNamespacedResourcesFolder(deps, "testns", []string{"namespaces", "testns"}))

	// Back to namespaces, selection should be "testns"
	if cur := nav.Back(); cur == nil || pathString(cur) != "namespaces" {
		t.Fatalf("expected namespaces after first back, got %v", cur)
	}
	if sel := nav.CurrentSelectionID(); sel != "testns" {
		t.Fatalf("expected selection 'testns', got %q", sel)
	}
	// Back to context root, selection should be "/namespaces"
	if cur := nav.Back(); cur == nil || pathString(cur) != "contexts/"+deps.CtxName {
		t.Fatalf("expected context root after second back, got %v", cur)
	}
	if sel := nav.CurrentSelectionID(); sel != "namespaces" {
		t.Fatalf("expected selection 'namespaces', got %q", sel)
	}
}

// TestClusterStartupSelectionRestore mirrors startup selection restore at cluster root (/).
func TestClusterStartupSelectionRestore(t *testing.T) {
	t.Parallel()
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil || cfg == nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() { _ = testEnv.Stop() }()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	cli, err := crclient.New(cfg, crclient.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx := context.TODO()
	if err := cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "testns"}}); err != nil {
		t.Fatalf("create ns: %v", err)
	}

	cl, err := kccluster.New(cfg)
	if err != nil {
		t.Fatalf("kccluster: %v", err)
	}
	go cl.Start(ctx)
	deps := hierarchyDeps(cl, ctx, "envtest")

	root := models.NewRootFolder(deps, nil)
	nav := NewNavigator(root)
	// Simulate app startup sequence
	nav.SetSelectionID("namespaces")
	nav.Push(models.NewClusterObjectsFolder(deps, schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, []string{"namespaces"}))
	nav.SetSelectionID("testns")
	nav.Push(models.NewNamespacedResourcesFolder(deps, "testns", []string{"namespaces", "testns"}))

	// Back to namespaces
	if cur := nav.Back(); cur == nil || pathString(cur) != "namespaces" {
		t.Fatalf("expected namespaces, got %v", cur)
	}
	if sel := nav.CurrentSelectionID(); sel != "testns" {
		t.Fatalf("expected 'testns', got %q", sel)
	}
	// Back to root
	if cur := nav.Back(); cur == nil || pathString(cur) != "/" {
		t.Fatalf("expected root /, got %v", cur)
	}
	if sel := nav.CurrentSelectionID(); sel != "namespaces" {
		t.Fatalf("expected 'namespaces', got %q", sel)
	}
}

// TestGroupObjectBackSelectionRestore: enter a group, then an object, then back restores object selection,
// and another back restores group selection.
func TestGroupObjectBackSelectionRestore(t *testing.T) {
	t.Parallel()
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil || cfg == nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() { _ = testEnv.Stop() }()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	cli, err := crclient.New(cfg, crclient.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx := context.TODO()
	if err := cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "testns"}}); err != nil {
		t.Fatalf("create ns: %v", err)
	}
	if err := cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "testns"}, Data: map[string]string{"a": "A"}}); err != nil {
		t.Fatalf("create cm1: %v", err)
	}

	cl, err := kccluster.New(cfg)
	if err != nil {
		t.Fatalf("kccluster: %v", err)
	}
	go cl.Start(ctx)
	deps := hierarchyDeps(cl, ctx, "envtest")

	root := models.NewRootFolder(deps, nil)
	nav := NewNavigator(root)
	// Into namespaces -> testns -> groups
	nav.SetSelectionID("namespaces")
	nav.Push(models.NewClusterObjectsFolder(deps, schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, []string{"namespaces"}))
	nav.SetSelectionID("testns")
	nav.Push(models.NewNamespacedResourcesFolder(deps, "testns", []string{"namespaces", "testns"}))
	// Find configmaps group and enter objects
	var objs models.Folder
	var groupID string
	current := nav.Current()
	count := current.Len(ctx)
	rows := current.Lines(ctx, 0, count)
	for _, r := range rows {
		id, cells, _, _ := r.Columns()
		if len(cells) > 0 && cells[0] == "/configmaps" {
			groupID = id
			if e, ok := r.(models.Enterable); ok {
				f, err := e.Enter()
				if err == nil {
					objs = f
				}
				break
			}
		}
	}
	if objs == nil {
		t.Fatalf("enter configmaps objects failed")
	}
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return objs.Len(ctx) > 0 })
	// Remember group selection and enter objects
	nav.SetSelectionID(groupID)
	nav.Push(objs)
	// Enter cm1 keys (simulate selection)
	nav.SetSelectionID("cm1")
	// Ensure a keys folder can be constructed by calling Enterable on cm1 row
	current = nav.Current()
	count = current.Len(ctx)
	rows = current.Lines(ctx, 0, count)
	var keys models.Folder
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 && cells[0] == "/cm1" {
			if e, ok := r.(models.Enterable); ok {
				f, err := e.Enter()
				if err == nil {
					keys = f
				}
				break
			}
		}
	}
	if keys == nil {
		t.Fatalf("enter cm1 keys failed")
	}
	nav.Push(keys)
	// Back to objects; selection should be cm1
	if cur := nav.Back(); cur == nil || pathString(cur) != "namespaces/testns/configmaps" {
		t.Fatalf("expected namespaces/testns/configmaps, got %s", pathString(cur))
	}
	if sel := nav.CurrentSelectionID(); sel != "cm1" {
		t.Fatalf("expected selection 'cm1', got %q", sel)
	}
	// Back to groups; selection should be groupID
	if cur := nav.Back(); cur == nil || pathString(cur) != "namespaces/testns" {
		t.Fatalf("expected namespaces/testns, got %s", pathString(cur))
	}
	if sel := nav.CurrentSelectionID(); sel != groupID {
		t.Fatalf("expected selection %q, got %q", groupID, sel)
	}
}
