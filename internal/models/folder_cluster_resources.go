package models

import (
	"fmt"

	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ClusterResourcesFolder models resource groups that are cluster scoped (e.g., nodes).
type ClusterResourcesFolder struct {
	*ResourcesFolder
}

// NewClusterResourcesFolder creates a cluster-scoped resources folder.
func NewClusterResourcesFolder(deps Deps, path []string) *ClusterResourcesFolder {
	base := NewBaseFolder(deps, nil, path, nil)
	folder := &ClusterResourcesFolder{ResourcesFolder: NewResourcesFolder(base)}
	base.SetPopulate(folder.populate)
	return folder
}

func (f *ClusterResourcesFolder) populate(*BaseFolder) ([]table.Row, error) {
	specs, err := f.resourceGroupSpecs()
	if err != nil {
		return nil, err
	}
	rows := f.ResourcesFolder.finalize(specs)
	return rows, nil
}

func (f *ClusterResourcesFolder) resourceGroupSpecs() ([]resourceGroupSpec, error) {
	cfg := f.Deps.AppConfig
	infos, err := f.Deps.Cl.GetResourceInfos()
	if err != nil {
		return nil, err
	}
	entries := make([]resourceEntry, 0, len(infos))
	for _, info := range infos {
		if info.Namespaced || info.Resource == "namespaces" {
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
		id := fmt.Sprintf("%s/%s/%s", entry.gvr.Group, entry.gvr.Version, entry.gvr.Resource)
		cells := []string{"/" + entry.info.Resource, groupVersionString(entry.info.GVK.Group, entry.info.GVK.Version), ""}
		basePath := append(append([]string{}, f.Path()...), entry.info.Resource)
		cellsCopy := append([]string(nil), cells...)
		pathCopy := append([]string(nil), basePath...)
		gvr := entry.gvr
		specs = append(specs, resourceGroupSpec{
			id:        id,
			cells:     cellsCopy,
			path:      pathCopy,
			style:     nameStyle,
			gvr:       gvr,
			namespace: "",
			watchable: true,
			enter: func() (Folder, error) {
				return NewClusterObjectsFolder(f.Deps, gvr, pathCopy), nil
			},
		})
	}
	return specs, nil
}
