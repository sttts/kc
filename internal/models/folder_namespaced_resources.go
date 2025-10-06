package models

import (
	"fmt"
	"strings"

	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NamespacedResourcesFolder models resource groups scoped to a namespace.
type NamespacedResourcesFolder struct {
	*ResourcesFolder
	Namespace string
}

// NewNamespacedResourcesFolder creates a namespace-scoped resources folder.
func NewNamespacedResourcesFolder(deps Deps, namespace string, path []string) *NamespacedResourcesFolder {
	base := NewBaseFolder(deps, nil, path, nil)
	folder := &NamespacedResourcesFolder{
		ResourcesFolder: NewResourcesFolder(base),
		Namespace:       namespace,
	}
	base.SetPopulate(folder.populate)
	return folder
}

func (f *NamespacedResourcesFolder) populate(*BaseFolder) ([]table.Row, error) {
	items, err := f.resourceGroupItems()
	if err != nil {
		return nil, err
	}
	rows := f.ResourcesFolder.finalize(items)
	return rows, nil
}

func (f *NamespacedResourcesFolder) resourceGroupItems() ([]*ResourceGroupItem, error) {
	cfg := f.Deps.Config()
	infos, err := f.Deps.Cl.GetResourceInfos()
	if err != nil {
		return nil, err
	}
	entries := make([]resourceEntry, 0, len(infos))
	for _, info := range infos {
		if !info.Namespaced {
			continue
		}
		if !verbsInclude(info.Verbs, "list") || !verbsInclude(info.Verbs, "watch") {
			continue
		}
		gvr := schema.GroupVersionResource{Group: info.GVK.Group, Version: info.GVK.Version, Resource: info.Resource}
		entries = append(entries, resourceEntry{info: info, gvr: gvr})
	}
	sortResourceEntries(entries, strings.ToLower(string(cfg.Resources.Order)), favoritesMap(cfg.Resources.Favorites))
	items := make([]*ResourceGroupItem, 0, len(entries))
	nameStyle := WhiteStyle()
	for _, entry := range entries {
		id := fmt.Sprintf("%s/%s/%s/%s", f.Namespace, entry.gvr.Group, entry.gvr.Version, entry.gvr.Resource)
		cells := []string{"/" + entry.info.Resource, groupVersionString(entry.info.GVK.Group, entry.info.GVK.Version), ""}
		basePath := append(append([]string{}, f.Path()...), entry.info.Resource)
		gvr := entry.gvr
		ns := f.Namespace
		item := NewResourceGroupItem(f.Deps, gvr, ns, id, cells, basePath, nameStyle, true, func() (Folder, error) {
			return NewNamespacedObjectsFolder(f.Deps, gvr, ns, basePath), nil
		})
		items = append(items, item)
	}
	return items, nil
}
