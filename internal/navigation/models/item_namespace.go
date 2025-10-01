package models

import "github.com/charmbracelet/lipgloss/v2"

// NamespaceItem embeds an ObjectRow and adds Enter support.
type NamespaceItem struct {
	*ObjectRow
	enter func() (Folder, error)
}

func NewNamespaceItem(obj *ObjectRow, enter func() (Folder, error)) *NamespaceItem {
	return &NamespaceItem{ObjectRow: obj, enter: enter}
}

func (n *NamespaceItem) Enter() (Folder, error) {
	if n.enter == nil {
		return nil, nil
	}
	return n.enter()
}

// ObjectWithChildItem embeds an object row and adds Enter support for child folders.
type ObjectWithChildItem struct {
	*ObjectRow
	enter func() (Folder, error)
}

func NewObjectWithChildItem(obj *ObjectRow, enter func() (Folder, error)) *ObjectWithChildItem {
	return &ObjectWithChildItem{ObjectRow: obj, enter: enter}
}

func (o *ObjectWithChildItem) Enter() (Folder, error) {
	if o.enter == nil {
		return nil, nil
	}
	return o.enter()
}

// PodItem, ConfigMapItem, SecretItem are viewable object rows without extra behaviour.
type PodItem struct{ *ObjectRow }
type ConfigMapItem struct{ *ObjectRow }
type SecretItem struct{ *ObjectRow }

// NewPodItem creates a PodItem with optional view content.
func NewPodItem(row *ObjectRow) *PodItem { return &PodItem{ObjectRow: row} }

func NewConfigMapItem(row *ObjectRow) *ConfigMapItem { return &ConfigMapItem{ObjectRow: row} }
func NewSecretItem(row *ObjectRow) *SecretItem       { return &SecretItem{ObjectRow: row} }
