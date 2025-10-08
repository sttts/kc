package models

import (
	"context"
	"sort"

	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SecretKeysFolder lists the data keys for a Secret.
type SecretKeysFolder struct {
	*BaseFolder
	Namespace string
	Name      string
}

// NewSecretKeysFolder constructs the Secret data keys folder.
func NewSecretKeysFolder(deps Deps, parentPath []string, namespace, name string) *SecretKeysFolder {
	path := append(append([]string{}, parentPath...), "data")
	cols := []table.Column{{Title: " Name"}}
	base := NewBaseFolder(deps, cols, path)
	folder := &SecretKeysFolder{BaseFolder: base, Namespace: namespace, Name: name}
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	rows := newLiveKeyRowSource(deps, gvr, namespace, name, folder.buildRows, folder.BaseFolder.markDirtyFromSource)
	base.SetRowSource(rows)
	return folder
}

func (f *SecretKeysFolder) Parent() (schema.GroupVersionResource, string, string) {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, f.Namespace, f.Name
}

func (f *SecretKeysFolder) buildRows(ctx context.Context) ([]table.Row, error) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	obj, err := f.Deps.Cl.GetByGVR(ctx, gvr, f.Namespace, f.Name)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, nil
	}
	sec, err := decodeSecret(obj.Object)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(sec.Data))
	for k := range sec.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([]table.Row, 0, len(keys))
	style := WhiteStyle()
	for _, k := range keys {
		rowPath := append(append([]string{}, f.Path()...), k)
		item := NewSimpleItem(k, []string{k}, rowPath, style)
		item.WithViewContent(keyViewContent(f.Deps, gvr, f.Namespace, f.Name, k, true))
		rows = append(rows, item)
	}
	return rows, nil
}
