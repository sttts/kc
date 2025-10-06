package models

import (
	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ContextRootFolder represents the root folder when browsing within a context scope.
type ContextRootFolder struct {
	*ClusterResourcesFolder
}

// NewContextRootFolder scaffolds a context-specific root folder using the provided path.
func NewContextRootFolder(deps Deps, path []string) *ContextRootFolder {
	localPath := append([]string{}, path...)
	cluster := NewClusterResourcesFolder(deps, localPath)
	folder := &ContextRootFolder{ClusterResourcesFolder: cluster}
	cluster.BaseFolder.SetPopulate(folder.populate)
	return folder
}

func (f *ContextRootFolder) populate(*BaseFolder) ([]table.Row, error) {
	nameStyle := WhiteStyle()

	gvrNamespaces := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	nsPath := append(append([]string{}, f.Path()...), "namespaces")
	namespacesItem := NewResourceGroupItem(f.Deps, gvrNamespaces, "", "namespaces", []string{"/namespaces", "v1", ""}, nsPath, nameStyle, true, func() (Folder, error) {
		return NewClusterObjectsFolder(f.Deps, gvrNamespaces, nsPath), nil
	})

	groupItems := []*ResourceGroupItem{namespacesItem}
	clusterItems, err := f.ClusterResourcesFolder.resourceGroupItems()
	if err != nil {
		return nil, err
	}
	groupItems = append(groupItems, clusterItems...)

	rows := f.ResourcesFolder.finalize(groupItems)
	return rows, nil
}
