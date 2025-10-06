package navigation

import (
	"github.com/sttts/kc/internal/navigation/models"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ChildConstructor builds a next-level Folder for an object row of a given GVR.
// ns may be empty for cluster-scoped objects. basePath is the absolute path
// segments to the parent object (e.g., ["namespaces","ns","configmaps","cm1"]).
type ChildConstructor func(deps Deps, ns, name string, basePath []string) Folder

// RegisterChild registers a constructor for virtual children under object rows
// of the given GVR (e.g., pods → containers, configmaps/secrets → data keys).
func RegisterChild(gvr schema.GroupVersionResource, ctor ChildConstructor) {
	models.RegisterChild(gvr, func(deps models.Deps, ns, name string, basePath []string) models.Folder {
		return ctor(fromModelsDeps(deps), ns, name, basePath)
	})
}
