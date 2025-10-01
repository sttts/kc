package models

import (
	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ContextRootFolder represents the root folder when browsing within a context scope.
type ContextRootFolder struct {
	*ClusterResourcesFolder
	ContextName string
}

// NewContextRootFolder scaffolds a context-specific root folder.
func NewContextRootFolder(deps Deps, contextName string) *ContextRootFolder {
	path := []string{"contexts"}
	if contextName != "" {
		path = append(path, contextName)
	}
	key := composeKey(deps, path)
	cluster := NewClusterResourcesFolder(deps, path, key)
	folder := &ContextRootFolder{
		ClusterResourcesFolder: cluster,
		ContextName:            contextName,
	}
	cluster.BaseFolder.SetPopulate(folder.populate)
	return folder
}

func (f *ContextRootFolder) populate(*BaseFolder) ([]table.Row, error) {
	opts := resolveViewOptions(f.Deps)
	nameStyle := WhiteStyle()
	rows := make([]table.Row, 0, 64)

	gvrNamespaces := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	nsPath := append(append([]string{}, f.Path()...), "namespaces")
	namespacesItem := NewResourceGroupItem(f.Deps, gvrNamespaces, "", "namespaces", []string{"/namespaces", "v1", ""}, nsPath, nameStyle, true, func() (Folder, error) {
		key := composeKey(f.Deps, nsPath)
		return NewClusterObjectsFolder(f.Deps, gvrNamespaces, nsPath, key), nil
	})

	groupItems := []*ResourceGroupItem{namespacesItem}
	clusterItems, err := f.resourceGroupItems(opts)
	if err != nil {
		return nil, err
	}
	groupItems = append(groupItems, clusterItems...)
	rows = append(rows, f.ResourcesFolder.finalize(groupItems, opts)...)

	return rows, nil
}
