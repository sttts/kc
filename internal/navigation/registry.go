package navigation

import (
    "sync"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

// ChildConstructor builds a next-level Folder for an object row of a given GVR.
// ns may be empty for cluster-scoped objects. basePath is the absolute path
// segments to the parent object (e.g., ["namespaces","ns","configmaps","cm1"]).
type ChildConstructor func(deps Deps, ns, name string, basePath []string) Folder

var (
    regMu  sync.RWMutex
    regMap = map[schema.GroupVersionResource]ChildConstructor{}
)

// RegisterChild registers a constructor for virtual children under object rows
// of the given GVR (e.g., pods → containers, configmaps/secrets → data keys).
func RegisterChild(gvr schema.GroupVersionResource, ctor ChildConstructor) {
    regMu.Lock(); defer regMu.Unlock()
    regMap[gvr] = ctor
}

func childFor(gvr schema.GroupVersionResource) (ChildConstructor, bool) {
    regMu.RLock(); defer regMu.RUnlock()
    c, ok := regMap[gvr]
    return c, ok
}
