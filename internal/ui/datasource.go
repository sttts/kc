package ui

import (
    "context"
    "fmt"

    "github.com/sschimanski/kc/pkg/resources"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

// NamespacesDataSource provides live listings for namespaces using a StoreProvider.
type NamespacesDataSource struct {
    store  resources.StoreProvider
    mapper func(schema.GroupVersionKind) (schema.GroupVersionResource, error)
}

func NewNamespacesDataSource(store resources.StoreProvider, mapper func(schema.GroupVersionKind) (schema.GroupVersionResource, error)) *NamespacesDataSource {
    return &NamespacesDataSource{store: store, mapper: mapper}
}

// List returns items for namespaces at "/namespaces".
func (d *NamespacesDataSource) List() ([]Item, error) {
    if d.store == nil || d.mapper == nil {
        return nil, fmt.Errorf("data source not initialized")
    }
    gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
    gvr, err := d.mapper(gvk)
    if err != nil {
        return nil, err
    }
    lst, err := d.store.Store().List(context.Background(), resources.StoreKey{GVR: gvr, Namespace: ""})
    if err != nil {
        return nil, err
    }
    items := make([]Item, 0, len(lst.Items))
    for i := range lst.Items {
        ns := &lst.Items[i]
        items = append(items, Item{
            Name:     ns.GetName(),
            Type:     ItemTypeNamespace,
            GVK:      "v1 Namespace",
            Selected: false,
        })
    }
    return items, nil
}

// Watch opens a watch channel for namespace events and returns it with a cancel func.
func (d *NamespacesDataSource) Watch(ctx context.Context) (<-chan resources.Event, context.CancelFunc, error) {
    if d.store == nil || d.mapper == nil {
        return nil, func(){}, fmt.Errorf("data source not initialized")
    }
    gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
    gvr, err := d.mapper(gvk)
    if err != nil { return nil, func(){}, err }
    ch, cancel, err := d.store.Store().Watch(ctx, resources.StoreKey{GVR: gvr, Namespace: ""})
    if err != nil { return nil, func(){}, err }
    return ch, cancel, nil
}
