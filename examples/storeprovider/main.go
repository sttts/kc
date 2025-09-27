package main

import (
    "fmt"
    "log"
    "time"

    "github.com/sschimanski/kc/pkg/kubeconfig"
    "github.com/sschimanski/kc/pkg/navigation"
    "github.com/sschimanski/kc/pkg/resources"
)

// Demonstrates wiring a per-context controller-runtime cluster/cache store
// into the navigation manager using ClusterPool and CRStoreProvider.
func main() {
    fmt.Println("kc StoreProvider Demo")
    fmt.Println("======================")

    // Discover kubeconfigs and pick the first context
    kubeMgr := kubeconfig.NewManager()
    if err := kubeMgr.DiscoverKubeconfigs(); err != nil {
        log.Printf("Failed to discover kubeconfigs: %v", err)
        return
    }
    contexts := kubeMgr.GetContexts()
    if len(contexts) == 0 {
        fmt.Println("No contexts discovered; ensure ~/.kube/config exists")
        return
    }
    ctx := contexts[0]
    fmt.Printf("Using context: %s (kubeconfig: %s)\n", ctx.Name, ctx.Kubeconfig.Path)

    // Build a controller-runtime resources.Manager for mapping and fallbacks.
    cfg, err := kubeMgr.CreateClientConfig(ctx)
    if err != nil { log.Fatalf("client config: %v", err) }
    resMgr, err := resources.NewManager(cfg)
    if err != nil { log.Fatalf("resources manager: %v", err) }
    if err := resMgr.Start(); err != nil { log.Fatalf("start resources manager: %v", err) }
    defer resMgr.Stop()

    // Create a ClusterPool with a 2-minute idle TTL and start eviction loop.
    pool := resources.NewClusterPool(2 * time.Minute)
    pool.Start()
    defer pool.Stop()

    // Create a StoreProvider bound to this kubeconfig+context.
    key := resources.ClusterKey{KubeconfigPath: ctx.Kubeconfig.Path, ContextName: ctx.Name}
    storeProvider := resources.NewCRStoreProvider(pool, key)

    // Wire into navigation and load context resources (namespaces, etc.).
    nav := navigation.NewManager(kubeMgr, resMgr)
    nav.SetStoreProvider(storeProvider)
    if err := nav.BuildHierarchy(); err != nil { log.Fatalf("build hierarchy: %v", err) }

    if err := nav.LoadContextResources(ctx); err != nil {
        log.Fatalf("load context resources: %v", err)
    }

    // Print discovered namespaces under this context node.
    state := nav.GetState()
    ctxNode := findContextNodeByName(state.Root, ctx.Name)
    if ctxNode == nil {
        fmt.Println("Context node not found in hierarchy")
        return
    }
    fmt.Printf("\nNamespaces under context %s:\n", ctx.Name)
    for _, child := range ctxNode.Children {
        fmt.Printf(" - %s\n", child.Name)
    }

    fmt.Println("\nStoreProvider demo completed.")
}

func findContextNodeByName(n *navigation.Node, name string) *navigation.Node {
    if n.Type == navigation.NodeTypeContext && n.Name == name {
        return n
    }
    for _, c := range n.Children {
        if got := findContextNodeByName(c, name); got != nil {
            return got
        }
    }
    return nil
}

