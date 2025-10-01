package models

import table "github.com/sttts/kc/internal/table"

// ConfigMapKeysFolder lists the data keys for a ConfigMap.
type ConfigMapKeysFolder struct {
	*BaseFolder
	Namespace string
	Name      string
}

// NewConfigMapKeysFolder constructs the ConfigMap data keys folder.
func NewConfigMapKeysFolder(deps Deps, parentPath []string, namespace, name string) *ConfigMapKeysFolder {
	path := append(append([]string{}, parentPath...), "data")
	key := composeKey(deps, path)
	cols := []table.Column{{Title: " Name"}}
	base := NewBaseFolder(deps, cols, path, key, nil)
	return &ConfigMapKeysFolder{BaseFolder: base, Namespace: namespace, Name: name}
}
