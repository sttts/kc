package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sttts/kc/pkg/kubeconfig"
	"github.com/sttts/kc/pkg/navigation"
	"github.com/sttts/kc/pkg/resources"
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
	// Choose current context from the selected kubeconfig (prefer $KUBECONFIG if set)
	ctx := selectCurrentContext(kubeMgr)
	if ctx == nil {
		fmt.Println("No current context found; ensure ~/.kube/config exists and has a current-context")
		return
	}
	fmt.Printf("Using context: %s (kubeconfig: %s)\n", ctx.Name, ctx.Kubeconfig.Path)

	// Build a controller-runtime resources.Manager for mapping and fallbacks.
	cfg, err := kubeMgr.CreateClientConfig(ctx)
	if err != nil {
		log.Fatalf("client config: %v", err)
	}
	resMgr, err := resources.NewManager(cfg)
	if err != nil {
		log.Fatalf("resources manager: %v", err)
	}
	if err := resMgr.Start(); err != nil {
		log.Fatalf("start resources manager: %v", err)
	}
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
	if err := nav.BuildHierarchy(); err != nil {
		log.Fatalf("build hierarchy: %v", err)
	}

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

// selectCurrentContext prefers $KUBECONFIG (first path) current-context, else any kubeconfig's current-context, else first.
func selectCurrentContext(mgr *kubeconfig.Manager) *kubeconfig.Context {
	// From env
	if env := os.Getenv("KUBECONFIG"); env != "" {
		sep := ":"
		if runtime.GOOS == "windows" {
			sep = ";"
		}
		for _, p := range strings.Split(env, sep) {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			for _, kc := range mgr.GetKubeconfigs() {
				if sameFile(kc.Path, p) {
					if ctx := mgr.GetCurrentContext(kc); ctx != nil {
						return ctx
					}
				}
			}
		}
	}
	// Any kubeconfig with a current-context
	for _, kc := range mgr.GetKubeconfigs() {
		if ctx := mgr.GetCurrentContext(kc); ctx != nil {
			return ctx
		}
	}
	// Fallback: first discovered context
	cs := mgr.GetContexts()
	if len(cs) > 0 {
		return cs[0]
	}
	return nil
}

func sameFile(a, b string) bool {
	ap, err1 := filepath.Abs(a)
	bp, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	return ap == bp
}
