package navigation

import (
	"github.com/sttts/kc/internal/navigation/models"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func init() {
	// Register default virtual children for known resources
	RegisterChild(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, func(deps Deps, ns, name string, basePath []string) Folder {
		return models.NewPodContainersFolder(toModelsDeps(deps), basePath, ns, name)
	})
	RegisterChild(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, func(deps Deps, ns, name string, basePath []string) Folder {
		return models.NewConfigMapKeysFolder(toModelsDeps(deps), basePath, ns, name)
	})
	RegisterChild(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, func(deps Deps, ns, name string, basePath []string) Folder {
		return models.NewSecretKeysFolder(toModelsDeps(deps), basePath, ns, name)
	})
	// Selecting a Namespace object enters its namespaced resource groups
	RegisterChild(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, func(deps Deps, ns, name string, basePath []string) Folder {
		key := composeKey(deps, basePath)
		return models.NewNamespacedResourcesFolder(toModelsDeps(deps), name, basePath, key)
	})
}
