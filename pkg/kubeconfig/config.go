package kubeconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Kubeconfig represents a kubeconfig file
type Kubeconfig struct {
	Path    string
	Config  *api.Config
	Context string
}

// Context represents a Kubernetes context
type Context struct {
	Name       string
	Cluster    string
	Namespace  string
	User       string
	Kubeconfig *Kubeconfig
}

// Cluster represents a Kubernetes cluster
type Cluster struct {
	Name    string
	Server  string
	Context *Context
}

// Manager handles kubeconfig discovery and management
type Manager struct {
	kubeconfigs []*Kubeconfig
	contexts    []*Context
	clusters    []*Cluster
}

// NewManager creates a new kubeconfig manager
func NewManager() *Manager {
	return &Manager{
		kubeconfigs: make([]*Kubeconfig, 0),
		contexts:    make([]*Context, 0),
		clusters:    make([]*Cluster, 0),
	}
}

// DiscoverKubeconfigs discovers all kubeconfig files in ~/.kube
func (m *Manager) DiscoverKubeconfigs() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	kubeDir := filepath.Join(homeDir, ".kube")

	// Check if .kube directory exists
	if _, err := os.Stat(kubeDir); os.IsNotExist(err) {
		return fmt.Errorf("kube directory does not exist: %s", kubeDir)
	}

	// Load the main kubeconfig
	mainConfigPath := filepath.Join(kubeDir, "config")
	if _, err := os.Stat(mainConfigPath); err == nil {
		config, err := clientcmd.LoadFromFile(mainConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load main kubeconfig: %w", err)
		}

		kubeconfig := &Kubeconfig{
			Path:   mainConfigPath,
			Config: config,
		}
		m.kubeconfigs = append(m.kubeconfigs, kubeconfig)
	}

	// Discover additional kubeconfig files
	err = filepath.Walk(kubeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and hidden files
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Skip the main config file (already loaded)
		if path == mainConfigPath {
			return nil
		}

		// Try to load as kubeconfig
		config, err := clientcmd.LoadFromFile(path)
		if err != nil {
			// Not a valid kubeconfig, skip
			return nil
		}

		kubeconfig := &Kubeconfig{
			Path:   path,
			Config: config,
		}
		m.kubeconfigs = append(m.kubeconfigs, kubeconfig)

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk kube directory: %w", err)
	}

	return m.buildContextsAndClusters()
}

// buildContextsAndClusters builds the context and cluster lists from kubeconfigs
func (m *Manager) buildContextsAndClusters() error {
	m.contexts = make([]*Context, 0)
	m.clusters = make([]*Cluster, 0)

	for _, kubeconfig := range m.kubeconfigs {
		// Build contexts
		for contextName, context := range kubeconfig.Config.Contexts {
			namespace := context.Namespace
			if namespace == "" {
				namespace = "default"
			}

			ctx := &Context{
				Name:       contextName,
				Cluster:    context.Cluster,
				Namespace:  namespace,
				User:       context.AuthInfo,
				Kubeconfig: kubeconfig,
			}
			m.contexts = append(m.contexts, ctx)
		}

		// Build clusters
		for clusterName, cluster := range kubeconfig.Config.Clusters {
			clusterObj := &Cluster{
				Name:   clusterName,
				Server: cluster.Server,
			}
			m.clusters = append(m.clusters, clusterObj)
		}
	}

	return nil
}

// GetKubeconfigs returns all discovered kubeconfigs
func (m *Manager) GetKubeconfigs() []*Kubeconfig {
	return m.kubeconfigs
}

// GetContexts returns all discovered contexts
func (m *Manager) GetContexts() []*Context {
	return m.contexts
}

// GetClusters returns all discovered clusters
func (m *Manager) GetClusters() []*Cluster {
	return m.clusters
}

// GetContextByName finds a context by name
func (m *Manager) GetContextByName(name string) *Context {
	for _, ctx := range m.contexts {
		if ctx.Name == name {
			return ctx
		}
	}
	return nil
}

// GetKubeconfigByPath finds a kubeconfig by path
func (m *Manager) GetKubeconfigByPath(path string) *Kubeconfig {
	for _, kc := range m.kubeconfigs {
		if kc.Path == path {
			return kc
		}
	}
	return nil
}

// SetCurrentContext sets the current context for a kubeconfig
func (m *Manager) SetCurrentContext(kubeconfig *Kubeconfig, contextName string) error {
	if kubeconfig.Config.CurrentContext == contextName {
		return nil // Already set
	}

	kubeconfig.Config.CurrentContext = contextName
	kubeconfig.Context = contextName

	// Write back to file
	return clientcmd.WriteToFile(*kubeconfig.Config, kubeconfig.Path)
}

// GetCurrentContext returns the current context for a kubeconfig
func (m *Manager) GetCurrentContext(kubeconfig *Kubeconfig) *Context {
	if kubeconfig.Context != "" {
		return m.GetContextByName(kubeconfig.Context)
	}

	if kubeconfig.Config.CurrentContext != "" {
		return m.GetContextByName(kubeconfig.Config.CurrentContext)
	}

	return nil
}

// GetContextsForKubeconfig returns all contexts for a specific kubeconfig
func (m *Manager) GetContextsForKubeconfig(kubeconfig *Kubeconfig) []*Context {
	var contexts []*Context
	for _, ctx := range m.contexts {
		if ctx.Kubeconfig == kubeconfig {
			contexts = append(contexts, ctx)
		}
	}
	return contexts
}

// GetNamespacesForContext returns the namespaces available for a context
func (m *Manager) GetNamespacesForContext(ctx *Context) ([]string, error) {
	// This would typically involve making an API call to list namespaces
	// For now, return a default list
	return []string{"default", "kube-system", "kube-public", "kube-node-lease"}, nil
}

// CreateClient creates a controller-runtime client for a context
func (m *Manager) CreateClient(ctx *Context) (client.Client, error) {
	// Create client config
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: ctx.Kubeconfig.Path},
		&clientcmd.ConfigOverrides{
			CurrentContext: ctx.Name,
		},
	).ClientConfig()

	if err != nil {
		return nil, fmt.Errorf("failed to create client config: %w", err)
	}

	// Create controller-runtime client
	c, err := client.New(config, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return c, nil
}

// CreateClientForKubeconfig creates a controller-runtime client for a kubeconfig
func (m *Manager) CreateClientForKubeconfig(kubeconfig *Kubeconfig) (client.Client, error) {
	// Create client config
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig.Path},
		&clientcmd.ConfigOverrides{},
	).ClientConfig()

	if err != nil {
		return nil, fmt.Errorf("failed to create client config: %w", err)
	}

	// Create controller-runtime client
	c, err := client.New(config, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return c, nil
}

// CreateClientConfig creates a REST config for a context
func (m *Manager) CreateClientConfig(ctx *Context) (*rest.Config, error) {
	// Create client config
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: ctx.Kubeconfig.Path},
		&clientcmd.ConfigOverrides{
			CurrentContext: ctx.Name,
		},
	).ClientConfig()

	if err != nil {
		return nil, fmt.Errorf("failed to create client config: %w", err)
	}

	return config, nil
}
