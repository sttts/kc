package models

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ClusterObjectsFolder lists cluster-scoped resources for a GVR.
type ClusterObjectsFolder struct {
	*ObjectsFolder
}

// NewClusterObjectsFolder constructs a cluster-scoped objects folder.
func NewClusterObjectsFolder(deps Deps, gvr schema.GroupVersionResource, path []string) *ClusterObjectsFolder {
	folder := &ClusterObjectsFolder{
		ObjectsFolder: NewObjectsFolder(deps, gvr, "", path),
	}
	return folder
}
