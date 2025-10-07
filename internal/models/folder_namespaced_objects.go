package models

import (
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
	return folder
}
