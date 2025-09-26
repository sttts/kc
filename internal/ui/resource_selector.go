package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourceSelector represents the F2 resource selection dialog
type ResourceSelector struct {
	presets      []ResourcePreset
	customSets   []ResourceSet
	allResources []schema.GroupVersionKind
	selected     int
	scrollTop    int
	width        int
	height       int
	showCustom   bool
}

// ResourcePreset represents a predefined resource set
type ResourcePreset struct {
	Name        string
	Description string
	Resources   []schema.GroupVersionKind
}

// ResourceSet represents a custom resource set
type ResourceSet struct {
	Name        string
	Description string
	Resources   []schema.GroupVersionKind
}

// NewResourceSelector creates a new resource selector
func NewResourceSelector(allResources []schema.GroupVersionKind) *ResourceSelector {
	return &ResourceSelector{
		presets:      getDefaultPresets(),
		customSets:   make([]ResourceSet, 0),
		allResources: allResources,
		selected:     0,
		scrollTop:    0,
		showCustom:   false,
	}
}

// Init initializes the resource selector
func (rs *ResourceSelector) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the resource selector state
func (rs *ResourceSelector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			rs.moveUp()
		case "down", "j":
			rs.moveDown()
		case "enter":
			return rs, rs.selectItem()
		case "tab":
			rs.toggleView()
		case "ctrl+a":
			return rs, rs.createCustomSet()
		case "ctrl+d":
			return rs, rs.deleteCustomSet()
		case "ctrl+e":
			return rs, rs.editCustomSet()
		case "esc":
			return rs, tea.Quit
		}
	}

	return rs, nil
}

// View renders the resource selector
func (rs *ResourceSelector) View() string {
	// Create header
	header := rs.renderHeader()

	// Create content
	content := rs.renderContent()

	// Create footer
	footer := rs.renderFooter()

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
		footer,
	)
}

// SetDimensions sets the selector dimensions
func (rs *ResourceSelector) SetDimensions(width, height int) {
	rs.width = width
	rs.height = height
}

// renderHeader renders the header
func (rs *ResourceSelector) renderHeader() string {
	title := "Resource Selection"
	if rs.showCustom {
		title = "Custom Resource Sets"
	}

	return ResourceSelectorHeaderStyle.
		Width(rs.width).
		Height(1).
		Align(lipgloss.Center).
		Render(title)
}

// renderContent renders the main content
func (rs *ResourceSelector) renderContent() string {
	var items []string

	if rs.showCustom {
		// Show custom sets
		for i, set := range rs.customSets {
			item := fmt.Sprintf("  %s - %s (%d resources)", set.Name, set.Description, len(set.Resources))
			if i == rs.selected {
				item = "> " + item
			}
			items = append(items, item)
		}
	} else {
		// Show presets
		for i, preset := range rs.presets {
			item := fmt.Sprintf("  %s - %s (%d resources)", preset.Name, preset.Description, len(preset.Resources))
			if i == rs.selected {
				item = "> " + item
			}
			items = append(items, item)
		}
	}

	// Add "All Resources" option
	allItem := fmt.Sprintf("  All Resources (%d resources)", len(rs.allResources))
	if rs.selected == len(items) {
		allItem = "> " + allItem
	}
	items = append(items, allItem)

	// Render items with scrolling
	contentHeight := rs.height - 3 // -3 for header and footer
	start := rs.scrollTop
	end := start + contentHeight
	if end > len(items) {
		end = len(items)
	}

	var visibleItems []string
	for i := start; i < end; i++ {
		if i < len(items) {
			visibleItems = append(visibleItems, items[i])
		}
	}

	// Fill remaining space
	for len(visibleItems) < contentHeight {
		visibleItems = append(visibleItems, "")
	}

	return lipgloss.JoinVertical(lipgloss.Left, visibleItems...)
}

// renderFooter renders the footer with help
func (rs *ResourceSelector) renderFooter() string {
	help := "Tab: Switch view | Enter: Select | Ctrl+A: Add custom | Ctrl+D: Delete | Ctrl+E: Edit | Esc: Cancel"
	return ResourceSelectorFooterStyle.
		Width(rs.width).
		Height(1).
		Align(lipgloss.Center).
		Render(help)
}

// moveUp moves selection up
func (rs *ResourceSelector) moveUp() {
	if rs.selected > 0 {
		rs.selected--
		// Adjust scroll if needed
		if rs.selected < rs.scrollTop {
			rs.scrollTop = rs.selected
		}
	}
}

