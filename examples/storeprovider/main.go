package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "runtime"
    "strings"
    "time"

    "github.com/sttts/kc/pkg/kubeconfig"
    kccluster "github.com/sttts/kc/internal/cluster"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

// Demonstrates kccluster.Pool (controller-runtime cluster per context) and a
// simple namespace listing using the dynamic helpers. This replaces the old
// StoreProvider demo which used a custom resources layer.
func main() {
    fmt.Println("kc Cluster Pool Demo")
    fmt.Println("====================")

    // Discover kubeconfigs and pick the current context
    kubeMgr := kubeconfig.NewManager()
    if err := kubeMgr.DiscoverKubeconfigs(); err != nil {
        log.Printf("Failed to discover kubeconfigs: %v", err)
        return
    }
    ctx := selectCurrentContext(kubeMgr)
    if ctx == nil {
        fmt.Println("No current context found; ensure ~/.kube/config exists and has a current-context")
        return
    }
    fmt.Printf("Using context: %s (kubeconfig: %s)\n", ctx.Name, ctx.Kubeconfig.Path)

    // Create a cluster pool with 2m TTL and start it
    pool := kccluster.NewPool(2 * time.Minute)
    pool.Start()
    defer pool.Stop()

    // Get (and start) a cluster for this kubeconfig+context
    appCtx := context.TODO()
    key := kccluster.Key{KubeconfigPath: ctx.Kubeconfig.Path, ContextName: ctx.Name}
    cl, err := pool.Get(appCtx, key)
    if err != nil { log.Fatalf("pool get: %v", err) }

    // List namespaces using the cluster helpers
    gvrNS, err := cl.GVKToGVR(schema.GroupVersionKind{Group:"", Version:"v1", Kind:"Namespace"})
    if err != nil { log.Fatalf("map GVK to GVR: %v", err) }
    ul, err := cl.ListByGVR(appCtx, gvrNS, "")
    if err != nil { log.Fatalf("list namespaces: %v", err) }
    fmt.Printf("\nNamespaces (%d):\n", len(ul.Items))
    for _, it := range ul.Items { fmt.Printf(" - %s\n", it.GetName()) }

    fmt.Println("\nCluster pool demo completed.")
}

// selectCurrentContext prefers $KUBECONFIG (first path) current-context, else any kubeconfig's current-context, else first.
func selectCurrentContext(mgr *kubeconfig.Manager) *kubeconfig.Context {
    // From env
    if env := os.Getenv("KUBECONFIG"); env != "" {
        sep := ":"
        if runtime.GOOS == "windows" { sep = ";" }
        for _, p := range strings.Split(env, sep) {
            p = strings.TrimSpace(p)
            if p == "" { continue }
            for _, kc := range mgr.GetKubeconfigs() {
                if sameFile(kc.Path, p) {
                    if ctx := mgr.GetCurrentContext(kc); ctx != nil { return ctx }
                }
            }
        }
    }
    // Any kubeconfig with a current-context
    for _, kc := range mgr.GetKubeconfigs() {
        if ctx := mgr.GetCurrentContext(kc); ctx != nil { return ctx }
    }
    // Fallback: first discovered context
    cs := mgr.GetContexts()
    if len(cs) > 0 { return cs[0] }
    return nil
}

func sameFile(a, b string) bool {
    ap, err1 := filepath.Abs(a)
    bp, err2 := filepath.Abs(b)
    if err1 != nil || err2 != nil { return a == b }
    return ap == bp
}
