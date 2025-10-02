package navigation

import (
	"strings"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sttts/kc/internal/navigation/models"
	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type folderWrapper struct {
	inner models.Folder
}

func newFolderWrapper(m models.Folder) *folderWrapper {
	if m == nil {
		return nil
	}
	return &folderWrapper{inner: m}
}

func (w *folderWrapper) innerFolder() models.Folder { return w.inner }

func (w *folderWrapper) Columns() []table.Column { return w.inner.Columns() }

func (w *folderWrapper) Title() string {
	path := w.inner.Path()
	if len(path) == 0 {
		return "/"
	}
	return strings.Join(path, "/")
}

func (w *folderWrapper) Key() string { return w.inner.Key() }

func (w *folderWrapper) ItemByID(id string) (Item, bool) {
	item, ok := w.inner.ItemByID(id)
	if !ok {
		return nil, false
	}
	if navItem, ok := item.(Item); ok {
		return navItem, true
	}
	return &itemWrapper{inner: item}, true
}

func (w *folderWrapper) Lines(top, num int) []table.Row        { return w.inner.Lines(top, num) }
func (w *folderWrapper) Above(id string, n int) []table.Row    { return w.inner.Above(id, n) }
func (w *folderWrapper) Below(id string, n int) []table.Row    { return w.inner.Below(id, n) }
func (w *folderWrapper) Len() int                              { return w.inner.Len() }
func (w *folderWrapper) Find(id string) (int, table.Row, bool) { return w.inner.Find(id) }

func (w *folderWrapper) ObjectListMeta() (schema.GroupVersionResource, string, bool) {
	if prov, ok := w.inner.(interface {
		ObjectListMeta() (schema.GroupVersionResource, string, bool)
	}); ok {
		return prov.ObjectListMeta()
	}
	return schema.GroupVersionResource{}, "", false
}

func (w *folderWrapper) Parent() (schema.GroupVersionResource, string, string) {
	if prov, ok := w.inner.(interface {
		Parent() (schema.GroupVersionResource, string, string)
	}); ok {
		return prov.Parent()
	}
	return schema.GroupVersionResource{}, "", ""
}

func (w *folderWrapper) Refresh() {
	if refresher, ok := w.inner.(interface{ Refresh() }); ok {
		refresher.Refresh()
	}
}

func (w *folderWrapper) IsDirty() bool {
	if dirty, ok := w.inner.(interface{ IsDirty() bool }); ok {
		return dirty.IsDirty()
	}
	return false
}

// itemWrapper bridges a models.Item that may not directly satisfy navigation.Item.
type itemWrapper struct {
	inner models.Item
}

func (i *itemWrapper) Columns() (string, []string, []*lipgloss.Style, bool) { return i.inner.Columns() }
func (i *itemWrapper) Details() string                                      { return i.inner.Details() }
func (i *itemWrapper) Path() []string                                       { return i.inner.Path() }

func wrapFolder(m models.Folder) Folder {
	if m == nil {
		return nil
	}
	return &folderWrapper{inner: m}
}

func unwrapFolder(f Folder) models.Folder {
	switch v := f.(type) {
	case *folderWrapper:
		return v.inner
	case interface{ innerFolder() models.Folder }:
		return v.innerFolder()
	}
	return nil
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
			f, err := d.EnterContext(name, basePath)
			if err != nil {
				return nil, err
			}
			return unwrapFolder(f), nil
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
			mf, err := d.EnterContext(name, basePath)
			if err != nil {
				return nil, err
			}
			return wrapFolder(mf), nil
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