// moveDown moves selection down
func (rs *ResourceSelector) moveDown() {
	maxItems := len(rs.presets) + 1 // +1 for "All Resources"
	if rs.showCustom {
		maxItems = len(rs.customSets) + 1
	}

	if rs.selected < maxItems-1 {
		rs.selected++
		// Adjust scroll if needed
		contentHeight := rs.height - 3
		if rs.selected >= rs.scrollTop+contentHeight {
			rs.scrollTop = rs.selected - contentHeight + 1
		}
	}
}

// toggleView toggles between presets and custom sets
func (rs *ResourceSelector) toggleView() {
	rs.showCustom = !rs.showCustom
	rs.selected = 0
	rs.scrollTop = 0
}

// selectItem selects the current item
func (rs *ResourceSelector) selectItem() tea.Cmd {
	// TODO: Return the selected resource set
	return tea.Quit
}

// createCustomSet creates a new custom resource set
func (rs *ResourceSelector) createCustomSet() tea.Cmd {
	// TODO: Implement custom set creation dialog
	return nil
}

// deleteCustomSet deletes the selected custom set
func (rs *ResourceSelector) deleteCustomSet() tea.Cmd {
	if rs.showCustom && rs.selected < len(rs.customSets) {
		// TODO: Implement deletion with confirmation
		rs.customSets = append(rs.customSets[:rs.selected], rs.customSets[rs.selected+1:]...)
		if rs.selected >= len(rs.customSets) {
			rs.selected = max(0, len(rs.customSets)-1)
		}
	}
	return nil
}

// editCustomSet edits the selected custom set
func (rs *ResourceSelector) editCustomSet() tea.Cmd {
	// TODO: Implement custom set editing dialog
	return nil
}

// getDefaultPresets returns the default resource presets
func getDefaultPresets() []ResourcePreset {
	return []ResourcePreset{
		{
			Name:        "Core",
			Description: "Core Kubernetes resources",
			Resources: []schema.GroupVersionKind{
				{Group: "", Version: "v1", Kind: "Pod"},
				{Group: "", Version: "v1", Kind: "Service"},
				{Group: "", Version: "v1", Kind: "ConfigMap"},
				{Group: "", Version: "v1", Kind: "Secret"},
				{Group: "", Version: "v1", Kind: "Namespace"},
				{Group: "", Version: "v1", Kind: "Node"},
			},
		},
		{
			Name:        "Apps",
			Description: "Application workload resources",
			Resources: []schema.GroupVersionKind{
				{Group: "apps", Version: "v1", Kind: "Deployment"},
				{Group: "apps", Version: "v1", Kind: "ReplicaSet"},
				{Group: "apps", Version: "v1", Kind: "StatefulSet"},
				{Group: "apps", Version: "v1", Kind: "DaemonSet"},
				{Group: "apps", Version: "v1", Kind: "Job"},
				{Group: "apps", Version: "v1", Kind: "CronJob"},
			},
		},
		{
			Name:        "Networking",
			Description: "Networking resources",
			Resources: []schema.GroupVersionKind{
				{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
				{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"},
				{Group: "", Version: "v1", Kind: "Service"},
				{Group: "", Version: "v1", Kind: "Endpoints"},
			},
		},
		{
			Name:        "Storage",
			Description: "Storage resources",
			Resources: []schema.GroupVersionKind{
				{Group: "", Version: "v1", Kind: "PersistentVolume"},
				{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},
				{Group: "storage.k8s.io", Version: "v1", Kind: "StorageClass"},
			},
		},
		{
			Name:        "RBAC",
			Description: "RBAC resources",
			Resources: []schema.GroupVersionKind{
				{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
				{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
				{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
				{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding"},
			},
		},
		{
			Name:        "Monitoring",
			Description: "Monitoring and observability resources",
			Resources: []schema.GroupVersionKind{
				{Group: "metrics.k8s.io", Version: "v1beta1", Kind: "NodeMetrics"},
				{Group: "metrics.k8s.io", Version: "v1beta1", Kind: "PodMetrics"},
			},
		},
	}
}

// GetSelectedResources returns the resources from the selected preset/set
func (rs *ResourceSelector) GetSelectedResources() []schema.GroupVersionKind {
	if rs.showCustom {
		if rs.selected < len(rs.customSets) {
			return rs.customSets[rs.selected].Resources
		}
	} else {
		if rs.selected < len(rs.presets) {
			return rs.presets[rs.selected].Resources
		}
	}

	// Return all resources if "All Resources" is selected
	return rs.allResources
}
