package models

import (
	"sort"

	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ConfigMapKeysFolder lists the data keys for a ConfigMap.
type ConfigMapKeysFolder struct {
	*BaseFolder
	Namespace string
	Name      string
}

// NewConfigMapKeysFolder constructs the ConfigMap data keys folder.
func NewConfigMapKeysFolder(deps Deps, parentPath []string, namespace, name string) *ConfigMapKeysFolder {
	path := append(append([]string{}, parentPath...), "data")
	cols := []table.Column{{Title: " Name"}}
	base := NewBaseFolder(deps, cols, path)
	folder := &ConfigMapKeysFolder{BaseFolder: base, Namespace: namespace, Name: name}
	base.SetPopulate(folder.populate)
	return folder
}

func (f *ConfigMapKeysFolder) Parent() (schema.GroupVersionResource, string, string) {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, f.Namespace, f.Name
}

func (f *ConfigMapKeysFolder) populate() ([]table.Row, error) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	obj, err := f.Deps.Cl.GetByGVR(f.Deps.Ctx, gvr, f.Namespace, f.Name)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, nil
	}
	cm, err := decodeConfigMap(obj.Object)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(cm.Data))
	for k := range cm.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([]table.Row, 0, len(keys))
	style := WhiteStyle()
	for _, k := range keys {
		rowPath := append(append([]string{}, f.Path()...), k)
		item := NewSimpleItem(k, []string{k}, rowPath, style)
		item.WithViewContent(keyViewContent(f.Deps, gvr, f.Namespace, f.Name, k, false))
		rows = append(rows, item)
	}
	return rows, nil
}
