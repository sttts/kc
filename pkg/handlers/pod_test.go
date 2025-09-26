package handlers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestPodHandler_GetActions(t *testing.T) {
	handler := NewPodHandler()

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	actions := handler.GetActions(pod)

	// Should have generic actions (Delete, Edit, Describe, View) + pod-specific actions (Logs, Exec)
	if len(actions) < 4 {
		t.Errorf("Expected at least 4 actions, got %d", len(actions))
	}

	// Check that we have generic actions
	actionNames := make(map[string]bool)
	for _, action := range actions {
		actionNames[action.Name] = true
	}

	expectedGeneric := []string{"Delete", "Edit", "Describe", "View"}
	for _, name := range expectedGeneric {
		if !actionNames[name] {
			t.Errorf("Missing generic action: %s", name)
		}
	}

	// Check that we have pod-specific actions
	expectedPod := []string{"Logs", "Exec"}
	for _, name := range expectedPod {
		if !actionNames[name] {
			t.Errorf("Missing pod-specific action: %s", name)
		}
	}
}

func TestPodHandler_GetDisplayColumns(t *testing.T) {
	handler := NewPodHandler()
	columns := handler.GetDisplayColumns()

	// Should have generic columns (Name, Namespace, Age) + pod-specific columns (Status, Ready, Restarts)
	if len(columns) < 3 {
		t.Errorf("Expected at least 3 columns, got %d", len(columns))
	}

	// Check column names
	columnNames := make(map[string]bool)
	for _, column := range columns {
		columnNames[column.Name] = true
	}

	expectedGeneric := []string{"Name", "Namespace", "Age"}
	for _, name := range expectedGeneric {
		if !columnNames[name] {
			t.Errorf("Missing generic column: %s", name)
		}
	}

	expectedPod := []string{"Status", "Ready", "Restarts"}
	for _, name := range expectedPod {
		if !columnNames[name] {
			t.Errorf("Missing pod-specific column: %s", name)
		}
	}
}

func TestPodHandler_GetStatus(t *testing.T) {
	handler := NewPodHandler()

	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected string
	}{
		{
			name: "Pending pod",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
			expected: "Pending",
		},
		{
			name: "Running pod",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			expected: "Running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.GetStatus(tt.pod)
			if result != tt.expected {
				t.Errorf("GetStatus() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	gvk := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Pod",
	}

	handler := NewPodHandler()

	// Test registration
	registry.Register(gvk, handler)

	if !registry.Has(gvk) {
		t.Error("Handler should be registered")
	}

	// Test retrieval
	retrievedHandler, err := registry.Get(gvk)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if retrievedHandler != handler {
		t.Errorf("Retrieved handler = %v, want %v", retrievedHandler, handler)
	}

	// Test listing
	handlers := registry.List()
	if len(handlers) != 1 {
		t.Errorf("Expected 1 handler, got %d", len(handlers))
	}

	if handlers[gvk] != handler {
		t.Errorf("Listed handler = %v, want %v", handlers[gvk], handler)
	}
}

func TestGlobalRegistry(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Pod",
	}

	handler := NewPodHandler()

	// Test global registration
	Register(gvk, handler)

	if !Has(gvk) {
		t.Error("Handler should be registered in global registry")
	}

	// Test global retrieval
	retrievedHandler, err := Get(gvk)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if retrievedHandler != handler {
		t.Errorf("Retrieved handler = %v, want %v", retrievedHandler, handler)
	}
}
