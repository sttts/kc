package navigation

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NodeType represents the type of navigation node
type NodeType int

const (
	NodeTypeKubeconfig NodeType = iota
	NodeTypeContext
	NodeTypeNamespace
	NodeTypeResource
	NodeTypeClusterResource
	// NodeTypeDirectory represents a virtual directory node (e.g. "Current", "Contexts", "Kubeconfigs")
	NodeTypeDirectory
	// NodeTypeResourceType represents a resource type (GVK/GVR) grouping node
	NodeTypeResourceType
)

// Node represents a navigation node in the hierarchy
type Node struct {
	Type       NodeType
	Name       string
	Path       string
	Parent     *Node
	Children   []*Node
	Object     client.Object               // For resource nodes
	GVK        schema.GroupVersionKind     // For resource nodes
	GVR        schema.GroupVersionResource // For resource nodes
	IsExpanded bool
	IsSelected bool
}

// NewNode creates a new navigation node
func NewNode(nodeType NodeType, name, path string) *Node {
	return &Node{
		Type:     nodeType,
		Name:     name,
		Path:     path,
		Children: make([]*Node, 0),
	}
}

// AddChild adds a child node
func (n *Node) AddChild(child *Node) {
	child.Parent = n
	n.Children = append(n.Children, child)
}

// IsLeaf returns true if the node has no children
func (n *Node) IsLeaf() bool {
	return len(n.Children) == 0
}

// GetFullPath returns the full path from root to this node
func (n *Node) GetFullPath() string {
	if n.Parent == nil {
		return n.Path
	}
	return n.Parent.GetFullPath() + "/" + n.Path
}

// FindChild finds a child node by name
func (n *Node) FindChild(name string) *Node {
	for _, child := range n.Children {
		if child.Name == name {
			return child
		}
	}
	return nil
}

// ClearChildren removes all children
func (n *Node) ClearChildren() {
	n.Children = make([]*Node, 0)
}

// NavigationState represents the current navigation state
type NavigationState struct {
	Root          *Node
	CurrentNode   *Node
	SelectedNodes []*Node
	ExpandedNodes map[string]bool
	Filter        string
	SortBy        string
	SortAscending bool
}

// NewNavigationState creates a new navigation state
func NewNavigationState() *NavigationState {
	return &NavigationState{
		ExpandedNodes: make(map[string]bool),
	}
}

// SetCurrentNode sets the current navigation node
func (ns *NavigationState) SetCurrentNode(node *Node) {
	ns.CurrentNode = node
}

// ToggleExpanded toggles the expanded state of a node
func (ns *NavigationState) ToggleExpanded(node *Node) {
	path := node.GetFullPath()
	ns.ExpandedNodes[path] = !ns.ExpandedNodes[path]
	node.IsExpanded = ns.ExpandedNodes[path]
}

// IsExpanded returns true if a node is expanded
func (ns *NavigationState) IsExpanded(node *Node) bool {
	path := node.GetFullPath()
	return ns.ExpandedNodes[path]
}

// AddSelectedNode adds a node to the selection
func (ns *NavigationState) AddSelectedNode(node *Node) {
	// Remove if already selected
	for i, selected := range ns.SelectedNodes {
		if selected == node {
			ns.SelectedNodes = append(ns.SelectedNodes[:i], ns.SelectedNodes[i+1:]...)
			node.IsSelected = false
			return
		}
	}

	// Add to selection
	ns.SelectedNodes = append(ns.SelectedNodes, node)
	node.IsSelected = true
}

// ClearSelection clears all selected nodes
func (ns *NavigationState) ClearSelection() {
	for _, node := range ns.SelectedNodes {
		node.IsSelected = false
	}
	ns.SelectedNodes = make([]*Node, 0)
}

// GetSelectedNodes returns all selected nodes
func (ns *NavigationState) GetSelectedNodes() []*Node {
	return ns.SelectedNodes
}
