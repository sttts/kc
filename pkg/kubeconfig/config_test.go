package kubeconfig

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/tools/clientcmd/api"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()

	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}

	if len(manager.kubeconfigs) != 0 {
		t.Errorf("Expected empty kubeconfigs slice, got length %d", len(manager.kubeconfigs))
	}

	if len(manager.contexts) != 0 {
		t.Errorf("Expected empty contexts slice, got length %d", len(manager.contexts))
	}

	if len(manager.clusters) != 0 {
		t.Errorf("Expected empty clusters slice, got length %d", len(manager.clusters))
	}
}

func TestKubeconfig(t *testing.T) {
	config := &api.Config{
		CurrentContext: "test-context",
		Contexts: map[string]*api.Context{
			"test-context": {
				Cluster:   "test-cluster",
				AuthInfo:  "test-user",
				Namespace: "default",
			},
		},
		Clusters: map[string]*api.Cluster{
			"test-cluster": {
				Server: "https://test-server:6443",
			},
		},
		AuthInfos: map[string]*api.AuthInfo{
			"test-user": {
				Token: "test-token",
			},
		},
	}

	kubeconfig := &Kubeconfig{
		Path:    "/test/path/config",
		Config:  config,
		Context: "test-context",
	}

	if kubeconfig.Path != "/test/path/config" {
		t.Errorf("Path = %v, want %v", kubeconfig.Path, "/test/path/config")
	}

	if kubeconfig.Context != "test-context" {
		t.Errorf("Context = %v, want %v", kubeconfig.Context, "test-context")
	}

	if kubeconfig.Config.CurrentContext != "test-context" {
		t.Errorf("Config.CurrentContext = %v, want %v", kubeconfig.Config.CurrentContext, "test-context")
	}
}

func TestContext(t *testing.T) {
	kubeconfig := &Kubeconfig{
		Path: "/test/config",
	}

	context := &Context{
		Name:       "test-context",
		Cluster:    "test-cluster",
		Namespace:  "default",
		User:       "test-user",
		Kubeconfig: kubeconfig,
	}

	if context.Name != "test-context" {
		t.Errorf("Name = %v, want %v", context.Name, "test-context")
	}

	if context.Cluster != "test-cluster" {
		t.Errorf("Cluster = %v, want %v", context.Cluster, "test-cluster")
	}

	if context.Namespace != "default" {
		t.Errorf("Namespace = %v, want %v", context.Namespace, "default")
	}

	if context.User != "test-user" {
		t.Errorf("User = %v, want %v", context.User, "test-user")
	}

	if context.Kubeconfig != kubeconfig {
		t.Errorf("Kubeconfig reference is not the same")
	}
}

func TestCluster(t *testing.T) {
	context := &Context{
		Name: "test-context",
	}

	cluster := &Cluster{
		Name:    "test-cluster",
		Server:  "https://test-server:6443",
		Context: context,
	}

	if cluster.Name != "test-cluster" {
		t.Errorf("Name = %v, want %v", cluster.Name, "test-cluster")
	}

	if cluster.Server != "https://test-server:6443" {
		t.Errorf("Server = %v, want %v", cluster.Server, "https://test-server:6443")
	}

	if cluster.Context != context {
		t.Errorf("Context reference is not the same")
	}
}

func TestBuildContextsAndClusters(t *testing.T) {
	manager := NewManager()

	config1 := &api.Config{
		CurrentContext: "context1",
		Contexts: map[string]*api.Context{
			"context1": {
				Cluster:   "cluster1",
				AuthInfo:  "user1",
				Namespace: "default",
			},
			"context2": {
				Cluster:   "cluster1",
				AuthInfo:  "user2",
				Namespace: "kube-system",
			},
		},
		Clusters: map[string]*api.Cluster{
			"cluster1": {
				Server: "https://server1:6443",
			},
		},
	}

	config2 := &api.Config{
		CurrentContext: "context3",
		Contexts: map[string]*api.Context{
			"context3": {
				Cluster:   "cluster2",
				AuthInfo:  "user3",
				Namespace: "default",
			},
		},
		Clusters: map[string]*api.Cluster{
			"cluster2": {
				Server: "https://server2:6443",
			},
		},
	}

	kubeconfig1 := &Kubeconfig{
		Path:   "/config1",
		Config: config1,
	}

	kubeconfig2 := &Kubeconfig{
		Path:   "/config2",
		Config: config2,
	}

	manager.kubeconfigs = []*Kubeconfig{kubeconfig1, kubeconfig2}

	err := manager.buildContextsAndClusters()
	if err != nil {
		t.Fatalf("buildContextsAndClusters() failed: %v", err)
	}

	// Check contexts
	if len(manager.contexts) != 3 {
		t.Errorf("Expected 3 contexts, got %d", len(manager.contexts))
	}

	// Check clusters
	if len(manager.clusters) != 2 {
		t.Errorf("Expected 2 clusters, got %d", len(manager.clusters))
	}

	// Verify context details
	context1 := manager.GetContextByName("context1")
	if context1 == nil {
		t.Fatal("context1 not found")
	}
	if context1.Namespace != "default" {
		t.Errorf("context1.Namespace = %v, want %v", context1.Namespace, "default")
	}
	if context1.Kubeconfig != kubeconfig1 {
		t.Errorf("context1.Kubeconfig reference is not correct")
	}

	context2 := manager.GetContextByName("context2")
	if context2 == nil {
		t.Fatal("context2 not found")
	}
	if context2.Namespace != "kube-system" {
		t.Errorf("context2.Namespace = %v, want %v", context2.Namespace, "kube-system")
	}
}

