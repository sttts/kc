package handlers

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Action represents an action that can be performed on a resource
type Action struct {
	Name                 string
	Description          string
	Command              string
	Args                 []string
	RequiresConfirmation bool
}

// SubResource represents a sub-resource like logs, exec, etc.
type SubResource struct {
	Name        string
	Description string
	Handler     SubResourceHandler
}

// SubResourceHandler handles sub-resource operations
type SubResourceHandler interface {
	// Execute performs the sub-resource operation
	Execute(obj client.Object) error

	// GetContent returns the content for viewing
	GetContent(obj client.Object) (string, error)
}

// DisplayColumn defines how to display a column in the resource list
type DisplayColumn struct {
	Name     string
	Width    int
	Getter   func(obj client.Object) string
	Sortable bool
}

// ResourceHandler defines the interface for resource-specific operations
type ResourceHandler interface {
	// GetActions returns the list of actions available for this resource type
	GetActions(obj client.Object) []Action

	// GetSubResources returns sub-resources available for this resource
	GetSubResources(obj client.Object) []SubResource

	// GetDisplayColumns returns the columns to display for this resource type
	GetDisplayColumns() []DisplayColumn

	// GetStatus returns the status string for a resource
	GetStatus(obj client.Object) string
}

// BaseHandler provides generic operations that all Kubernetes resources support
type BaseHandler struct{}

// NewBaseHandler creates a new base handler
func NewBaseHandler() *BaseHandler {
	return &BaseHandler{}
}

// GetGenericActions returns the list of generic actions available for all resources
func (h *BaseHandler) GetGenericActions(obj client.Object, gvk schema.GroupVersionKind) []Action {
	actions := []Action{
		{
			Name:                 "Delete",
			Description:          "Delete the resource",
			Command:              "kubectl",
			Args:                 h.buildDeleteArgs(obj, gvk),
			RequiresConfirmation: true,
		},
		{
			Name:                 "Edit",
			Description:          "Edit the resource",
			Command:              "kubectl",
			Args:                 h.buildEditArgs(obj, gvk),
			RequiresConfirmation: false,
		},
		{
			Name:                 "Describe",
			Description:          "Describe the resource",
			Command:              "kubectl",
			Args:                 h.buildDescribeArgs(obj, gvk),
			RequiresConfirmation: false,
		},
		{
			Name:                 "View",
			Description:          "View the resource (F3)",
			Command:              "kubectl",
			Args:                 h.buildViewArgs(obj, gvk),
			RequiresConfirmation: false,
		},
	}

	return actions
}

// GetGenericDisplayColumns returns the generic columns that all resources have
func (h *BaseHandler) GetGenericDisplayColumns() []DisplayColumn {
	return []DisplayColumn{
		{
			Name:     "Name",
			Width:    20,
			Getter:   func(obj client.Object) string { return obj.GetName() },
			Sortable: true,
		},
		{
			Name:     "Namespace",
			Width:    15,
			Getter:   func(obj client.Object) string { return obj.GetNamespace() },
			Sortable: true,
		},
		{
			Name:     "Age",
			Width:    10,
			Getter:   func(obj client.Object) string { return h.getAge(obj) },
			Sortable: true,
		},
	}
}

// buildDeleteArgs builds kubectl delete arguments for a resource
func (h *BaseHandler) buildDeleteArgs(obj client.Object, gvk schema.GroupVersionKind) []string {
	args := []string{"delete", gvk.Kind, obj.GetName()}

	if obj.GetNamespace() != "" {
		args = append(args, "-n", obj.GetNamespace())
	}

	return args
}

// buildEditArgs builds kubectl edit arguments for a resource
func (h *BaseHandler) buildEditArgs(obj client.Object, gvk schema.GroupVersionKind) []string {
	args := []string{"edit", gvk.Kind, obj.GetName()}

	if obj.GetNamespace() != "" {
		args = append(args, "-n", obj.GetNamespace())
	}

	return args
}

// buildDescribeArgs builds kubectl describe arguments for a resource
func (h *BaseHandler) buildDescribeArgs(obj client.Object, gvk schema.GroupVersionKind) []string {
	args := []string{"describe", gvk.Kind, obj.GetName()}

	if obj.GetNamespace() != "" {
		args = append(args, "-n", obj.GetNamespace())
	}

	return args
}

// buildViewArgs builds kubectl get arguments for viewing a resource
func (h *BaseHandler) buildViewArgs(obj client.Object, gvk schema.GroupVersionKind) []string {
	args := []string{"get", gvk.Kind, obj.GetName(), "-o", "yaml"}

	if obj.GetNamespace() != "" {
		args = append(args, "-n", obj.GetNamespace())
	}

	return args
}

// getAge returns the age of the resource
func (h *BaseHandler) getAge(obj client.Object) string {
	// This would typically calculate the age from creation timestamp
	// For now, return a placeholder
	return "1h"
}
