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
		return false
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

func deriveNamespace(item models.Item, selected []Item, path string) string {
	if ns := namespaceFromItem(item); ns != "" {
		return ns
	}
	for _, sel := range selected {
		if ns := namespaceFromItem(sel.Item); ns != "" {
			return ns
		}
	}
	return namespaceFromPath(path)
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

func namespaceFromPath(path string) string {
	parts := strings.Split(path, "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "namespaces" && parts[i+1] != "" {
			return parts[i+1]
		}
	}
	return ""
}
