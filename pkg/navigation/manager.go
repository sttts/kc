package navigation

import (
    "fmt"
    "path/filepath"
    "strings"

    "github.com/sttts/kc/pkg/kubeconfig"
)

// Manager manages the navigation hierarchy
type Manager struct {
    kubeconfigManager *kubeconfig.Manager
    state             *NavigationState
}

// NewManager creates a new navigation manager
func NewManager(kubeMgr *kubeconfig.Manager, _ interface{}) *Manager {
    return &Manager{
        kubeconfigManager: kubeMgr,
        state:             NewNavigationState(),
    }
}

// BuildHierarchy builds the complete navigation hierarchy
func (m *Manager) BuildHierarchy() error {
	// Create root node (virtual directory)
	root := NewNode(NodeTypeDirectory, "Kubernetes", "")
	m.state.Root = root
	m.state.CurrentNode = root

	// Top level: current kubeconfig node, contexts directory, kubeconfigs directory
	if err := m.buildTopLevelNodes(root); err != nil {
		return fmt.Errorf("failed to build top-level nodes: %w", err)
	}

	return nil
}

// buildTopLevelNodes creates: current kubeconfig node, contexts dir, kubeconfigs dir
func (m *Manager) buildTopLevelNodes(root *Node) error {
	kubeconfigs := m.kubeconfigManager.GetKubeconfigs()

	// Current kubeconfig (best-effort: pick the first kubeconfig)
	if len(kubeconfigs) > 0 {
		currentKC := kubeconfigs[0]
		currentKCName := filepath.Base(currentKC.Path)
		currentNode := NewNode(NodeTypeKubeconfig, currentKCName, currentKCName)
		root.AddChild(currentNode)

		// Add the contexts from the current kubeconfig as children for convenience
		contexts := m.kubeconfigManager.GetContextsForKubeconfig(currentKC)
		for _, ctx := range contexts {
			ctxNode := NewNode(NodeTypeContext, ctx.Name, ctx.Name)
			ctxNode.Path = fmt.Sprintf("%s/%s", currentKCName, ctx.Name)
			currentNode.AddChild(ctxNode)
		}
	}

	// Contexts directory with all contexts
	contextsDir := NewNode(NodeTypeDirectory, "Contexts", "Contexts")
	root.AddChild(contextsDir)
	for _, kc := range kubeconfigs {
		contexts := m.kubeconfigManager.GetContextsForKubeconfig(kc)
		for _, ctx := range contexts {
			ctxNode := NewNode(NodeTypeContext, ctx.Name, ctx.Name)
			ctxNode.Path = fmt.Sprintf("%s/%s", filepath.Base(kc.Path), ctx.Name)
			contextsDir.AddChild(ctxNode)
		}
	}

	// Kubeconfigs directory with all kubeconfigs
	kubeconfigsDir := NewNode(NodeTypeDirectory, "Kubeconfigs", "Kubeconfigs")
	root.AddChild(kubeconfigsDir)
	for _, kc := range kubeconfigs {
		name := filepath.Base(kc.Path)
		kcNode := NewNode(NodeTypeKubeconfig, name, name)
		kubeconfigsDir.AddChild(kcNode)

		// Optionally add contexts beneath each kubeconfig
		contexts := m.kubeconfigManager.GetContextsForKubeconfig(kc)
		for _, ctx := range contexts {
			ctxNode := NewNode(NodeTypeContext, ctx.Name, ctx.Name)
			ctxNode.Path = fmt.Sprintf("%s/%s", name, ctx.Name)
			kcNode.AddChild(ctxNode)
		}
	}

	return nil
}

// LoadContextResources removed: legacy resource layer no longer supported here.

// findContextNode finds a context node by name
func (m *Manager) findContextNode(contextName string) *Node {
	return m.findNodeByPath(m.state.Root, contextName)
}

// findNodeByPath recursively finds a node by path
func (m *Manager) findNodeByPath(node *Node, path string) *Node {
	if strings.Contains(node.Path, path) {
		return node
	}

	for _, child := range node.Children {
		if found := m.findNodeByPath(child, path); found != nil {
			return found
		}
	}

	return nil
}

// loadNamespaces removed: legacy resource layer no longer supported here.

// LoadNamespaceResources loads resources for a specific namespace
// LoadNamespaceResources removed: legacy resource layer no longer supported here.

// findNamespaceNode finds a namespace node by name
func (m *Manager) findNamespaceNode(namespaceName string) *Node {
	return m.findNodeByName(m.state.Root, namespaceName, NodeTypeNamespace)
}

// findNodeByName recursively finds a node by name and type
func (m *Manager) findNodeByName(node *Node, name string, nodeType NodeType) *Node {
	if node.Name == name && node.Type == nodeType {
		return node
	}

	for _, child := range node.Children {
		if found := m.findNodeByName(child, name, nodeType); found != nil {
			return found
		}
	}

	return nil
}

// loadResourcesForType loads resources of a specific type
// loadResourcesForType removed: legacy resource layer no longer supported here.

// createObjectList creates an object list for a given GVK
// createObjectList removed in favor of generic dynamic client listing

// GetState returns the current navigation state
func (m *Manager) GetState() *NavigationState {
	return m.state
}

// NavigateTo navigates to a specific node
func (m *Manager) NavigateTo(node *Node) error {
    m.state.SetCurrentNode(node)

    // Legacy resource loading removed.
    return nil
}

// GetCurrentNode returns the current navigation node
func (m *Manager) GetCurrentNode() *Node {
	return m.state.CurrentNode
}

// GetResourceManager returns the current resource manager
// GetResourceManager removed.
