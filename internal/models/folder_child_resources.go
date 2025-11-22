package models

import (
	"context"
	"fmt"

	table "github.com/sttts/kc/internal/table"
	"github.com/sttts/kc/internal/tablecache"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NewChildResourcesFolder creates a folder that lists resources of childGVR
// that are owned by the parent resource (parentGVR/parentNamespace/parentName).
func NewChildResourcesFolder(deps Deps, parentGVR schema.GroupVersionResource, parentNamespace, parentName string, childGVR schema.GroupVersionResource, path []string) *ObjectsFolder {
	// Create the base ObjectsFolder for the child resource
	folder := NewObjectsFolder(deps, childGVR, parentNamespace, path, nil)

	// Capture the original populate function
	originalPopulate := folder.rows.populate

	// Override populate to fetch parent UID first and set the filter
	folder.rows.populate = func(ctx context.Context) ([]table.Row, error) {
		// 1. Fetch parent to get UID
		parent, err := deps.Cl.GetByGVR(ctx, parentGVR, parentNamespace, parentName)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			return nil, fmt.Errorf("parent %s/%s not found", parentNamespace, parentName)
		}
		parentUID := parent.GetUID()

		// 2. Set filter on folder
		folder.Filter = func(row *tablecache.Row) bool {
			for _, owner := range row.ObjectMeta.OwnerReferences {
				if owner.UID == parentUID {
					return true
				}
			}
			return false
		}
		folder.FilterUnstructured = func(u *unstructured.Unstructured) bool {
			for _, owner := range u.GetOwnerReferences() {
				if owner.UID == parentUID {
					return true
				}
			}
			return false
		}

		// 3. Call original populate
		return originalPopulate(ctx)
	}

	return folder
}
