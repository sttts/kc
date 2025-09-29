package navigation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sttts/kc/pkg/kubeconfig"
)

func writeKubeconfigFile(t *testing.T, dir, name string, kubeconfigYAML string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(kubeconfigYAML), 0644); err != nil {
		t.Fatalf("failed to write kubeconfig %s: %v", name, err)
	}
	return path
}

func TestBuildHierarchy_WithDirectories(t *testing.T) {
	// Prepare temp HOME with .kube and two kubeconfig files
	tempHome := t.TempDir()
	kubeDir := filepath.Join(tempHome, ".kube")
	if err := os.MkdirAll(kubeDir, 0755); err != nil {
		t.Fatalf("failed to create .kube: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tempHome)

	mainConfig := `
apiVersion: v1
clusters:
- cluster:
    server: https://cluster-a
  name: cluster-a
contexts:
- context:
    cluster: cluster-a
    user: user-a
  name: context-a
current-context: context-a
users:
- name: user-a
  user:
    token: a
`
	otherConfig := `
apiVersion: v1
clusters:
- cluster:
    server: https://cluster-b
  name: cluster-b
contexts:
- context:
    cluster: cluster-b
    user: user-b
  name: context-b
current-context: context-b
users:
- name: user-b
  user:
    token: b
`

	writeKubeconfigFile(t, kubeDir, "config", mainConfig)
	writeKubeconfigFile(t, kubeDir, "other", otherConfig)

	kubeMgr := kubeconfig.NewManager()
	if err := kubeMgr.DiscoverKubeconfigs(); err != nil {
		t.Fatalf("DiscoverKubeconfigs() error: %v", err)
	}

	navMgr := NewManager(kubeMgr, nil)
	if err := navMgr.BuildHierarchy(); err != nil {
		t.Fatalf("BuildHierarchy() error: %v", err)
	}

	root := navMgr.GetState().Root
	if root == nil {
		t.Fatal("root is nil")
	}

	// Expect exactly three top-level entries: current kubeconfig, Contexts dir, Kubeconfigs dir
	if len(root.Children) != 3 {
		t.Fatalf("expected 3 root children, got %d", len(root.Children))
	}

	var currentNode, contextsDir, kubeconfigsDir *Node
	for _, c := range root.Children {
		switch c.Type {
		case NodeTypeKubeconfig:
			currentNode = c
		case NodeTypeDirectory:
			if c.Name == "Contexts" {
				contextsDir = c
			}
			if c.Name == "Kubeconfigs" {
				kubeconfigsDir = c
			}
		}
	}

	if currentNode == nil {
		t.Error("current kubeconfig node not found among root children")
	}
	if contextsDir == nil {
		t.Error("Contexts directory not found among root children")
	}
	if kubeconfigsDir == nil {
		t.Error("Kubeconfigs directory not found among root children")
	}

	// Contexts directory should include at least the two contexts
	if contextsDir != nil {
		got := make(map[string]bool)
		for _, c := range contextsDir.Children {
			got[c.Name] = true
		}
		if !got["context-a"] || !got["context-b"] {
			t.Errorf("contextsDir missing expected contexts, got: %v", got)
		}
	}

	// Kubeconfigs directory should include at least the two files
	if kubeconfigsDir != nil {
		if len(kubeconfigsDir.Children) < 2 {
			t.Errorf("expected >=2 kubeconfigs, got %d", len(kubeconfigsDir.Children))
		}
	}
}

// Note: End-to-end grouping tests would require a live cluster or extensive fakes.
// Here we only ensure that calling LoadNamespaceResources on an empty manager
// does not panic and creates no nodes when resourceManager is nil.
// Legacy resource-loading methods were removed; hierarchy manager now only
// builds kubeconfig/context directories. No resource-manager tests remain here.
