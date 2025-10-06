package models

import (
	"fmt"

	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// RootFolder represents the "/" entry point listing contexts, namespaces, and cluster resources.
type RootFolder struct {
	*ClusterResourcesFolder
	enterContext func(name string, basePath []string) (Folder, error)
}

// NewRootFolder scaffolds a root folder with default columns.
func NewRootFolder(deps Deps, enterContext func(name string, basePath []string) (Folder, error)) *RootFolder {
	cluster := NewClusterResourcesFolder(deps, nil)
	root := &RootFolder{ClusterResourcesFolder: cluster, enterContext: enterContext}
	cluster.BaseFolder.SetPopulate(root.populate)
	return root
}

func (f *RootFolder) populate(*BaseFolder) ([]table.Row, error) {
	cfg := f.Deps.AppConfig
	showNonEmpty := cfg.Resources.ShowNonEmptyOnly

	rows := make([]table.Row, 0, 64)
	nameStyle := WhiteStyle()

	if count := len(f.Deps.KubeConfig.Contexts); count > 0 {
		itemPath := append(append([]string{}, f.Path()...), "contexts")
		enter := func() (Folder, error) {
			return NewContextsFolder(f.Deps, f.enterContext), nil
		}
		item := NewContextListItem("contexts", []string{"/contexts", "", ""}, itemPath, GreenStyle(), count, enter)
		if !(showNonEmpty && item.Empty()) {
			item.Cells[2] = fmt.Sprintf("%d", item.Count())
			rows = append(rows, item)
		}
	}

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
	rows = append(rows, f.ResourcesFolder.finalize(groupItems)...)

	return rows, nil
}
