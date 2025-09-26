package navigation

import (
	"testing"
)

func TestNewNode(t *testing.T) {
	node := NewNode(NodeTypeNamespace, "test-namespace", "test-namespace")

	if node.Type != NodeTypeNamespace {
		t.Errorf("Expected type %v, got %v", NodeTypeNamespace, node.Type)
	}

	if node.Name != "test-namespace" {
		t.Errorf("Expected name 'test-namespace', got '%s'", node.Name)
	}

	if node.Path != "test-namespace" {
		t.Errorf("Expected path 'test-namespace', got '%s'", node.Path)
	}

	if len(node.Children) != 0 {
		t.Errorf("Expected empty children, got %d", len(node.Children))
	}
}

func TestAddChild(t *testing.T) {
	parent := NewNode(NodeTypeContext, "test-context", "test-context")
	child := NewNode(NodeTypeNamespace, "test-namespace", "test-namespace")

	parent.AddChild(child)

	if len(parent.Children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(parent.Children))
	}

	if parent.Children[0] != child {
		t.Error("Child not added correctly")
	}

	if child.Parent != parent {
		t.Error("Parent not set correctly")
	}
}

func TestIsLeaf(t *testing.T) {
	node := NewNode(NodeTypeNamespace, "test-namespace", "test-namespace")

	if !node.IsLeaf() {
		t.Error("Expected node to be a leaf")
	}

	child := NewNode(NodeTypeResource, "test-resource", "test-resource")
	node.AddChild(child)

	if node.IsLeaf() {
		t.Error("Expected node to not be a leaf after adding child")
	}
}

func TestGetFullPath(t *testing.T) {
	root := NewNode(NodeTypeKubeconfig, "kubeconfig", "kubeconfig")
	context := NewNode(NodeTypeContext, "context", "context")
	namespace := NewNode(NodeTypeNamespace, "namespace", "namespace")

	root.AddChild(context)
	context.AddChild(namespace)

	expectedPath := "kubeconfig/context/namespace"
	if namespace.GetFullPath() != expectedPath {
		t.Errorf("Expected path '%s', got '%s'", expectedPath, namespace.GetFullPath())
	}
}

func TestFindChild(t *testing.T) {
	parent := NewNode(NodeTypeContext, "test-context", "test-context")
	child1 := NewNode(NodeTypeNamespace, "namespace1", "namespace1")
	child2 := NewNode(NodeTypeNamespace, "namespace2", "namespace2")

	parent.AddChild(child1)
	parent.AddChild(child2)

	found := parent.FindChild("namespace1")
	if found != child1 {
		t.Error("FindChild did not return correct child")
	}

	notFound := parent.FindChild("nonexistent")
	if notFound != nil {
		t.Error("FindChild should return nil for nonexistent child")
	}
}

func TestClearChildren(t *testing.T) {
	parent := NewNode(NodeTypeContext, "test-context", "test-context")
	child1 := NewNode(NodeTypeNamespace, "namespace1", "namespace1")
	child2 := NewNode(NodeTypeNamespace, "namespace2", "namespace2")

	parent.AddChild(child1)
	parent.AddChild(child2)

	if len(parent.Children) != 2 {
		t.Errorf("Expected 2 children, got %d", len(parent.Children))
	}

	parent.ClearChildren()

	if len(parent.Children) != 0 {
		t.Errorf("Expected 0 children after clear, got %d", len(parent.Children))
	}
}

func TestNewNavigationState(t *testing.T) {
	state := NewNavigationState()

	if state.Root != nil {
		t.Error("Expected root to be nil initially")
	}

	if state.CurrentNode != nil {
		t.Error("Expected current node to be nil initially")
	}

	if len(state.SelectedNodes) != 0 {
		t.Error("Expected empty selected nodes initially")
	}

	if state.ExpandedNodes == nil {
		t.Error("Expected expanded nodes map to be initialized")
	}
}

