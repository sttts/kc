package models

import table "github.com/sttts/kc/internal/table"

// SecretKeysFolder lists the data keys for a Secret.
type SecretKeysFolder struct {
	*BaseFolder
	Namespace string
	Name      string
}

// NewSecretKeysFolder constructs the Secret data keys folder.
func NewSecretKeysFolder(deps Deps, parentPath []string, namespace, name string) *SecretKeysFolder {
	path := append(append([]string{}, parentPath...), "data")
	key := composeKey(deps, path)
	cols := []table.Column{{Title: " Name"}}
	base := NewBaseFolder(deps, cols, path, key, nil)
	return &SecretKeysFolder{BaseFolder: base, Namespace: namespace, Name: name}
}
