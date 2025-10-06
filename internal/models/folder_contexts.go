package models

import (
	"sort"

	table "github.com/sttts/kc/internal/table"
)

// ContextsFolder lists available kubeconfig contexts (if provided).
type ContextsFolder struct {
	*BaseFolder
}

// NewContextsFolder constructs the contexts folder with default single-column layout.
func NewContextsFolder(deps Deps) *ContextsFolder {
	path := []string{"contexts"}
	cols := []table.Column{{Title: " Name"}}
	base := NewBaseFolder(deps, cols, path, nil)
	folder := &ContextsFolder{BaseFolder: base}
	base.SetPopulate(folder.populate)
	return folder
}

func (f *ContextsFolder) populate(*BaseFolder) ([]table.Row, error) {
	rows := make([]table.Row, 0, 16)
	if f.Deps.ListContexts == nil {
		return rows, nil
	}
	names := append([]string(nil), f.Deps.ListContexts()...)
	sort.Strings(names)
	nameStyle := WhiteStyle()
	for _, name := range names {
		itemPath := append(append([]string{}, f.Path()...), name)
		name := name
		var enter func() (Folder, error)
		if f.Deps.EnterContext != nil {
			enter = func() (Folder, error) {
				return f.Deps.EnterContext(name, itemPath)
			}
		}
		item := NewContextItem(name, []string{name}, itemPath, nameStyle, enter)
		rows = append(rows, item)
	}
	return rows, nil
}
