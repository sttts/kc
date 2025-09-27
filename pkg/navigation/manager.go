package navigation

import (
    "context"
    "fmt"
    "path/filepath"
    "sort"
    "strings"

    "github.com/sschimanski/kc/pkg/kubeconfig"
    "github.com/sschimanski/kc/pkg/resources"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

// Manager manages the navigation hierarchy
type Manager struct {
    kubeconfigManager *kubeconfig.Manager
    resourceManager   *resources.Manager
    state             *NavigationState
    storeProvider     resources.StoreProvider
}

// NewManager creates a new navigation manager
func NewManager(kubeMgr *kubeconfig.Manager, resourceMgr *resources.Manager) *Manager {
    return &Manager{
        kubeconfigManager: kubeMgr,
        resourceManager:   resourceMgr,
        state:             NewNavigationState(),
    }
}

// SetStoreProvider injects a resources.StoreProvider bound to the active kubeconfig+context.
func (m *Manager) SetStoreProvider(p resources.StoreProvider) { m.storeProvider = p }

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

// LoadContextResources loads resources for a specific context
func (m *Manager) LoadContextResources(ctx *kubeconfig.Context) error {
	// Create resource manager for this context
	config, err := m.kubeconfigManager.CreateClientConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to create client config: %w", err)
	}

	resourceMgr, err := resources.NewManager(config)
	if err != nil {
		return fmt.Errorf("failed to create resource manager: %w", err)
	}

	if err := resourceMgr.Start(); err != nil {
		return fmt.Errorf("failed to start resource manager: %w", err)
	}

	m.resourceManager = resourceMgr

	// Find the context node and load its resources
	contextNode := m.findContextNode(ctx.Name)
	if contextNode == nil {
		return fmt.Errorf("context node not found: %s", ctx.Name)
	}

	// Clear existing children
	contextNode.ClearChildren()

	// Add cluster-wide resources
	clusterNode := NewNode(NodeTypeClusterResource, "Cluster Resources", "cluster")
	contextNode.AddChild(clusterNode)

	// Add namespaces
	if err := m.loadNamespaces(contextNode); err != nil {
		return fmt.Errorf("failed to load namespaces: %w", err)
	}

	return nil
}

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

// loadNamespaces loads namespace nodes
func (m *Manager) loadNamespaces(contextNode *Node) error {
    if m.resourceManager == nil {
        return fmt.Errorf("no resource manager configured")
    }
    // Prefer store provider (informer-backed); fall back to manager.ListNamespaces.
    var list *unstructured.UnstructuredList
    var err error
    if m.storeProvider != nil {
        gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
        gvr, mapErr := m.resourceManager.GVKToGVR(gvk)
        if mapErr == nil {
            list, err = m.storeProvider.Store().List(context.TODO(), resources.StoreKey{GVR: gvr, Namespace: ""})
        } else {
            err = mapErr
        }
    } else {
        // Generic namespace listing via resources.Manager
        list, err = m.resourceManager.ListNamespaces()
    }
    if err != nil {
        return fmt.Errorf("failed to list namespaces: %w", err)
    }

	// Sort by name
	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].GetName() < list.Items[j].GetName()
	})

	for i := range list.Items {
		ns := list.Items[i]
		namespaceNode := NewNode(NodeTypeNamespace, ns.GetName(), ns.GetName())
		contextNode.AddChild(namespaceNode)
	}

	return nil
}

// LoadNamespaceResources loads resources for a specific namespace
func (m *Manager) LoadNamespaceResources(namespaceName string) error {
	if m.resourceManager == nil {
		return fmt.Errorf("no resource manager configured")
	}
	// Find the namespace node
	namespaceNode := m.findNamespaceNode(namespaceName)
	if namespaceNode == nil {
		return fmt.Errorf("namespace node not found: %s", namespaceName)
	}

	// Clear existing children
	namespaceNode.ClearChildren()

	// Get supported resource types via discovery
	supportedResources, err := m.resourceManager.GetSupportedResources()
	if err != nil {
		return fmt.Errorf("failed to get supported resources: %w", err)
	}

	// Group by resource type (GVK) before listing instances
	// Create a ResourceType node then populate instance children
	for _, gvk := range supportedResources {
		// Create a grouping node for the resource type
		groupName := gvk.Kind
		if gvk.Group != "" {
			groupName = fmt.Sprintf("%s.%s/%s", gvk.Kind, gvk.Group, gvk.Version)
		} else {
			groupName = fmt.Sprintf("%s/%s", gvk.Kind, gvk.Version)
		}
		typeNode := NewNode(NodeTypeResourceType, groupName, groupName)
		typeNode.GVK = gvk
		namespaceNode.AddChild(typeNode)

		// Load instances under the type node
		if err := m.loadResourcesForType(typeNode, gvk, namespaceName); err != nil {
			fmt.Printf("Warning: failed to load resources for %s: %v\n", gvk.String(), err)
		}
	}

	return nil
}

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
func (m *Manager) loadResourcesForType(parentNode *Node, gvk schema.GroupVersionKind, namespace string) error {
    var list *unstructured.UnstructuredList
    var err error
    if m.storeProvider != nil {
        gvr, mapErr := m.resourceManager.GVKToGVR(gvk)
        if mapErr == nil {
            list, err = m.storeProvider.Store().List(context.TODO(), resources.StoreKey{GVR: gvr, Namespace: namespace})
        } else {
            err = mapErr
        }
    } else {
        // Use generic dynamic listing through resources manager
        list, err = m.resourceManager.ListByGVK(gvk, namespace)
    }
	if err != nil {
		return err
	}
	for i := range list.Items {
		item := list.Items[i]
		resourceNode := NewNode(NodeTypeResource, item.GetName(), item.GetName())
		resourceNode.GVK = gvk
		parentNode.AddChild(resourceNode)
	}
	return nil
}

// createObjectList creates an object list for a given GVK
// createObjectList removed in favor of generic dynamic client listing

// GetState returns the current navigation state
func (m *Manager) GetState() *NavigationState {
	return m.state
}

// NavigateTo navigates to a specific node
func (m *Manager) NavigateTo(node *Node) error {
	m.state.SetCurrentNode(node)

	// Load resources if this is a namespace node
	if node.Type == NodeTypeNamespace {
		return m.LoadNamespaceResources(node.Name)
	}

	return nil
}

// GetCurrentNode returns the current navigation node
func (m *Manager) GetCurrentNode() *Node {
	return m.state.CurrentNode
}

// GetResourceManager returns the current resource manager
func (m *Manager) GetResourceManager() *resources.Manager {
	return m.resourceManager
}
