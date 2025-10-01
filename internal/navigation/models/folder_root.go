package models

import (
	"fmt"

	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// RootFolder represents the "/" entry point listing contexts, namespaces, and cluster resources.
type RootFolder struct {
	*ClusterResourcesFolder
}

// NewRootFolder scaffolds a root folder with default columns.
func NewRootFolder(deps Deps) *RootFolder {
	cluster := NewClusterResourcesFolder(deps, nil, "root")
	root := &RootFolder{ClusterResourcesFolder: cluster}
	cluster.BaseFolder.SetPopulate(root.populate)
	return root
}

func (f *RootFolder) populate(*BaseFolder) ([]table.Row, error) {
	rows := make([]table.Row, 0, 64)
	opts := resolveViewOptions(f.Deps)
	nameStyle := WhiteStyle()

	if f.Deps.ListContexts != nil {
		itemPath := append(append([]string{}, f.Path()...), "contexts")
		item := NewContextListItem("contexts", []string{"/contexts", "", ""}, itemPath, GreenStyle(), f.Deps.ListContexts, func() (Folder, error) {
			return NewContextsFolder(f.Deps), nil
		})
		if !(opts.ShowNonEmptyOnly && item.Empty()) {
			item.Cells[2] = fmt.Sprintf("%d", item.Count())
			rows = append(rows, item)
		}
	}

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
