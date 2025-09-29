package main

import (
    "context"
    "fmt"
    "log"

    "github.com/sttts/kc/pkg/handlers"
    "github.com/sttts/kc/pkg/kubeconfig"
    kccluster "github.com/sttts/kc/internal/cluster"
    corev1 "k8s.io/api/core/v1"
)

func main() {
	// Create a kubeconfig manager
	kubeconfigManager := kubeconfig.NewManager()

	// Discover kubeconfigs
	fmt.Println("Discovering kubeconfigs...")
	err := kubeconfigManager.DiscoverKubeconfigs()
	if err != nil {
		log.Printf("Warning: Failed to discover kubeconfigs: %v", err)
		fmt.Println("This is expected if no kubeconfigs are found.")
		return
	}

    // Get the first kubeconfig context
    contexts := kubeconfigManager.GetContexts()
	if len(contexts) == 0 {
		fmt.Println("No contexts found")
		return
	}

    kctx := contexts[0]
    fmt.Printf("Using context: %s\n", kctx.Name)

	// Create a client for the context (not used in this example)
    _, err = kubeconfigManager.CreateClient(kctx)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Get the client config
    config, err := kubeconfigManager.CreateClientConfig(kctx)
	if err != nil {
		log.Fatalf("Failed to create client config: %v", err)
	}

    // Create a cluster
    cl, err := kccluster.New(config)
    if err != nil { log.Fatalf("cluster: %v", err) }
    ctx := context.TODO()
    go cl.Start(ctx)

	// Register a pod handler
    podHandler := handlers.NewPodHandler()
    // Handlers are demo-only; keep registration out in real app
    _ = podHandler

	// Start the manager
    fmt.Println("Starting cluster...")

	// List pods using the client directly
	fmt.Println("\nListing pods...")
	podList := &corev1.PodList{}
    err = cl.GetClient().List(ctx, podList)
	if err != nil {
		log.Printf("Failed to list pods: %v", err)
	} else {
		fmt.Printf("Found %d pods:\n", len(podList.Items))
		for i, pod := range podList.Items {
			if i >= 5 { // Limit to first 5 pods
				fmt.Printf("  ... and %d more\n", len(podList.Items)-5)
				break
			}
			fmt.Printf("  - %s (namespace: %s, status: %s)\n",
				pod.Name, pod.Namespace, pod.Status.Phase)
		}
	}

	// List namespaces using the client directly
	fmt.Println("\nListing namespaces...")
	namespaceList := &corev1.NamespaceList{}
    err = cl.GetClient().List(ctx, namespaceList)
	if err != nil {
		log.Printf("Failed to list namespaces: %v", err)
	} else {
		fmt.Printf("Found %d namespaces:\n", len(namespaceList.Items))
		for i, ns := range namespaceList.Items {
			if i >= 10 { // Limit to first 10 namespaces
				fmt.Printf("  ... and %d more\n", len(namespaceList.Items)-10)
				break
			}
			fmt.Printf("  - %s (status: %s)\n", ns.Name, ns.Status.Phase)
		}
	}

	// Test handler functionality
	fmt.Println("\nTesting handler functionality...")
	if len(podList.Items) > 0 {
		pod := &podList.Items[0]
    {
        // Get actions
        actions := podHandler.GetActions(pod)
			fmt.Printf("Actions for pod %s:\n", pod.Name)
			for _, action := range actions {
				fmt.Printf("  - %s: %s\n", action.Name, action.Description)
			}

			// Get status
        status := podHandler.GetStatus(pod)
			fmt.Printf("Status: %s\n", status)

			// Get display columns
        columns := podHandler.GetDisplayColumns()
			fmt.Printf("Display columns:\n")
			for _, column := range columns {
				fmt.Printf("  - %s (width: %d, sortable: %v)\n",
					column.Name, column.Width, column.Sortable)
			}
		}
	}

	// Show supported resources
	fmt.Println("\nSupported resource types:")
    supportedResources, err := cl.GetResourceInfos()
    if err != nil {
        log.Printf("Failed to get supported resources: %v", err)
    } else {
        for _, info := range supportedResources {
            fmt.Printf("  - %s (%s/%s)\n", info.Resource, info.GVK.Group, info.GVK.Version)
        }
    }

	fmt.Println("\nResource manager demo completed!")
}
