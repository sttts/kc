package ui

import (
    "context"
    "fmt"

    "github.com/sttts/kc/pkg/resources"
    "k8s.io/apimachinery/pkg/runtime/schema"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// NamespacesDataSource provides live listings for namespaces using a StoreProvider.
type NamespacesDataSource struct {
    store  resources.StoreProvider
    mapper func(schema.GroupVersionKind) (schema.GroupVersionResource, error)
    table  func(context.Context, schema.GroupVersionResource, string) (*metav1.Table, error)
}

func NewNamespacesDataSource(store resources.StoreProvider, mapper func(schema.GroupVersionKind) (schema.GroupVersionResource, error), table func(context.Context, schema.GroupVersionResource, string) (*metav1.Table, error)) *NamespacesDataSource {
    return &NamespacesDataSource{store: store, mapper: mapper, table: table}
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

// ListTable returns headers, rows and items when server-side Table is available for namespaces.
func (d *NamespacesDataSource) ListTable() ([]string, [][]string, []Item, error) {
    if d.store == nil || d.mapper == nil || d.table == nil {
        return nil, nil, nil, fmt.Errorf("table not available")
    }
    gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
    gvr, err := d.mapper(gvk)
    if err != nil { return nil, nil, nil, err }
    tbl, err := d.table(context.Background(), gvr, "")
    if err != nil || tbl == nil { return nil, nil, nil, fmt.Errorf("no table") }
    headers := make([]string, len(tbl.ColumnDefinitions))
    nameIdx := 0
    for i, col := range tbl.ColumnDefinitions {
        headers[i] = col.Name
        if col.Name == "Name" || col.Name == "NAME" || col.Name == "name" { nameIdx = i }
    }
    rows := make([][]string, 0, len(tbl.Rows))
    items := make([]Item, 0, len(tbl.Rows))
    for _, r := range tbl.Rows {
        cells := make([]string, len(r.Cells))
        for i := range r.Cells { cells[i] = fmt.Sprint(r.Cells[i]) }
        rows = append(rows, cells)
        name := ""
        if nameIdx < len(cells) { name = cells[nameIdx] } else if len(cells) > 0 { name = cells[0] }
        items = append(items, Item{Name: name, Type: ItemTypeNamespace})
    }
    return headers, rows, items, nil
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

// PodsDataSource provides live listings for pods in a namespace using a StoreProvider.
type PodsDataSource struct {
    store  resources.StoreProvider
    mapper func(schema.GroupVersionKind) (schema.GroupVersionResource, error)
}

func NewPodsDataSource(store resources.StoreProvider, mapper func(schema.GroupVersionKind) (schema.GroupVersionResource, error)) *PodsDataSource {
    return &PodsDataSource{store: store, mapper: mapper}
}

func (d *PodsDataSource) List(namespace string) ([]Item, error) {
    if d.store == nil || d.mapper == nil { return nil, fmt.Errorf("data source not initialized") }
    gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
    gvr, err := d.mapper(gvk)
    if err != nil { return nil, err }
    lst, err := d.store.Store().List(context.Background(), resources.StoreKey{GVR: gvr, Namespace: namespace})
    if err != nil { return nil, err }
    items := make([]Item, 0, len(lst.Items))
    for i := range lst.Items {
        pod := &lst.Items[i]
        items = append(items, Item{Name: pod.GetName(), Type: ItemTypeResource, GVK: "v1 Pod"})
    }
    return items, nil
}

func (d *PodsDataSource) Watch(ctx context.Context, namespace string) (<-chan resources.Event, context.CancelFunc, error) {
    if d.store == nil || d.mapper == nil { return nil, func(){}, fmt.Errorf("data source not initialized") }
    gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
    gvr, err := d.mapper(gvk)
    if err != nil { return nil, func(){}, err }
    return d.store.Store().Watch(ctx, resources.StoreKey{GVR: gvr, Namespace: namespace})
}

// GenericDataSource provides list/watch for any GVK.
type GenericDataSource struct {
    store  resources.StoreProvider
    mapper func(schema.GroupVersionKind) (schema.GroupVersionResource, error)
    gvk    schema.GroupVersionKind
    table  func(context.Context, schema.GroupVersionResource, string) (*metav1.Table, error)
}

func NewGenericDataSource(store resources.StoreProvider, mapper func(schema.GroupVersionKind) (schema.GroupVersionResource, error), table func(context.Context, schema.GroupVersionResource, string) (*metav1.Table, error), gvk schema.GroupVersionKind) *GenericDataSource {
    return &GenericDataSource{store: store, mapper: mapper, table: table, gvk: gvk}
}

func (d *GenericDataSource) List(namespace string) ([]Item, error) {
    if d.store == nil || d.mapper == nil { return nil, fmt.Errorf("data source not initialized") }
    gvr, err := d.mapper(d.gvk)
    if err != nil { return nil, err }
    // Prefer server-side Table if available
    if d.table != nil {
        if tbl, err := d.table(context.Background(), gvr, namespace); err == nil && tbl != nil {
            items := make([]Item, 0, len(tbl.Rows))
            nameIdx := 0
            for i, col := range tbl.ColumnDefinitions {
                if col.Name == "Name" || col.Name == "NAME" || col.Name == "name" {
                    nameIdx = i
                    break
                }
            }
            for _, row := range tbl.Rows {
                cells := row.Cells
                var name string
                if nameIdx < len(cells) {
                    name = fmt.Sprint(cells[nameIdx])
                } else if len(cells) > 0 {
                    name = fmt.Sprint(cells[0])
                }
                items = append(items, Item{Name: name, Type: ItemTypeResource, GVK: d.gvk.String()})
            }
            return items, nil
        }
    }
    lst, err := d.store.Store().List(context.Background(), resources.StoreKey{GVR: gvr, Namespace: namespace})
    if err != nil { return nil, err }
    items := make([]Item, 0, len(lst.Items))
    for i := range lst.Items {
        o := &lst.Items[i]
        items = append(items, Item{Name: o.GetName(), Type: ItemTypeResource, GVK: d.gvk.String()})
    }
    return items, nil
}

func (d *GenericDataSource) Watch(ctx context.Context, namespace string) (<-chan resources.Event, context.CancelFunc, error) {
    if d.store == nil || d.mapper == nil { return nil, func(){}, fmt.Errorf("data source not initialized") }
    gvr, err := d.mapper(d.gvk)
    if err != nil { return nil, func(){}, err }
    return d.store.Store().Watch(ctx, resources.StoreKey{GVR: gvr, Namespace: namespace})
}

func (d *GenericDataSource) Get(namespace, name string) (*unstructured.Unstructured, error) {
    if d.store == nil || d.mapper == nil { return nil, fmt.Errorf("data source not initialized") }
    gvr, err := d.mapper(d.gvk)
    if err != nil { return nil, err }
    return d.store.Store().Get(context.Background(), resources.StoreKey{GVR: gvr, Namespace: namespace}, name)
}

// ListTable returns server-side table headers/rows and items for selection when available.
func (d *GenericDataSource) ListTable(namespace string) ([]string, [][]string, []Item, error) {
    if d.store == nil || d.mapper == nil || d.table == nil { return nil, nil, nil, fmt.Errorf("table not available") }
    gvr, err := d.mapper(d.gvk)
    if err != nil { return nil, nil, nil, err }
    tbl, err := d.table(context.Background(), gvr, namespace)
    if err != nil || tbl == nil { return nil, nil, nil, fmt.Errorf("no table") }
    headers := make([]string, len(tbl.ColumnDefinitions))
    nameIdx := 0
    for i, col := range tbl.ColumnDefinitions {
        headers[i] = col.Name
        if col.Name == "Name" || col.Name == "NAME" || col.Name == "name" { nameIdx = i }
    }
    rows := make([][]string, 0, len(tbl.Rows))
    items := make([]Item, 0, len(tbl.Rows))
    for _, r := range tbl.Rows {
        cells := make([]string, len(r.Cells))
        for i := range r.Cells { cells[i] = fmt.Sprint(r.Cells[i]) }
        rows = append(rows, cells)
        name := ""
        if nameIdx < len(cells) { name = cells[nameIdx] } else if len(cells) > 0 { name = cells[0] }
        items = append(items, Item{Name: name, Type: ItemTypeResource, GVK: d.gvk.String()})
    }
    return headers, rows, items, nil
}
