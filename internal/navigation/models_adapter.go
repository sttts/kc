package navigation

import (
	"strings"

	"github.com/sttts/kc/internal/navigation/models"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Exported constructors -----------------------------------------------------------------

func NewRootFolder(deps models.Deps) models.Folder {
	return models.NewRootFolder(deps)
}

func NewContextRootFolder(deps models.Deps, basePath []string) models.Folder {
	name := ""
	if len(basePath) > 1 {
		name = basePath[len(basePath)-1]
	}
	return models.NewContextRootFolder(deps, name)
}

func NewNamespacedGroupsFolder(deps models.Deps, namespace string, basePath []string) models.Folder {
	key := composeKey(deps, basePath)
	return models.NewNamespacedResourcesFolder(deps, namespace, basePath, key)
}

func NewNamespacedObjectsFolder(deps models.Deps, gvr schema.GroupVersionResource, namespace string, basePath []string) models.Folder {
	key := composeKey(deps, basePath)
	return models.NewNamespacedObjectsFolder(deps, gvr, namespace, basePath, key)
}

func NewClusterObjectsFolder(deps models.Deps, gvr schema.GroupVersionResource, basePath []string) models.Folder {
	key := composeKey(deps, basePath)
	return models.NewClusterObjectsFolder(deps, gvr, basePath, key)
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

func composeKey(deps models.Deps, path []string) string {
	if len(path) == 0 {
		return deps.CtxName
	}
	rel := strings.Join(path, "/")
	if deps.CtxName == "" {
		return rel
	}
	return deps.CtxName + "/" + rel
}
