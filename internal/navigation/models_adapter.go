package navigation

import (
	"github.com/sttts/kc/internal/navigation/models"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Exported constructors -----------------------------------------------------------------

func NewRootFolder(deps models.Deps) models.Folder {
	return models.NewRootFolder(deps)
}

func NewContextRootFolder(deps models.Deps, basePath []string) models.Folder {
	return models.NewContextRootFolder(deps, basePath)
}

func NewNamespacedGroupsFolder(deps models.Deps, namespace string, basePath []string) models.Folder {
	return models.NewNamespacedResourcesFolder(deps, namespace, basePath)
}

func NewNamespacedObjectsFolder(deps models.Deps, gvr schema.GroupVersionResource, namespace string, basePath []string) models.Folder {
	return models.NewNamespacedObjectsFolder(deps, gvr, namespace, basePath)
}

func NewClusterObjectsFolder(deps models.Deps, gvr schema.GroupVersionResource, basePath []string) models.Folder {
	return models.NewClusterObjectsFolder(deps, gvr, basePath)
}

func NewPodContainersFolder(deps models.Deps, namespace, pod string, basePath []string) models.Folder {
	return models.NewPodContainersFolder(deps, basePath, namespace, pod)
}

func NewConfigMapKeysFolder(deps models.Deps, namespace, name string, basePath []string) models.Folder {
	return models.NewConfigMapKeysFolder(deps, basePath, namespace, name)
}

func NewSecretKeysFolder(deps models.Deps, namespace, name string, basePath []string) models.Folder {
	return models.NewSecretKeysFolder(deps, basePath, namespace, name)
}

func NewContextsFolder(deps models.Deps, basePath []string) models.Folder {
	return models.NewContextsFolder(deps)
}
