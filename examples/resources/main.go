package main

import (
	"context"
	"fmt"
	"log"

	"github.com/sschimanski/kc/pkg/handlers"
	"github.com/sschimanski/kc/pkg/kubeconfig"
	"github.com/sschimanski/kc/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	// Get the first context
	contexts := kubeconfigManager.GetContexts()
	if len(contexts) == 0 {
		fmt.Println("No contexts found")
		return
	}

	ctx := contexts[0]
	fmt.Printf("Using context: %s\n", ctx.Name)

	// Create a client for the context (not used in this example)
	_, err = kubeconfigManager.CreateClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Get the client config
	config, err := kubeconfigManager.CreateClientConfig(ctx)
	if err != nil {
		log.Fatalf("Failed to create client config: %v", err)
	}

	// Create a resource manager
	resourceManager, err := resources.NewManager(config)
	if err != nil {
		log.Fatalf("Failed to create resource manager: %v", err)
	}

	// Register a pod handler
	podHandler := handlers.NewPodHandler()
	podGVK := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Pod",
	}
	resourceManager.RegisterHandler(podGVK, podHandler)

	// Start the manager
	fmt.Println("Starting resource manager...")
	if err := resourceManager.Start(); err != nil {
		log.Fatalf("Failed to start resource manager: %v", err)
	}
	defer resourceManager.Stop()

	// List pods using the client directly
	fmt.Println("\nListing pods...")
	podList := &corev1.PodList{}
	err = resourceManager.Client().List(context.Background(), podList)
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
	err = resourceManager.Client().List(context.Background(), namespaceList)
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
		handler, err := resourceManager.GetHandler(podGVK)
		if err != nil {
			log.Printf("Failed to get handler: %v", err)
		} else {
			// Get actions
			actions := handler.GetActions(pod)
			fmt.Printf("Actions for pod %s:\n", pod.Name)
			for _, action := range actions {
				fmt.Printf("  - %s: %s\n", action.Name, action.Description)
			}

			// Get status
			status := handler.GetStatus(pod)
			fmt.Printf("Status: %s\n", status)

			// Get display columns
			columns := handler.GetDisplayColumns()
			fmt.Printf("Display columns:\n")
			for _, column := range columns {
				fmt.Printf("  - %s (width: %d, sortable: %v)\n",
					column.Name, column.Width, column.Sortable)
			}
		}
	}

	// Show supported resources
	fmt.Println("\nSupported resource types:")
	supportedResources, err := resourceManager.GetSupportedResources()
	if err != nil {
		log.Printf("Failed to get supported resources: %v", err)
	} else {
		for _, gvk := range supportedResources {
			fmt.Printf("  - %s\n", gvk.String())
		}
	}

	fmt.Println("\nResource manager demo completed!")
}
