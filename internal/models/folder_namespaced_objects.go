package models

import (
	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NamespacedObjectsFolder lists namespace-scoped resources for a GVR.
type NamespacedObjectsFolder struct {
	*ObjectsFolder
}

// NewNamespacedObjectsFolder constructs a namespaced objects folder.
func NewNamespacedObjectsFolder(deps Deps, gvr schema.GroupVersionResource, namespace string, path []string) *NamespacedObjectsFolder {
	folder := &NamespacedObjectsFolder{
		ObjectsFolder: NewObjectsFolder(deps, gvr, namespace, path),
	}
	folder.BaseFolder.SetPopulate(folder.populate)
	return folder
}

func (f *NamespacedObjectsFolder) populate() ([]table.Row, error) {
	return f.ObjectsFolder.populateRows()
}
