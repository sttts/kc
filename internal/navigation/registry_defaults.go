package navigation

import "k8s.io/apimachinery/pkg/runtime/schema"

func init() {
    // Register default virtual children for known resources
    RegisterChild(schema.GroupVersionResource{Group:"", Version:"v1", Resource:"pods"}, func(deps Deps, ns, name string, basePath []string) Folder {
        return NewPodContainersFolder(deps, ns, name, basePath)
    })
    RegisterChild(schema.GroupVersionResource{Group:"", Version:"v1", Resource:"configmaps"}, func(deps Deps, ns, name string, basePath []string) Folder {
        return NewConfigMapKeysFolder(deps, ns, name, basePath)
    })
    RegisterChild(schema.GroupVersionResource{Group:"", Version:"v1", Resource:"secrets"}, func(deps Deps, ns, name string, basePath []string) Folder {
        return NewSecretKeysFolder(deps, ns, name, basePath)
    })
}
