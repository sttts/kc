package main

import (
	"fmt"
	"log"

	"github.com/sttts/kc/pkg/kubeconfig"
	"github.com/sttts/kc/pkg/navigation"
)

func main() {
	fmt.Println("Kubernetes Commander - Navigation Demo")
	fmt.Println("=====================================")

	// Create kubeconfig manager
	kubeMgr := kubeconfig.NewManager()

	// Discover kubeconfigs
	fmt.Println("\n1. Discovering kubeconfigs...")
	err := kubeMgr.DiscoverKubeconfigs()
	if err != nil {
		log.Printf("Warning: Failed to discover kubeconfigs: %v", err)
		fmt.Println("This is expected if no kubeconfigs are found.")
		return
	}

	// Create navigation manager
	navMgr := navigation.NewManager(kubeMgr, nil)

	// Build hierarchy
	fmt.Println("\n2. Building navigation hierarchy...")
	err = navMgr.BuildHierarchy()
	if err != nil {
		log.Fatalf("Failed to build hierarchy: %v", err)
	}

	// Display hierarchy
	fmt.Println("\n3. Navigation hierarchy:")
	displayHierarchy(navMgr.GetState().Root, 0)

	// Show current state
	state := navMgr.GetState()
	fmt.Printf("\n4. Current node: %s (type: %v)\n",
		state.CurrentNode.Name, state.CurrentNode.Type)

	// Test node operations
	fmt.Println("\n5. Testing node operations...")
	testNodeOperations(state)

	fmt.Println("\nNavigation demo completed!")
}

func displayHierarchy(node *navigation.Node, depth int) {
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}

	nodeTypeStr := getNodeTypeString(node.Type)
	fmt.Printf("%s- %s (%s)\n", indent, node.Name, nodeTypeStr)

	for _, child := range node.Children {
		displayHierarchy(child, depth+1)
	}
}

func getNodeTypeString(nodeType navigation.NodeType) string {
	switch nodeType {
	case navigation.NodeTypeKubeconfig:
		return "Kubeconfig"
	case navigation.NodeTypeContext:
		return "Context"
	case navigation.NodeTypeNamespace:
		return "Namespace"
	case navigation.NodeTypeResource:
		return "Resource"
	case navigation.NodeTypeClusterResource:
		return "Cluster Resource"
	default:
		return "Unknown"
	}
}

func testNodeOperations(state *navigation.NavigationState) {
	// Test expanding/collapsing
	if len(state.Root.Children) > 0 {
		firstChild := state.Root.Children[0]
		fmt.Printf("Testing expand/collapse on: %s\n", firstChild.Name)

		// Toggle expanded state
		state.ToggleExpanded(firstChild)
		fmt.Printf("  Expanded: %v\n", state.IsExpanded(firstChild))

		state.ToggleExpanded(firstChild)
		fmt.Printf("  Expanded after toggle: %v\n", state.IsExpanded(firstChild))
	}

	// Test selection
	if len(state.Root.Children) > 0 {
		firstChild := state.Root.Children[0]
		fmt.Printf("Testing selection on: %s\n", firstChild.Name)

		// Add to selection
		state.AddSelectedNode(firstChild)
		fmt.Printf("  Selected nodes: %d\n", len(state.GetSelectedNodes()))

		// Clear selection
		state.ClearSelection()
		fmt.Printf("  Selected nodes after clear: %d\n", len(state.GetSelectedNodes()))
	}
}
