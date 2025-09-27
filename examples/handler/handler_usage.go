package main

import (
	"fmt"
	"log"

	"github.com/sttts/kc/pkg/handlers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func main() {
	// Create a pod handler
	podHandler := handlers.NewPodHandler()

	// Create a sample pod
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "nginx",
					RestartCount: 2,
				},
			},
		},
	}

	// Get actions for the pod
	actions := podHandler.GetActions(pod)
	fmt.Println("Available actions for pod:")
	for _, action := range actions {
		fmt.Printf("  - %s: %s\n", action.Name, action.Description)
		if action.RequiresConfirmation {
			fmt.Printf("    (requires confirmation)\n")
		}
	}

	// Get display columns
	columns := podHandler.GetDisplayColumns()
	fmt.Println("\nDisplay columns:")
	for _, column := range columns {
		fmt.Printf("  - %s (width: %d, sortable: %v)\n",
			column.Name, column.Width, column.Sortable)
	}

	// Get status
	status := podHandler.GetStatus(pod)
	fmt.Printf("\nPod status: %s\n", status)

	// Get sub-resources
	subResources := podHandler.GetSubResources(pod)
	fmt.Println("\nSub-resources:")
	for _, subResource := range subResources {
		fmt.Printf("  - %s: %s\n", subResource.Name, subResource.Description)
	}

	// Register the handler in the global registry
	gvk := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Pod",
	}

	handlers.Register(gvk, podHandler)

	// Retrieve the handler from the registry
	retrievedHandler, err := handlers.Get(gvk)
	if err != nil {
		log.Fatalf("Failed to get handler: %v", err)
	}

	fmt.Printf("\nRetrieved handler type: %T\n", retrievedHandler)

	// List all registered handlers
	allHandlers := handlers.List()
	fmt.Printf("\nTotal registered handlers: %d\n", len(allHandlers))
	for gvk, handler := range allHandlers {
		fmt.Printf("  - %v: %T\n", gvk, handler)
	}
}