func TestGetContextByName(t *testing.T) {
	manager := NewManager()

	context1 := &Context{Name: "context1"}
	context2 := &Context{Name: "context2"}

	manager.contexts = []*Context{context1, context2}

	// Test existing context
	result := manager.GetContextByName("context1")
	if result != context1 {
		t.Errorf("GetContextByName(\"context1\") = %v, want %v", result, context1)
	}

	// Test non-existing context
	result = manager.GetContextByName("non-existing")
	if result != nil {
		t.Errorf("GetContextByName(\"non-existing\") = %v, want nil", result)
	}
}

func TestGetKubeconfigByPath(t *testing.T) {
	manager := NewManager()

	kubeconfig1 := &Kubeconfig{Path: "/config1"}
	kubeconfig2 := &Kubeconfig{Path: "/config2"}

	manager.kubeconfigs = []*Kubeconfig{kubeconfig1, kubeconfig2}

	// Test existing kubeconfig
	result := manager.GetKubeconfigByPath("/config1")
	if result != kubeconfig1 {
		t.Errorf("GetKubeconfigByPath(\"/config1\") = %v, want %v", result, kubeconfig1)
	}

	// Test non-existing kubeconfig
	result = manager.GetKubeconfigByPath("/non-existing")
	if result != nil {
		t.Errorf("GetKubeconfigByPath(\"/non-existing\") = %v, want nil", result)
	}
}

func TestGetCurrentContext(t *testing.T) {
	manager := NewManager()

	config := &api.Config{
		CurrentContext: "default-context",
		Contexts: map[string]*api.Context{
			"default-context": {
				Cluster:   "cluster1",
				AuthInfo:  "user1",
				Namespace: "default",
			},
		},
	}

	kubeconfig := &Kubeconfig{
		Path:    "/config",
		Config:  config,
		Context: "explicit-context",
	}

	context := &Context{
		Name:       "explicit-context",
		Cluster:    "cluster1",
		Namespace:  "default",
		User:       "user1",
		Kubeconfig: kubeconfig,
	}

	manager.contexts = []*Context{context}

	// Test with explicit context
	result := manager.GetCurrentContext(kubeconfig)
	if result != context {
		t.Errorf("GetCurrentContext() = %v, want %v", result, context)
	}

	// Test with config current context
	kubeconfig.Context = ""
	result = manager.GetCurrentContext(kubeconfig)
	if result != nil {
		t.Errorf("GetCurrentContext() with no explicit context = %v, want nil", result)
	}
}

func TestGetContextsForKubeconfig(t *testing.T) {
	manager := NewManager()

	kubeconfig1 := &Kubeconfig{Path: "/config1"}
	kubeconfig2 := &Kubeconfig{Path: "/config2"}

	context1 := &Context{Name: "context1", Kubeconfig: kubeconfig1}
	context2 := &Context{Name: "context2", Kubeconfig: kubeconfig1}
	context3 := &Context{Name: "context3", Kubeconfig: kubeconfig2}

	manager.contexts = []*Context{context1, context2, context3}

	// Test getting contexts for kubeconfig1
	result := manager.GetContextsForKubeconfig(kubeconfig1)
	if len(result) != 2 {
		t.Errorf("Expected 2 contexts for kubeconfig1, got %d", len(result))
	}

	// Test getting contexts for kubeconfig2
	result = manager.GetContextsForKubeconfig(kubeconfig2)
	if len(result) != 1 {
		t.Errorf("Expected 1 context for kubeconfig2, got %d", len(result))
	}
	if result[0] != context3 {
		t.Errorf("Expected context3, got %v", result[0])
	}
}

func TestGetNamespacesForContext(t *testing.T) {
	manager := NewManager()
	context := &Context{Name: "test-context"}

	namespaces, err := manager.GetNamespacesForContext(context)
	if err != nil {
		t.Fatalf("GetNamespacesForContext() failed: %v", err)
	}

	expected := []string{"default", "kube-system", "kube-public", "kube-node-lease"}
	if len(namespaces) != len(expected) {
		t.Errorf("Expected %d namespaces, got %d", len(expected), len(namespaces))
	}

	for i, ns := range expected {
		if namespaces[i] != ns {
			t.Errorf("namespaces[%d] = %v, want %v", i, namespaces[i], ns)
		}
	}
}

func TestDiscoverKubeconfigs_NoKubeDir(t *testing.T) {
	// Create a temporary directory without .kube
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	defer func() {
		os.Setenv("HOME", originalHome)
	}()

	os.Setenv("HOME", tempDir)

	manager := NewManager()
	err := manager.DiscoverKubeconfigs()

	if err == nil {
		t.Error("Expected error when .kube directory doesn't exist")
	}
}

func TestDiscoverKubeconfigs_EmptyKubeDir(t *testing.T) {
	// Create a temporary directory with empty .kube
	tempDir := t.TempDir()
	kubeDir := filepath.Join(tempDir, ".kube")
	err := os.MkdirAll(kubeDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create .kube directory: %v", err)
	}

	originalHome := os.Getenv("HOME")
	defer func() {
		os.Setenv("HOME", originalHome)
	}()

	os.Setenv("HOME", tempDir)

	manager := NewManager()
	err = manager.DiscoverKubeconfigs()

	if err != nil {
		t.Errorf("DiscoverKubeconfigs() failed: %v", err)
	}

	if len(manager.kubeconfigs) != 0 {
		t.Errorf("Expected 0 kubeconfigs, got %d", len(manager.kubeconfigs))
	}
}
