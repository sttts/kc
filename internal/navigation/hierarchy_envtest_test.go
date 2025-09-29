package navigation

import (
    "context"
    "testing"
    "time"

    kccluster "github.com/sttts/kc/internal/cluster"
    kctesting "github.com/sttts/kc/internal/testing"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/runtime/schema"
    crclient "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestHierarchyEnvtest(t *testing.T) {
    t.Parallel()
    // Start envtest API server
    testEnv := &envtest.Environment{}
    cfg, err := testEnv.Start()
    if err != nil || cfg == nil { t.Fatalf("start envtest: %v", err) }
    defer func(){ _ = testEnv.Stop() }()

    // Seed objects via a typed client
    scheme := runtime.NewScheme()
    _ = corev1.AddToScheme(scheme)
    cli, err := crclient.New(cfg, crclient.Options{Scheme: scheme})
    if err != nil { t.Fatalf("new client: %v", err) }
    ctx := context.TODO()
    // Namespace testns
    if err := cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "testns"}}); err != nil { t.Fatalf("create ns: %v", err) }
    // ConfigMap and Secret in testns
    if err := cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "testns"}, Data: map[string]string{"a":"A","b":"B"}}); err != nil { t.Fatalf("create cm1: %v", err) }
    if err := cli.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec1", Namespace: "testns"}, Data: map[string][]byte{"x": []byte("xx"), "y": []byte("yy")}}); err != nil { t.Fatalf("create sec1: %v", err) }
    // Node n1 (cluster-scoped)
    if err := cli.Create(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}); err != nil { t.Fatalf("create node: %v", err) }

    // Build kc cluster and start with ctx
    cl, err := kccluster.New(cfg)
    if err != nil { t.Fatalf("kccluster: %v", err) }
    go cl.Start(ctx)

    deps := Deps{Cl: cl, Ctx: ctx, CtxName: "envtest"}

    // 1) Root
    root := NewRootFolder(deps)
    // Wait until namespaces are visible
    kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return root.Len() > 0 })
    rows := root.Lines(0, root.Len())
    foundNamespaces := false
    for _, r := range rows { _, cells, _, _ := r.Columns(); if len(cells) > 0 && cells[0] == "/namespaces" { foundNamespaces = true; break } }
    if !foundNamespaces { t.Fatalf("root: /namespaces not found") }

    // 2) Enter /namespaces
    nsFolder := NewNamespacesFolder(deps)
    kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return nsFolder.Len() > 0 })
    rows = nsFolder.Lines(0, nsFolder.Len())
    foundTestns := false
    for _, r := range rows { _, cells, _, _ := r.Columns(); if len(cells) > 0 && cells[0] == "/testns" { foundTestns = true; break } }
    if !foundTestns { t.Fatalf("namespaces: /testns not found") }

    // 2b) Context root behaves like cluster root for this context
    ctxRoot := NewContextRootFolder(deps)
    if ctxRoot.Title() != "contexts/"+deps.CtxName { t.Fatalf("context root title: got %q", ctxRoot.Title()) }
    kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return ctxRoot.Len() > 0 })
    rows = ctxRoot.Lines(0, ctxRoot.Len())
    hasNamespaces := false
    hasNodes := false
    for _, r := range rows {
        _, cells, _, _ := r.Columns()
        if len(cells) > 0 {
            if cells[0] == "/namespaces" { hasNamespaces = true }
            if cells[0] == "/nodes" { hasNodes = true }
        }
    }
    if !hasNamespaces { t.Fatalf("context root: /namespaces not found") }
    if !hasNodes { t.Fatalf("context root: /nodes not found") }

    // 3) Enter groups for testns
    grp := NewNamespacedGroupsFolder(deps, "testns")
    kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return grp.Len() > 0 })
    rows = grp.Lines(0, grp.Len())
    hasCM, hasSec := false, false
    for _, r := range rows { _, cells, _, _ := r.Columns(); if len(cells) > 0 { if cells[0] == "/configmaps" { hasCM = true }; if cells[0] == "/secrets" { hasSec = true } } }
    if !hasCM || !hasSec { t.Fatalf("groups: expected /configmaps and /secrets, got %+v", rows) }

    // 4) Enter objects: configmaps
    gvrCM := schema.GroupVersionResource{Group:"", Version:"v1", Resource:"configmaps"}
    objs := NewNamespacedObjectsFolder(deps, gvrCM, "testns")
    kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return objs.Len() > 0 })
    rows = objs.Lines(0, objs.Len())
    foundCM1 := false
    for _, r := range rows { _, cells, _, _ := r.Columns(); if len(cells) > 0 && cells[0] == "cm1" { foundCM1 = true; break } }
    if !foundCM1 { t.Fatalf("objects: cm1 not found") }

    // 5) Enter cm1 keys
    keys := NewConfigMapKeysFolder(deps, "testns", "cm1")
    kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return keys.Len() >= 2 })
    rows = keys.Lines(0, keys.Len())
    hasA, hasB := false, false
    for _, r := range rows { _, cells, _, _ := r.Columns(); if len(cells) > 0 { if cells[0] == "a" { hasA = true }; if cells[0] == "b" { hasB = true } } }
    if !hasA || !hasB { t.Fatalf("cm1 keys: expected a and b") }

    // 6) Cluster-scoped objects: nodes
    gvrNodes := schema.GroupVersionResource{Group:"", Version:"v1", Resource:"nodes"}
    nodes := NewClusterObjectsFolder(deps, gvrNodes)
    kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return nodes.Len() > 0 })
    rows = nodes.Lines(0, nodes.Len())
    foundN1 := false
    for _, r := range rows { _, cells, _, _ := r.Columns(); if len(cells) > 0 && cells[0] == "n1" { foundN1 = true; break } }
    if !foundN1 { t.Fatalf("nodes: n1 not found") }
}
