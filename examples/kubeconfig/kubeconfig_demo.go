package main

import (
	"fmt"
	"log"

	"github.com/sschimanski/kc/pkg/kubeconfig"
)

func main() {
	// Create a kubeconfig manager
	manager := kubeconfig.NewManager()
	
	// Discover kubeconfigs
	fmt.Println("Discovering kubeconfigs...")
	err := manager.DiscoverKubeconfigs()
	if err != nil {
		log.Printf("Warning: Failed to discover kubeconfigs: %v", err)
		fmt.Println("This is expected if no kubeconfigs are found.")
	}
	
	// List discovered kubeconfigs
	kubeconfigs := manager.GetKubeconfigs()
	fmt.Printf("\nFound %d kubeconfig(s):\n", len(kubeconfigs))
	for i, kc := range kubeconfigs {
		fmt.Printf("  %d. %s\n", i+1, kc.Path)
	}
	
	// List contexts
	contexts := manager.GetContexts()
	fmt.Printf("\nFound %d context(s):\n", len(contexts))
	for i, ctx := range contexts {
		fmt.Printf("  %d. %s (cluster: %s, namespace: %s)\n", 
			i+1, ctx.Name, ctx.Cluster, ctx.Namespace)
	}
	
	// List clusters
	clusters := manager.GetClusters()
	fmt.Printf("\nFound %d cluster(s):\n", len(clusters))
	for i, cluster := range clusters {
		fmt.Printf("  %d. %s (%s)\n", 
			i+1, cluster.Name, cluster.Server)
	}
	
	// If we have contexts, try to create a client
	if len(contexts) > 0 {
		fmt.Println("\nTesting client creation...")
		ctx := contexts[0]
		fmt.Printf("Creating client for context: %s\n", ctx.Name)
		
		client, err := manager.CreateClient(ctx)
		if err != nil {
			log.Printf("Failed to create client: %v", err)
		} else {
			fmt.Printf("Successfully created client: %T\n", client)
		}
	}
	
	// If we have kubeconfigs, try to create a client from the first one
	if len(kubeconfigs) > 0 {
		fmt.Println("\nTesting client creation from kubeconfig...")
		kc := kubeconfigs[0]
		fmt.Printf("Creating client from kubeconfig: %s\n", kc.Path)
		
		client, err := manager.CreateClientForKubeconfig(kc)
		if err != nil {
			log.Printf("Failed to create client from kubeconfig: %v", err)
		} else {
			fmt.Printf("Successfully created client from kubeconfig: %T\n", client)
		}
	}
	
	fmt.Println("\nKubeconfig discovery completed!")
}
