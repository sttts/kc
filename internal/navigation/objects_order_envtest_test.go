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

func TestClusterObjectsOrderAndAgeEnvtest(t *testing.T) {
    // Start envtest
    testEnv := &envtest.Environment{}
    cfg, err := testEnv.Start()
    if err != nil || cfg == nil { t.Fatalf("start envtest: %v", err) }
    defer func(){ _ = testEnv.Stop() }()

    // Seed three namespaces with slight delays to vary creation times
    scheme := runtime.NewScheme(); _ = corev1.AddToScheme(scheme)
    cli, err := crclient.New(cfg, crclient.Options{Scheme: scheme})
    if err != nil { t.Fatalf("new client: %v", err) }
    ctx := context.TODO()
    for _, n := range []string{"a", "b", "c"} {
        if err := cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: n}}); err != nil { t.Fatalf("create ns %s: %v", n, err) }
        time.Sleep(20 * time.Millisecond)
    }

    // kc cluster
    cl, err := kccluster.New(cfg)
    if err != nil { t.Fatalf("kccluster: %v", err) }
    go cl.Start(ctx)

    // Deps with objects order toggles
    makeDeps := func(order string) Deps {
        return Deps{Cl: cl, Ctx: ctx, CtxName: "envtest", ViewOptions: func() ViewOptions { return ViewOptions{Columns: "normal", ObjectsOrder: order} }}
    }
    gvrNS := schema.GroupVersionResource{Group:"", Version:"v1", Resource:"namespaces"}

    // Order by name ascending
    f1 := NewClusterObjectsFolder(makeDeps("name"), gvrNS, []string{"namespaces"})
    kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return f1.Len() >= 3 })
    rows := f1.Lines(0, 3)
    got := []string{}
    for _, r := range rows {
        _, cells, _, _ := r.Columns()
        if len(cells) > 0 {
            v := cells[0]
            if len(v) > 0 && v[0] == '/' { v = v[1:] }
            got = append(got, v)
        }
    }
    if !(got[0] == "a" && got[1] == "b") { t.Fatalf("name asc order unexpected: %+v", got) }
    // Verify Age column exists
    cols := f1.Columns()
    if len(cols) == 0 || cols[len(cols)-1].Title != "Age" { t.Fatalf("missing Age column: %+v", cols) }

    // Order by -name
    f2 := NewClusterObjectsFolder(makeDeps("-name"), gvrNS, []string{"namespaces"})
    kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return f2.Len() >= 3 })
    rows = f2.Lines(0, 3)
    got = got[:0]
    for _, r := range rows {
        _, cells, _, _ := r.Columns()
        if len(cells) > 0 {
            v := cells[0]
            if len(v) > 0 && v[0] == '/' { v = v[1:] }
            got = append(got, v)
        }
    }
    if !(got[0] == "c" && got[1] == "b") { t.Fatalf("name desc order unexpected: %+v", got) }

    // Order by creation
    f3 := NewClusterObjectsFolder(makeDeps("creation"), gvrNS, []string{"namespaces"})
    kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return f3.Len() >= 3 })
    rows = f3.Lines(0, 3)
    got = got[:0]
    for _, r := range rows {
        _, cells, _, _ := r.Columns()
        if len(cells) > 0 {
            v := cells[0]
            if len(v) > 0 && v[0] == '/' { v = v[1:] }
            got = append(got, v)
        }
    }
    if !(got[0] == "a" && got[2] == "c") { t.Fatalf("creation asc order unexpected: %+v", got) }

    // Order by -creation
    f4 := NewClusterObjectsFolder(makeDeps("-creation"), gvrNS, []string{"namespaces"})
    kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return f4.Len() >= 3 })
    rows = f4.Lines(0, 3)
    got = got[:0]
    for _, r := range rows {
        _, cells, _, _ := r.Columns()
        if len(cells) > 0 {
            v := cells[0]
            if len(v) > 0 && v[0] == '/' { v = v[1:] }
            got = append(got, v)
        }
    }
    if !(got[0] == "c" && got[2] == "a") { t.Fatalf("creation desc order unexpected: %+v", got) }
}
