package models

import (
	"sort"

	table "github.com/sttts/kc/internal/table"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// ContextsFolder lists available kubeconfig contexts (if provided).
type ContextsFolder struct {
	*BaseFolder
	enter func(name string, basePath []string) (Folder, error)
}

// NewContextsFolder constructs the contexts folder with default single-column layout.
func NewContextsFolder(deps Deps, enter func(name string, basePath []string) (Folder, error)) *ContextsFolder {
	path := []string{"contexts"}
	cols := []table.Column{{Title: " Name"}}
	base := NewBaseFolder(deps, cols, path)
	folder := &ContextsFolder{BaseFolder: base, enter: enter}
	base.SetPopulate(folder.populate)
	return folder
}

func (f *ContextsFolder) populate() ([]table.Row, error) {
	rows := make([]table.Row, 0, 16)
	cfg := f.Deps.KubeConfig
	if len(cfg.Contexts) == 0 {
		return rows, nil
	}
	names := contextNames(cfg)
	nameStyle := WhiteStyle()
	for _, name := range names {
		itemPath := append(append([]string{}, f.Path()...), name)
		var enter func() (Folder, error)
		if f.enter != nil {
			nameCopy := name
			enter = func() (Folder, error) {
				return f.enter(nameCopy, itemPath)
			}
		}
		item := NewContextItem(name, []string{name}, itemPath, nameStyle, enter)
		rows = append(rows, item)
	}
	return rows, nil
}

func contextNames(cfg clientcmdapi.Config) []string {
	names := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
