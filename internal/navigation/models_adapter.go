package navigation

import (
	"strings"

	"github.com/sttts/kc/internal/navigation/models"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Exported constructors -----------------------------------------------------------------

func NewRootFolder(deps Deps) Folder {
	return models.NewRootFolder(toModelsDeps(deps))
}

func NewContextRootFolder(deps Deps, basePath []string) Folder {
	name := ""
	if len(basePath) > 1 {
		name = basePath[len(basePath)-1]
	}
	return models.NewContextRootFolder(toModelsDeps(deps), name)
}

func NewNamespacedGroupsFolder(deps Deps, namespace string, basePath []string) Folder {
	key := composeKey(deps, basePath)
	return models.NewNamespacedResourcesFolder(toModelsDeps(deps), namespace, basePath, key)
}

func NewNamespacedObjectsFolder(deps Deps, gvr schema.GroupVersionResource, namespace string, basePath []string) Folder {
	key := composeKey(deps, basePath)
	return models.NewNamespacedObjectsFolder(toModelsDeps(deps), gvr, namespace, basePath, key)
}

func NewClusterObjectsFolder(deps Deps, gvr schema.GroupVersionResource, basePath []string) Folder {
	key := composeKey(deps, basePath)
	return models.NewClusterObjectsFolder(toModelsDeps(deps), gvr, basePath, key)
}

func NewPodContainersFolder(deps Deps, namespace, pod string, basePath []string) Folder {
	return models.NewPodContainersFolder(toModelsDeps(deps), basePath, namespace, pod)
}

func NewConfigMapKeysFolder(deps Deps, namespace, name string, basePath []string) Folder {
	return models.NewConfigMapKeysFolder(toModelsDeps(deps), basePath, namespace, name)
}

func NewSecretKeysFolder(deps Deps, namespace, name string, basePath []string) Folder {
	return models.NewSecretKeysFolder(toModelsDeps(deps), basePath, namespace, name)
}

func NewContextsFolder(deps Deps, basePath []string) Folder {
	return models.NewContextsFolder(toModelsDeps(deps))
}

func toModelsDeps(d Deps) models.Deps {
	return models.Deps{
		Cl:           d.Cl,
		Ctx:          d.Ctx,
		CtxName:      d.CtxName,
		ListContexts: d.ListContexts,
		EnterContext: func(name string, basePath []string) (models.Folder, error) {
			if d.EnterContext == nil {
				return nil, nil
			}
			return d.EnterContext(name, basePath)
		},
		ViewOptions: func() models.ViewOptions {
			if d.ViewOptions == nil {
				return models.ViewOptions{}
			}
			vo := d.ViewOptions()
			return models.ViewOptions{
				ShowNonEmptyOnly: vo.ShowNonEmptyOnly,
				Order:            vo.Order,
				Favorites:        vo.Favorites,
				Columns:          vo.Columns,
				ObjectsOrder:     vo.ObjectsOrder,
				PeekInterval:     vo.PeekInterval,
			}
		},
	}
}

func fromModelsDeps(d models.Deps) Deps {
	return Deps{
		Cl:           d.Cl,
		Ctx:          d.Ctx,
		CtxName:      d.CtxName,
		ListContexts: d.ListContexts,
		EnterContext: func(name string, basePath []string) (Folder, error) {
			if d.EnterContext == nil {
				return nil, nil
			}
			return d.EnterContext(name, basePath)
		},
		ViewOptions: func() ViewOptions {
			if d.ViewOptions == nil {
				return ViewOptions{}
			}
			vo := d.ViewOptions()
			return ViewOptions{
				ShowNonEmptyOnly: vo.ShowNonEmptyOnly,
				Order:            vo.Order,
				Favorites:        vo.Favorites,
				Columns:          vo.Columns,
				ObjectsOrder:     vo.ObjectsOrder,
				PeekInterval:     vo.PeekInterval,
			}
		},
	}
}

func composeKey(deps Deps, path []string) string {
	if len(path) == 0 {
		return deps.CtxName
	}
	rel := strings.Join(path, "/")
	if deps.CtxName == "" {
		return rel
	}
	return deps.CtxName + "/" + rel
}
