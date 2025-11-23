package models

import (
	"context"
	"fmt"

	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ChildResourceTypesFolder lists the child resource types of a parent object (e.g., /replicasets under a Deployment).
type ChildResourceTypesFolder struct {
	*BaseFolder
	parentGVR       schema.GroupVersionResource
	parentNamespace string
	parentName      string
	childGVRs       []schema.GroupVersionResource
}

// NewChildResourceTypesFolder builds a folder with one entry per childGVR under the given parent object path.
// Entries are simple folder items without counts; entering one opens the filtered child resource list.
func NewChildResourceTypesFolder(deps Deps, parentGVR schema.GroupVersionResource, parentNamespace, parentName string, childGVRs []schema.GroupVersionResource, path []string) *ChildResourceTypesFolder {
	cols := []table.Column{{Title: " Name"}}
	base := NewBaseFolder(deps, cols, path)
	folder := &ChildResourceTypesFolder{
		BaseFolder:      base,
		parentGVR:       parentGVR,
		parentNamespace: parentNamespace,
		parentName:      parentName,
		childGVRs:       append([]schema.GroupVersionResource(nil), childGVRs...),
	}
	base.SetPopulate(folder.populate)
	return folder
}

func (f *ChildResourceTypesFolder) populate(ctx context.Context) ([]table.Row, error) {
	rows := make([]table.Row, 0, len(f.childGVRs))
	for _, child := range f.childGVRs {
		childGVR := child
		resName := childGVR.Resource
		id := fmt.Sprintf("%s/%s/%s/%s/%s", f.parentNamespace, f.parentName, childGVR.Group, childGVR.Version, childGVR.Resource)
		path := append(append([]string{}, f.Path()...), resName)
		item := NewChildResourceItem(id, []string{"/" + resName}, path, func() (Folder, error) {
			return NewChildResourcesFolder(f.Deps, f.parentGVR, f.parentNamespace, f.parentName, childGVR, path), nil
		})
		rows = append(rows, item)
	}
	return rows, nil
}
