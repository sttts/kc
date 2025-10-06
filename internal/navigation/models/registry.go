package models

import (
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ChildConstructor builds a child folder for an object row of a given GVR.
type ChildConstructor func(Deps, string, string, []string) Folder

var (
	childMu  sync.RWMutex
	childMap = map[schema.GroupVersionResource]ChildConstructor{}
)

// RegisterChild registers a constructor to build virtual child folders for the
// provided GVR (e.g., Pods -> Containers, ConfigMaps -> Keys).
func RegisterChild(gvr schema.GroupVersionResource, ctor ChildConstructor) {
	childMu.Lock()
	childMap[gvr] = ctor
	childMu.Unlock()
}

// ChildFor returns a registered constructor for the given GVR when available.
func ChildFor(gvr schema.GroupVersionResource) (ChildConstructor, bool) {
	childMu.RLock()
	ctor, ok := childMap[gvr]
	childMu.RUnlock()
	return ctor, ok
}

func init() {
	RegisterChild(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, func(deps Deps, ns, name string, basePath []string) Folder {
		return NewPodContainersFolder(deps, basePath, ns, name)
	})
	RegisterChild(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, func(deps Deps, ns, name string, basePath []string) Folder {
		return NewConfigMapKeysFolder(deps, basePath, ns, name)
	})
	RegisterChild(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, func(deps Deps, ns, name string, basePath []string) Folder {
		return NewSecretKeysFolder(deps, basePath, ns, name)
	})
	RegisterChild(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, func(deps Deps, ns, name string, basePath []string) Folder {
		return NewNamespacedResourcesFolder(deps, name, basePath)
	})
}
