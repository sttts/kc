package models

import (
	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ObjectsFolder provides shared scaffolding for object list folders.
type ObjectsFolder struct {
	*BaseFolder
	gvr       schema.GroupVersionResource
	namespace string // empty means cluster scope
}

// NewObjectsFolder constructs an object-list folder with the provided metadata.
func NewObjectsFolder(deps Deps, gvr schema.GroupVersionResource, namespace string, path []string, key string) *ObjectsFolder {
	base := NewBaseFolder(deps, nil, path, key, nil)
	base.SetColumns([]table.Column{{Title: " Name"}})
	return &ObjectsFolder{
		BaseFolder: base,
		gvr:        gvr,
		namespace:  namespace,
	}
}

// GVR exposes the folder's group-version-resource identifier.
func (o *ObjectsFolder) GVR() schema.GroupVersionResource { return o.gvr }

// Namespace returns the namespace when the folder is namespaced, or an empty string when cluster scoped.
func (o *ObjectsFolder) Namespace() string { return o.namespace }
