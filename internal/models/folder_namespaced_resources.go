package models

import (
	"context"
	"fmt"
	"strings"

	table "github.com/sttts/kc/internal/table"
	"github.com/sttts/kc/pkg/appconfig"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NamespacedResourcesFolder models resource groups scoped to a namespace.
type NamespacedResourcesFolder struct {
	*ResourcesFolder
	namespace string
}

// NewNamespacedResourcesFolder creates a namespace-scoped resources folder.
func NewNamespacedResourcesFolder(deps Deps, namespace string, path []string) *NamespacedResourcesFolder {
	base := NewBaseFolder(deps, nil, path)
	folder := &NamespacedResourcesFolder{
		ResourcesFolder: NewResourcesFolder(base),
		namespace:       namespace,
	}
	base.SetPopulate(folder.populate)
	return folder
}

// Namespace returns the namespace associated with this folder.
func (f *NamespacedResourcesFolder) Namespace() string {
	return f.namespace
}

func (f *NamespacedResourcesFolder) populate(ctx context.Context) ([]table.Row, error) {
	specs, err := f.resourceGroupSpecs()
	if err != nil {
		return nil, err
	}
	rows := f.ResourcesFolder.finalize(ctx, specs)
	return rows, nil
}

func (f *NamespacedResourcesFolder) resourceGroupSpecs() ([]resourceGroupSpec, error) {
	_, order, favorites := f.viewSettings()
	favSet := favoritesMap(favorites)
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
	if order == appconfig.OrderFavorites && len(favSet) == 0 {
		favorites = favoritesFromCategories(entries)
		favSet = favoritesMap(favorites)
	}
	sortResourceEntries(entries, order, favSet)
	specs := make([]resourceGroupSpec, 0, len(entries))
	nameStyle := WhiteStyle()
	favoriteStyle := FavoriteResourceStyle()
	highlightFavorites := order == appconfig.OrderFavorites && len(favSet) > 0
	for _, entry := range entries {
		verbsCopy := append([]string(nil), entry.info.Verbs...)
		id := fmt.Sprintf("%s/%s/%s/%s", f.namespace, entry.gvr.Group, entry.gvr.Version, entry.gvr.Resource)
		gvLabel := groupVersionString(entry.info.GVK.Group, entry.info.GVK.Version)
		cells := []string{"/" + entry.info.Resource, gvLabel, ""}
		basePath := append(append([]string{}, f.Path()...), entry.info.Resource)
		cellsCopy := append([]string(nil), cells...)
		pathCopy := append([]string(nil), basePath...)
		gvr := entry.gvr
		ns := f.namespace
		detail := fmt.Sprintf("%s (%s)", entry.info.Resource, gvLabel)
		style := nameStyle
		if highlightFavorites && favSet[strings.ToLower(entry.info.Resource)] {
			style = favoriteStyle
		}
		specs = append(specs, resourceGroupSpec{
			id:        id,
			cells:     cellsCopy,
			path:      pathCopy,
			detail:    detail,
			style:     style,
			gvr:       gvr,
			namespace: ns,
			watchable: true,
			enter: func() (Folder, error) {
				return NewNamespacedObjectsFolder(f.Deps, gvr, ns, pathCopy, verbsCopy), nil
			},
		})
	}
	return specs, nil
}
