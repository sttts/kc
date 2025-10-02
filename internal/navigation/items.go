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
	return models.NewNamespaceItem(obj, func() (models.Folder, error) {
		f, err := enter()
		if err != nil {
			return nil, err
		}
		return unwrapFolder(f), nil
	})
}

func newObjectWithChildItem(obj *ObjectRow, enter func() (Folder, error)) *ObjectWithChildItem {
	return models.NewObjectWithChildItem(obj, func() (models.Folder, error) {
		f, err := enter()
		if err != nil {
			return nil, err
		}
		return unwrapFolder(f), nil
	})
}

func newPodItem(row *ObjectRow) *PodItem             { return models.NewPodItem(row) }
func newConfigMapItem(row *ObjectRow) *ConfigMapItem { return models.NewConfigMapItem(row) }
func newSecretItem(row *ObjectRow) *SecretItem       { return models.NewSecretItem(row) }
func newContextItem(id string, cells []string, path []string, style *lipgloss.Style, enter func() (Folder, error)) *ContextItem {
	return models.NewContextItem(id, cells, path, style, func() (models.Folder, error) {
		f, err := enter()
		if err != nil {
			return nil, err
		}
		return unwrapFolder(f), nil
	})
}

func newContextListItem(id string, cells []string, path []string, style *lipgloss.Style, contextsFn func() []string, enter func() (Folder, error)) *ContextListItem {
	return models.NewContextListItem(id, cells, path, style, contextsFn, func() (models.Folder, error) {
		f, err := enter()
		if err != nil {
			return nil, err
		}
		return unwrapFolder(f), nil
	})
}

func newResourceGroupItem(deps Deps, gvr schema.GroupVersionResource, namespace, id string, cells []string, path []string, style *lipgloss.Style, watchable bool, enter func() (Folder, error)) *ResourceGroupItem {
	return models.NewResourceGroupItem(toModelsDeps(deps), gvr, namespace, id, cells, path, style, watchable, func() (models.Folder, error) {
		f, err := enter()
		if err != nil {
			return nil, err
		}
		return unwrapFolder(f), nil
	})
}
