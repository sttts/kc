package navigation

import (
    "github.com/charmbracelet/lipgloss/v2"
    "github.com/sttts/kc/internal/navigation/models"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

type (
    RowItem             = models.RowItem
    SimpleItem          = models.SimpleItem
    ObjectRow           = models.ObjectRow
    NamespaceItem       = models.NamespaceItem
    ObjectWithChildItem = models.ObjectWithChildItem
    PodItem             = models.PodItem
    ConfigMapItem       = models.ConfigMapItem
    SecretItem          = models.SecretItem
    ContextItem         = models.ContextItem
    ContextListItem     = models.ContextListItem
    ResourceGroupItem   = models.ResourceGroupItem
)

func newRowItem(id string, cells []string, path []string, style *lipgloss.Style) *RowItem {
    return models.NewRowItem(id, cells, path, style)
}

func newRowItemStyled(id string, cells []string, path []string, styles []*lipgloss.Style) *RowItem {
    return models.NewRowItemStyled(id, cells, path, styles)
}

func NewSimpleItem(id string, cells []string, path []string, style *lipgloss.Style) *SimpleItem {
    return models.NewSimpleItem(id, cells, path, style)
}

func newObjectRow(id string, cells []string, path []string, gvr schema.GroupVersionResource, namespace, name string, style *lipgloss.Style) *ObjectRow {
    return models.NewObjectRow(id, cells, path, gvr, namespace, name, style)
}

func newNamespaceItem(obj *ObjectRow, enter func() (Folder, error)) *NamespaceItem {
    return models.NewNamespaceItem(obj, enter)
}

func newObjectWithChildItem(obj *ObjectRow, enter func() (Folder, error)) *ObjectWithChildItem {
    return models.NewObjectWithChildItem(obj, enter)
}

func newPodItem(row *ObjectRow) *PodItem                 { return models.NewPodItem(row) }
func newConfigMapItem(row *ObjectRow) *ConfigMapItem     { return models.NewConfigMapItem(row) }
func newSecretItem(row *ObjectRow) *SecretItem           { return models.NewSecretItem(row) }
func newContextItem(id string, cells []string, path []string, style *lipgloss.Style, enter func() (Folder, error)) *ContextItem {
    return models.NewContextItem(id, cells, path, style, enter)
}

func newContextListItem(id string, cells []string, path []string, style *lipgloss.Style, contextsFn func() []string, enter func() (Folder, error)) *ContextListItem {
    return models.NewContextListItem(id, cells, path, style, contextsFn, enter)
}

func newResourceGroupItem(deps Deps, gvr schema.GroupVersionResource, namespace, id string, cells []string, path []string, style *lipgloss.Style, watchable bool, enter func() (Folder, error)) *ResourceGroupItem {
    return models.NewResourceGroupItem(toModelsDeps(deps), gvr, namespace, id, cells, path, style, watchable, enter)
}

// Helper to convert navigation.Deps to models.Deps without copying the callbacks unnecessarily.
func toModelsDeps(d Deps) models.Deps {
    return models.Deps{
        Cl:          d.Cl,
        Ctx:         d.Ctx,
        CtxName:     d.CtxName,
        ListContexts: d.ListContexts,
        EnterContext: d.EnterContext,
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
