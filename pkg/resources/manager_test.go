package resources

import (
	"testing"

	"github.com/sttts/kc/pkg/handlers"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestNewManager(t *testing.T) {
	// Create fake config
	config := &rest.Config{
		Host: "https://fake-host:6443",
	}

	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}

	if manager.Client() == nil {
		t.Error("Client is nil")
	}

	if manager.Cache() == nil {
		t.Error("Cache is nil")
	}

	if manager.Cluster() == nil {
		t.Error("Cluster is nil")
	}
}

func TestRegisterHandler(t *testing.T) {
	// Create fake config
	config := &rest.Config{
		Host: "https://fake-host:6443",
	}

	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	// Create a mock handler
	handler := &mockHandler{}

	// Register the handler
	gvk := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Pod",
	}

	manager.RegisterHandler(gvk, handler)

	// Test getting the handler
	retrievedHandler, err := manager.GetHandler(gvk)
	if err != nil {
		t.Fatalf("GetHandler() failed: %v", err)
	}

	if retrievedHandler != handler {
		t.Error("Handler not registered correctly")
	}
}

func TestGetHandler(t *testing.T) {
	// Create fake config
	config := &rest.Config{
		Host: "https://fake-host:6443",
	}

	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	// Test getting non-existent handler
	nonExistentGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}

	_, err = manager.GetHandler(nonExistentGVK)
	if err == nil {
		t.Error("Expected error for non-existent handler")
	}
}

func TestGetSupportedResources(t *testing.T) {
	// Create fake config
	config := &rest.Config{
		Host: "https://fake-host:6443",
	}

	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	// This test will fail in a fake environment since discovery client needs a real cluster
	// In a real test environment, you'd mock the discovery client
	_, err = manager.GetSupportedResources()
	if err == nil {
		t.Error("Expected error when using discovery client with fake config")
	}
}

func TestStop(t *testing.T) {
	// Create fake config
	config := &rest.Config{
		Host: "https://fake-host:6443",
	}

	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	// Stop should not panic
	manager.Stop()
}

func TestGVKToGVR_WithFakeConfigErrors(t *testing.T) {
	config := &rest.Config{Host: "https://fake-host:6443"}
	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	_, err = mgr.GVKToGVR(gvk)
	if err == nil {
		t.Error("expected error from GVKToGVR with fake config")
	}
}

func TestListByGVK_WithFakeConfigErrors(t *testing.T) {
	config := &rest.Config{Host: "https://fake-host:6443"}
	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
	_, err = mgr.ListByGVK(gvk, "")
	if err == nil {
		t.Error("expected error from ListByGVK with fake config")
	}
}

func TestListNamespaces_WithFakeConfigErrors(t *testing.T) {
	config := &rest.Config{Host: "https://fake-host:6443"}
	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}
	_, err = mgr.ListNamespaces()
	if err == nil {
		t.Error("expected error from ListNamespaces with fake config")
	}
}

// mockHandler is a mock implementation of ResourceHandler for testing
type mockHandler struct{}

func (m *mockHandler) GetActions(obj client.Object) []handlers.Action {
	return []handlers.Action{
		{
			Name:                 "Test Action",
			Description:          "A test action",
			Command:              "test",
			Args:                 []string{"arg1", "arg2"},
			RequiresConfirmation: false,
		},
	}
}

func (m *mockHandler) GetSubResources(obj client.Object) []handlers.SubResource {
	return []handlers.SubResource{
		{
			Name:        "test-subresource",
			Description: "A test sub-resource",
			Handler:     &mockSubResourceHandler{},
		},
	}
}

func (m *mockHandler) GetDisplayColumns() []handlers.DisplayColumn {
	return []handlers.DisplayColumn{
		{
			Name:     "Name",
			Width:    20,
			Getter:   func(obj client.Object) string { return obj.GetName() },
			Sortable: true,
		},
	}
}

func (m *mockHandler) GetStatus(obj client.Object) string {
	return "Test Status"
}

// mockSubResourceHandler is a mock implementation of SubResourceHandler for testing
type mockSubResourceHandler struct{}

func (m *mockSubResourceHandler) Execute(obj client.Object) error {
	return nil
}

func (m *mockSubResourceHandler) GetContent(obj client.Object) (string, error) {
	return "Test content", nil
}