func TestSetCurrentNode(t *testing.T) {
	state := NewNavigationState()
	node := NewNode(NodeTypeNamespace, "test-namespace", "test-namespace")

	state.SetCurrentNode(node)

	if state.CurrentNode != node {
		t.Error("Current node not set correctly")
	}
}

func TestToggleExpanded(t *testing.T) {
	state := NewNavigationState()
	node := NewNode(NodeTypeNamespace, "test-namespace", "test-namespace")

	// Initially not expanded
	if state.IsExpanded(node) {
		t.Error("Expected node to not be expanded initially")
	}

	// Toggle to expanded
	state.ToggleExpanded(node)
	if !state.IsExpanded(node) {
		t.Error("Expected node to be expanded after toggle")
	}

	// Toggle back to collapsed
	state.ToggleExpanded(node)
	if state.IsExpanded(node) {
		t.Error("Expected node to be collapsed after second toggle")
	}
}

func TestAddSelectedNode(t *testing.T) {
	state := NewNavigationState()
	node1 := NewNode(NodeTypeResource, "resource1", "resource1")
	node2 := NewNode(NodeTypeResource, "resource2", "resource2")

	// Add first node
	state.AddSelectedNode(node1)
	if len(state.SelectedNodes) != 1 {
		t.Errorf("Expected 1 selected node, got %d", len(state.SelectedNodes))
	}

	if !node1.IsSelected {
		t.Error("Expected node1 to be selected")
	}

	// Add second node
	state.AddSelectedNode(node2)
	if len(state.SelectedNodes) != 2 {
		t.Errorf("Expected 2 selected nodes, got %d", len(state.SelectedNodes))
	}

	// Remove first node by adding it again
	state.AddSelectedNode(node1)
	if len(state.SelectedNodes) != 1 {
		t.Errorf("Expected 1 selected node after toggle, got %d", len(state.SelectedNodes))
	}

	if node1.IsSelected {
		t.Error("Expected node1 to not be selected after toggle")
	}

	if !node2.IsSelected {
		t.Error("Expected node2 to still be selected")
	}
}

func TestClearSelection(t *testing.T) {
	state := NewNavigationState()
	node1 := NewNode(NodeTypeResource, "resource1", "resource1")
	node2 := NewNode(NodeTypeResource, "resource2", "resource2")

	state.AddSelectedNode(node1)
	state.AddSelectedNode(node2)

	if len(state.SelectedNodes) != 2 {
		t.Errorf("Expected 2 selected nodes, got %d", len(state.SelectedNodes))
	}

	state.ClearSelection()

	if len(state.SelectedNodes) != 0 {
		t.Errorf("Expected 0 selected nodes after clear, got %d", len(state.SelectedNodes))
	}

	if node1.IsSelected || node2.IsSelected {
		t.Error("Expected nodes to not be selected after clear")
	}
}

func TestGetSelectedNodes(t *testing.T) {
	state := NewNavigationState()
	node1 := NewNode(NodeTypeResource, "resource1", "resource1")
	node2 := NewNode(NodeTypeResource, "resource2", "resource2")

	state.AddSelectedNode(node1)
	state.AddSelectedNode(node2)

	selected := state.GetSelectedNodes()
	if len(selected) != 2 {
		t.Errorf("Expected 2 selected nodes, got %d", len(selected))
	}

	// Check that the returned slice contains the expected nodes
	found1, found2 := false, false
	for _, node := range selected {
		if node == node1 {
			found1 = true
		}
		if node == node2 {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Error("Expected both nodes to be in selected nodes")
	}
}

func TestResourceTypeNode(t *testing.T) {
	n := NewNode(NodeTypeResourceType, "Deployment/apps/v1", "Deployment/apps/v1")
	if n.Type != NodeTypeResourceType {
		t.Fatalf("Type = %v, want %v", n.Type, NodeTypeResourceType)
	}
	if n.GetFullPath() != "Deployment/apps/v1" {
		t.Fatalf("GetFullPath() = %s, want %s", n.GetFullPath(), "Deployment/apps/v1")
	}
}
