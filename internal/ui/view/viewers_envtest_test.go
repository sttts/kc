package view

import (
    "context"
    "testing"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/runtime/schema"
    metamapper "k8s.io/apimachinery/pkg/api/meta"
    "sigs.k8s.io/controller-runtime/pkg/envtest"

    kccluster "github.com/sttts/kc/internal/cluster"
    kctesting "github.com/sttts/kc/internal/testing"
)

// testCtx implements the viewers Context using the kc Cluster.
type testCtx struct{ cl *kccluster.Cluster }

func (t testCtx) RESTMapper() metamapper.RESTMapper { return t.cl.RESTMapper() }

func (t testCtx) GetObject(gvk schema.GroupVersionKind, namespace, name string) (map[string]interface{}, error) {
    gvr, err := t.cl.GVKToGVR(gvk)
    if err != nil { return nil, err }
    u, err := t.cl.GetByGVR(context.TODO(), gvr, namespace, name)
    if err != nil { return nil, err }
    return u.Object, nil
}

func TestConfigKeyViewAndSecretEnvtest(t *testing.T) {
    // Start envtest API server
    testEnv := &envtest.Environment{}
    cfg, err := testEnv.Start()
    if err != nil || cfg == nil { t.Fatalf("start envtest: %v", err) }
    defer func(){ _ = testEnv.Stop() }()

    // Start kc cluster
    cl, err := kccluster.New(cfg)
    if err != nil { t.Fatalf("cluster: %v", err) }
    ctx := context.TODO()
    go cl.Start(ctx)

    // Seed namespace, configmap, and secret via cluster's client
    sch := runtime.NewScheme(); _ = corev1.AddToScheme(sch)
    c := cl.GetClient()
    if err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "viewtest"}}); err != nil { t.Fatalf("create ns: %v", err) }
    cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "viewtest"}, Data: map[string]string{"a": "A", "json": "{\"x\":1}"}}
    if err := c.Create(ctx, cm); err != nil { t.Fatalf("create cm: %v", err) }
    sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec1", Namespace: "viewtest"}, Data: map[string][]byte{"b": []byte("B"), "bin": {0xff, 0x00}}}
    if err := c.Create(ctx, sec); err != nil { t.Fatalf("create secret: %v", err) }

    // Wait until cache observes objects
    kctesting.Eventually(t, 5_000_000_000, 50_000_000, func() bool {
        _, err := cl.GetByGVR(ctx, schema.GroupVersionResource{Group:"", Version:"v1", Resource:"configmaps"}, "viewtest", "cm1")
        return err == nil
    })

    tc := testCtx{cl: cl}

    // ConfigMap key 'a'
    v1 := &ConfigKeyView{Namespace: "viewtest", Name: "cm1", Key: "a", IsSecret: false}
    title, body, err := v1.BuildView(tc)
    if err != nil { t.Fatalf("configmap key view: %v", err) }
    if title != "cm1:a" || body != "A" {
        t.Fatalf("configmap view mismatch: title=%q body=%q", title, body)
    }

    // Secret key 'b' (UTF-8)
    v2 := &ConfigKeyView{Namespace: "viewtest", Name: "sec1", Key: "b", IsSecret: true}
    _, body2, err := v2.BuildView(tc)
    if err != nil { t.Fatalf("secret key view: %v", err) }
    if body2 != "B" { t.Fatalf("secret decoded value = %q, want %q", body2, "B") }

    // Secret key 'bin' (non-UTF-8) → base64 shown
    v3 := &ConfigKeyView{Namespace: "viewtest", Name: "sec1", Key: "bin", IsSecret: true}
    _, body3, err := v3.BuildView(tc)
    if err != nil { t.Fatalf("secret bin key view: %v", err) }
    if body3 == "\xff\x00" || body3 == string([]byte{0xff, 0x00}) {
        t.Fatalf("expected base64 output for non-UTF8 secret, got raw bytes")
    }
}

