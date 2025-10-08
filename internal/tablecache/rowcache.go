package tablecache

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type rowCache struct {
	cache.Cache
	mapper  meta.RESTMapper
	fetcher tableFetcher
}

func newRowCache(base cache.Cache, mapper meta.RESTMapper, fetcher tableFetcher) cache.Cache {
	return &rowCache{Cache: base, mapper: mapper, fetcher: fetcher}
}

func (c *rowCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	rowList, ok := list.(*RowList)
	if !ok {
		return c.Cache.List(ctx, list, opts...)
	}

	target := rowList.TableTarget()
	if target.Empty() {
		return fmt.Errorf("tablecache: RowList missing TableTarget")
	}

	mapping, err := c.mapper.RESTMapping(target.GroupKind(), target.Version)
	if err != nil {
		return err
	}

	lo := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(lo)
	}

	listOpts := lo.AsListOptions()
	table, err := c.fetcher.ListTable(ctx, mapping, lo.Namespace, *listOpts.DeepCopy())
	if err != nil {
		return err
	}

	rows, err := convertTable(table, target)
	if err != nil {
		return err
	}

	rowList.SetTableTarget(target)
	copy := rows.DeepCopy()
	*rowList = *copy
	return nil
}

func (c *rowCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	row, ok := obj.(*Row)
	if !ok {
		return c.Cache.Get(ctx, key, obj, opts...)
	}

	target := row.TableTarget()
	if target.Empty() {
		return fmt.Errorf("tablecache: Row missing TableTarget")
	}

	mapping, err := c.mapper.RESTMapping(target.GroupKind(), target.Version)
	if err != nil {
		return err
	}

	table, err := c.fetcher.GetTable(ctx, mapping, key.Namespace, key.Name)
	if err != nil {
		return err
	}

	rows, err := convertTable(table, target)
	if err != nil {
		return err
	}

	for i := range rows.Items {
		if rows.Items[i].Name == key.Name {
			*row = *rows.Items[i].DeepCopy()
			row.SetNamespace(key.Namespace)
			row.SetTableTarget(target)
			return nil
		}
	}

	return client.IgnoreNotFound(fmt.Errorf("tablecache: row %s/%s not found", key.Namespace, key.Name))
}

func (c *rowCache) GetInformer(ctx context.Context, obj client.Object, opts ...cache.InformerGetOption) (cache.Informer, error) {
	if _, ok := obj.(*Row); ok {
		return nil, fmt.Errorf("tablecache: informers not supported for Row")
	}
	return c.Cache.GetInformer(ctx, obj, opts...)
}

func (c *rowCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, opts ...cache.InformerGetOption) (cache.Informer, error) {
	if gvk.Group == SchemeGroupVersion.Group && (gvk.Kind == RowKind || gvk.Kind == RowListKind) {
		return nil, fmt.Errorf("tablecache: informers not supported for Row types")
	}
	return c.Cache.GetInformerForKind(ctx, gvk, opts...)
}

func (c *rowCache) IndexField(ctx context.Context, obj client.Object, field string, extract client.IndexerFunc) error {
	if _, ok := obj.(*Row); ok {
		return fmt.Errorf("tablecache: indexing not supported for Row")
	}
	return c.Cache.IndexField(ctx, obj, field, extract)
}
