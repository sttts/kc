package ui

import (
	"strings"

	models "github.com/sttts/kc/internal/models"
	"github.com/sttts/kc/pkg/appconfig"
)

func isCommandApplicable(cmd appconfig.CommandConfig, item models.Item, selectedCount int, activeNamespace string) bool {
	if selectedCount > 1 && !cmd.SupportsMultiSelection {
		return false
	}

	switch cmd.Type {
	case appconfig.CommandTypeGlobal:
		return true
	case appconfig.CommandTypeNamespace:
		if activeNamespace == "" {
			return false
		}
	case appconfig.CommandTypeSelector, appconfig.CommandTypeSticky:
		if item == nil {
			return false
		}
	default:
		if item == nil {
			return false
		}
	}

	if item == nil {
		// Namespace commands are allowed without a selected object as long as a namespace is active.
		return true
	}

	obj, ok := item.(models.ObjectItem)
	if !ok {
		// Non-object items (e.g., resource groups) are eligible unless the command
		// narrows to specific GVRs.
		if len(cmd.ShowFor.Resources) > 0 || len(cmd.ShowFor.Groups) > 0 {
			return false
		}
		return true
	}
	gvr := obj.GVR()

	if len(cmd.ShowFor.Resources) > 0 {
		match := false
		for _, r := range cmd.ShowFor.Resources {
			if strings.EqualFold(r, gvr.Resource) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	if len(cmd.ShowFor.Groups) > 0 {
		match := false
		for _, g := range cmd.ShowFor.Groups {
			if strings.EqualFold(g, gvr.Group) || (g == "" && gvr.Group == "") {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	return true
}

func deriveNamespace(item models.Item, selected []Item, folderNamespace string) string {
	if ns := namespaceFromItem(item); ns != "" {
		return ns
	}
	for _, sel := range selected {
		if ns := namespaceFromItem(sel.Item); ns != "" {
			return ns
		}
	}
	// Namespace objects themselves carry no namespace; use their name.
	if obj, ok := item.(models.ObjectItem); ok {
		gvr := obj.GVR()
		if gvr.Group == "" && gvr.Resource == "namespaces" {
			if name := obj.Name(); name != "" {
				return name
			}
		}
		// Cluster-scoped object: do not inherit folder namespace.
		return ""
	}

	// If there is a concrete (non-namespaced) selection, do not inherit folder namespace.
	if item != nil || len(selected) > 0 {
		return ""
	}

	if folderNamespace != "" {
		return folderNamespace
	}
	return ""
}

func namespaceFromItem(item models.Item) string {
	if item == nil {
		return ""
	}
	if obj, ok := item.(models.ObjectItem); ok {
		return obj.Namespace()
	}
	return ""
}
