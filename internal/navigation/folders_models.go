package navigation

import (
	"github.com/sttts/kc/internal/navigation/models"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type RootFolder struct{ *folderWrapper }

type ContextRootFolder struct{ *folderWrapper }

type NamespacedGroupsFolder struct{ *folderWrapper }

type NamespacedObjectsFolder struct{ *folderWrapper }

type ClusterObjectsFolder struct{ *folderWrapper }

type PodContainersFolder struct{ *folderWrapper }

type ConfigMapKeysFolder struct{ *folderWrapper }

type SecretKeysFolder struct{ *folderWrapper }

type ContextsFolder struct{ *folderWrapper }

func NewRootFolder(deps Deps) *RootFolder {
	fw := newFolderWrapper(models.NewRootFolder(toModelsDeps(deps)))
	return &RootFolder{folderWrapper: fw}
}

func NewContextRootFolder(deps Deps, basePath []string) *ContextRootFolder {
	name := ""
	if len(basePath) > 1 {
		name = basePath[len(basePath)-1]
	}
	fw := newFolderWrapper(models.NewContextRootFolder(toModelsDeps(deps), name))
	setPathAndKey(fw.inner, basePath, composeKey(deps, basePath))
	return &ContextRootFolder{folderWrapper: fw}
}

func NewNamespacedGroupsFolder(deps Deps, namespace string, basePath []string) *NamespacedGroupsFolder {
	key := composeKey(deps, basePath)
	fw := newFolderWrapper(models.NewNamespacedResourcesFolder(toModelsDeps(deps), namespace, basePath, key))
	return &NamespacedGroupsFolder{folderWrapper: fw}
}

func NewNamespacedObjectsFolder(deps Deps, gvr schema.GroupVersionResource, namespace string, basePath []string) *NamespacedObjectsFolder {
	key := composeKey(deps, basePath)
	fw := newFolderWrapper(models.NewNamespacedObjectsFolder(toModelsDeps(deps), gvr, namespace, basePath, key))
	return &NamespacedObjectsFolder{folderWrapper: fw}
}

func NewClusterObjectsFolder(deps Deps, gvr schema.GroupVersionResource, basePath []string) *ClusterObjectsFolder {
	key := composeKey(deps, basePath)
	fw := newFolderWrapper(models.NewClusterObjectsFolder(toModelsDeps(deps), gvr, basePath, key))
	return &ClusterObjectsFolder{folderWrapper: fw}
}

func NewPodContainersFolder(deps Deps, ns, pod string, basePath []string) *PodContainersFolder {
	fw := newFolderWrapper(models.NewPodContainersFolder(toModelsDeps(deps), basePath, ns, pod))
	return &PodContainersFolder{folderWrapper: fw}
}

func NewConfigMapKeysFolder(deps Deps, ns, name string, basePath []string) *ConfigMapKeysFolder {
	fw := newFolderWrapper(models.NewConfigMapKeysFolder(toModelsDeps(deps), basePath, ns, name))
	return &ConfigMapKeysFolder{folderWrapper: fw}
}

func NewSecretKeysFolder(deps Deps, ns, name string, basePath []string) *SecretKeysFolder {
	fw := newFolderWrapper(models.NewSecretKeysFolder(toModelsDeps(deps), basePath, ns, name))
	return &SecretKeysFolder{folderWrapper: fw}
}

func NewContextsFolder(deps Deps, basePath []string) *ContextsFolder {
	fw := newFolderWrapper(models.NewContextsFolder(toModelsDeps(deps)))
	if len(basePath) > 0 {
		setPathAndKey(fw.inner, basePath, composeKey(deps, basePath))
	}
	return &ContextsFolder{folderWrapper: fw}
}

func setPathAndKey(folder models.Folder, path []string, key string) {
	if setter, ok := folder.(interface{ SetPath([]string) }); ok {
		setter.SetPath(path)
	}
	if setter, ok := folder.(interface{ SetKey(string) }); ok {
		setter.SetKey(key)
	}
}

// Forward optional interfaces -------------------------------------------------

func (f *NamespacedObjectsFolder) ObjectListMeta() (schema.GroupVersionResource, string, bool) {
	return f.folderWrapper.ObjectListMeta()
}

func (f *ClusterObjectsFolder) ObjectListMeta() (schema.GroupVersionResource, string, bool) {
	return f.folderWrapper.ObjectListMeta()
}

func (f *ConfigMapKeysFolder) Parent() (schema.GroupVersionResource, string, string) {
	return f.folderWrapper.Parent()
}

func (f *SecretKeysFolder) Parent() (schema.GroupVersionResource, string, string) {
	return f.folderWrapper.Parent()
}
