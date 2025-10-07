package models

import (
	"fmt"

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
	base := NewBaseFolder(deps, nil, path)
	folder := &NamespacedResourcesFolder{
		ResourcesFolder: NewResourcesFolder(base),
		Namespace:       namespace,
	}
	base.SetPopulate(folder.populate)
	return folder
}

func (f *NamespacedResourcesFolder) populate() ([]table.Row, error) {
	specs, err := f.resourceGroupSpecs()
	if err != nil {
		return nil, err
	}
	rows := f.ResourcesFolder.finalize(specs)
	return rows, nil
}

func (f *NamespacedResourcesFolder) resourceGroupSpecs() ([]resourceGroupSpec, error) {
	cfg := f.Deps.AppConfig
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
	sortResourceEntries(entries, cfg.Resources.Order, favoritesMap(cfg.Resources.Favorites))
	specs := make([]resourceGroupSpec, 0, len(entries))
	nameStyle := WhiteStyle()
	for _, entry := range entries {
		id := fmt.Sprintf("%s/%s/%s/%s", f.Namespace, entry.gvr.Group, entry.gvr.Version, entry.gvr.Resource)
		cells := []string{"/" + entry.info.Resource, groupVersionString(entry.info.GVK.Group, entry.info.GVK.Version), ""}
		basePath := append(append([]string{}, f.Path()...), entry.info.Resource)
		cellsCopy := append([]string(nil), cells...)
		pathCopy := append([]string(nil), basePath...)
		gvr := entry.gvr
		ns := f.Namespace
		specs = append(specs, resourceGroupSpec{
			id:        id,
			cells:     cellsCopy,
			path:      pathCopy,
			style:     nameStyle,
			gvr:       gvr,
			namespace: ns,
			watchable: true,
			enter: func() (Folder, error) {
				return NewNamespacedObjectsFolder(f.Deps, gvr, ns, pathCopy), nil
			},
		})
	}
	return specs, nil
}
