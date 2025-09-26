package ui

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestNewResourceSelector(t *testing.T) {
	allResources := []schema.GroupVersionKind{
		{Group: "", Version: "v1", Kind: "Pod"},
		{Group: "", Version: "v1", Kind: "Service"},
		{Group: "apps", Version: "v1", Kind: "Deployment"},
	}

	selector := NewResourceSelector(allResources)

	if selector == nil {
		t.Fatal("NewResourceSelector returned nil")
	}

	if len(selector.presets) == 0 {
		t.Error("Expected presets to be initialized")
	}

	if len(selector.allResources) != 3 {
		t.Errorf("Expected 3 all resources, got %d", len(selector.allResources))
	}

	if selector.selected != 0 {
		t.Errorf("Expected selected to be 0, got %d", selector.selected)
	}

	if selector.showCustom {
		t.Error("Expected showCustom to be false initially")
	}
}

func TestResourceSelectorMoveUp(t *testing.T) {
	selector := NewResourceSelector([]schema.GroupVersionKind{})

	// Move up when at top should not change selection
	selector.moveUp()
	if selector.selected != 0 {
		t.Errorf("Expected selected to remain 0, got %d", selector.selected)
	}

	// Move down then up
	selector.moveDown()
	selector.moveUp()
	if selector.selected != 0 {
		t.Errorf("Expected selected to be 0 after up, got %d", selector.selected)
	}
}

func TestResourceSelectorMoveDown(t *testing.T) {
	selector := NewResourceSelector([]schema.GroupVersionKind{})

	// Move down
	selector.moveDown()
	if selector.selected != 1 {
		t.Errorf("Expected selected to be 1, got %d", selector.selected)
	}

	// Move down again
	selector.moveDown()
	if selector.selected != 2 {
		t.Errorf("Expected selected to be 2, got %d", selector.selected)
	}
}

func TestResourceSelectorToggleView(t *testing.T) {
	selector := NewResourceSelector([]schema.GroupVersionKind{})

	// Initially showing presets
	if selector.showCustom {
		t.Error("Expected showCustom to be false initially")
	}

	// Toggle to custom
	selector.toggleView()
	if !selector.showCustom {
		t.Error("Expected showCustom to be true after toggle")
	}

	// Toggle back to presets
	selector.toggleView()
	if selector.showCustom {
		t.Error("Expected showCustom to be false after second toggle")
	}
}

func TestGetDefaultPresets(t *testing.T) {
	presets := getDefaultPresets()

	if len(presets) == 0 {
		t.Error("Expected default presets to be non-empty")
	}

	// Check for expected presets
	expectedPresets := []string{"Core", "Apps", "Networking", "Storage", "RBAC", "Monitoring"}
	for _, expected := range expectedPresets {
		found := false
		for _, preset := range presets {
			if preset.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected preset %s not found", expected)
		}
	}

	// Check that presets have resources
	for _, preset := range presets {
		if len(preset.Resources) == 0 {
			t.Errorf("Preset %s has no resources", preset.Name)
		}
	}
}

func TestGetSelectedResources(t *testing.T) {
	allResources := []schema.GroupVersionKind{
		{Group: "", Version: "v1", Kind: "Pod"},
		{Group: "", Version: "v1", Kind: "Service"},
		{Group: "apps", Version: "v1", Kind: "Deployment"},
	}

	selector := NewResourceSelector(allResources)

	// Test selecting a preset
	selector.selected = 0 // Core preset
	resources := selector.GetSelectedResources()
	if len(resources) == 0 {
		t.Error("Expected selected resources to be non-empty")
	}

	// Test selecting "All Resources"
	selector.selected = len(selector.presets) // "All Resources" option
	resources = selector.GetSelectedResources()
	if len(resources) != len(allResources) {
		t.Errorf("Expected %d resources for 'All Resources', got %d", len(allResources), len(resources))
	}
}

func TestResourceSelectorSetDimensions(t *testing.T) {
	selector := NewResourceSelector([]schema.GroupVersionKind{})

	selector.SetDimensions(100, 50)

	if selector.width != 100 {
		t.Errorf("Expected width to be 100, got %d", selector.width)
	}

	if selector.height != 50 {
		t.Errorf("Expected height to be 50, got %d", selector.height)
	}
}
