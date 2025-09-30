package tableclient

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	tableAcceptHeader = "application/json;as=Table;g=meta.k8s.io;v=v1, application/json"
)

var tableOptions = &metav1.TableOptions{IncludeObject: metav1.IncludeObject}

// Reader decorates a controller-runtime reader so that table-aware lists and watches are materialized as Row objects.
type withWatch interface {
	client.Reader
	Watch(context.Context, client.ObjectList, ...client.ListOption) (watch.Interface, error)
}

type Reader struct {
	delegate withWatch
	mapper   meta.RESTMapper
	fetcher  tableFetcher
}

// NewReader constructs a table-aware reader using the provided REST config and scheme.
func NewReader(delegate withWatch, mapper meta.RESTMapper, cfg *rest.Config, scheme *runtime.Scheme) (*Reader, error) {
	if delegate == nil {
		return nil, fmt.Errorf("tableclient: delegate reader must not be nil")
	}
	if mapper == nil {
		return nil, fmt.Errorf("tableclient: rest mapper must not be nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("tableclient: rest config must not be nil")
	}
	if scheme == nil {
		return nil, fmt.Errorf("tableclient: scheme must not be nil")
	}
	fetcher, err := newRESTTableFetcher(cfg, scheme)
	if err != nil {
		return nil, err
	}
	return &Reader{delegate: delegate, mapper: mapper, fetcher: fetcher}, nil
}

// NewReaderWithFetcher is intended for tests, letting callers inject a custom table fetcher.
func NewReaderWithFetcher(delegate withWatch, mapper meta.RESTMapper, fetcher tableFetcher) *Reader {
	if delegate == nil {
		panic("tableclient: delegate reader must not be nil")
	}
	if mapper == nil {
		panic("tableclient: rest mapper must not be nil")
	}
	if fetcher == nil {
		panic("tableclient: table fetcher must not be nil")
	}
	return &Reader{delegate: delegate, mapper: mapper, fetcher: fetcher}
}

// Get delegates to the underlying reader.
func (r *Reader) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return r.delegate.Get(ctx, key, obj)
}

// List lists objects, materializing server-side tables into RowList instances when requested.
func (r *Reader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	targetGVK, rowList, err := targetFromList(list)
	if err != nil {
		if errors.Is(err, errUnsupportedTargetList) {
			return r.delegate.List(ctx, list, opts...)
		}
		return err
	}

	mapping, err := r.mapper.RESTMapping(targetGVK.GroupKind(), targetGVK.Version)
	if err != nil {
		return err
	}

	lo := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(lo)
	}

	listOpts := lo.AsListOptions()
	table, err := r.fetcher.ListTable(ctx, mapping, lo.Namespace, *listOpts.DeepCopy())
	if err != nil {
		return err
	}

	rows, err := convertTable(table, targetGVK)
	if err != nil {
		return err
	}

	rowList.SetTableTarget(targetGVK)
	copy := rows.DeepCopy()
	*rowList = *copy
	return nil
}

// Watch watches objects, converting table responses into Row watch events.
func (r *Reader) Watch(ctx context.Context, list client.ObjectList, opts ...client.ListOption) (watch.Interface, error) {
	targetGVK, _, err := targetFromList(list)
	if err != nil {
		if errors.Is(err, errUnsupportedTargetList) {
			return r.delegate.Watch(ctx, list, opts...)
		}
		return nil, err
	}

	mapping, err := r.mapper.RESTMapping(targetGVK.GroupKind(), targetGVK.Version)
	if err != nil {
		return nil, err
	}

	lo := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(lo)
	}

	listOpts := lo.AsListOptions()
	upstream, err := r.fetcher.WatchTable(ctx, mapping, lo.Namespace, *listOpts.DeepCopy())
	if err != nil {
		return nil, err
	}

	converter := func(evt watch.Event) ([]watch.Event, error) {
		if evt.Type == watch.Bookmark || evt.Type == watch.Error {
			return []watch.Event{evt}, nil
		}

		table, ok := evt.Object.(*metav1.Table)
		if !ok {
			return []watch.Event{evt}, nil
		}

		rows, err := convertTable(table, targetGVK)
		if err != nil {
			return nil, err
		}

		events := make([]watch.Event, 0, len(rows.Items))
		for i := range rows.Items {
			row := rows.Items[i].DeepCopy()
			row.SetTableTarget(targetGVK)
			events = append(events, watch.Event{Type: evt.Type, Object: row})
		}
		return events, nil
	}

	return newTableWatch(upstream, converter), nil
}

var errUnsupportedTargetList = errors.New("tableclient: list does not implement RowList")

func targetFromList(list client.ObjectList) (schema.GroupVersionKind, *RowList, error) {
	rowList, ok := list.(*RowList)
	if !ok {
		return schema.GroupVersionKind{}, nil, errUnsupportedTargetList
	}
	target := rowList.TableTarget()
	if target.Empty() {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("tableclient: list %T missing table target", list)
	}
	return target, rowList, nil
}

func convertTable(table *metav1.Table, target schema.GroupVersionKind) (*RowList, error) {
	if table == nil {
		return nil, fmt.Errorf("tableclient: nil table")
	}

	list := NewRowList(target)
	list.ListMeta = table.ListMeta
	if len(table.ColumnDefinitions) > 0 {
		list.Columns = append(list.Columns, table.ColumnDefinitions...)
	}

	for i := range table.Rows {
		row := NewRow(target)
		row.Columns = list.Columns
		table.Rows[i].DeepCopyInto(&row.TableRow)

		meta, err := extractMetadata(&table.Rows[i])
		if err != nil {
			return nil, err
		}
		row.ObjectMeta = meta
		list.Items = append(list.Items, *row)
	}

	return list, nil
}

func extractMetadata(row *metav1.TableRow) (metav1.ObjectMeta, error) {
	if row == nil {
		return metav1.ObjectMeta{}, fmt.Errorf("tableclient: nil row")
	}

	var runtimeObj runtime.Object
	switch {
	case row.Object.Object != nil:
		if obj, ok := row.Object.Object.(runtime.Object); ok {
			runtimeObj = obj
		}
	case len(row.Object.Raw) > 0:
		var u unstructured.Unstructured
		if err := u.UnmarshalJSON(row.Object.Raw); err != nil {
			return metav1.ObjectMeta{}, fmt.Errorf("tableclient: decode embedded object: %w", err)
		}
		runtimeObj = &u
	}

	if runtimeObj == nil {
		return metav1.ObjectMeta{}, nil
	}

	accessor, err := meta.Accessor(runtimeObj)
	if err != nil {
		return metav1.ObjectMeta{}, fmt.Errorf("tableclient: access embedded object metadata: %w", err)
	}

	meta := metav1.ObjectMeta{
		Name:            accessor.GetName(),
		Namespace:       accessor.GetNamespace(),
		UID:             accessor.GetUID(),
		ResourceVersion: accessor.GetResourceVersion(),
		Labels:          maps.Clone(accessor.GetLabels()),
		Annotations:     maps.Clone(accessor.GetAnnotations()),
		Finalizers:      slices.Clone(accessor.GetFinalizers()),
	}

	meta.SetCreationTimestamp(accessor.GetCreationTimestamp())
	meta.Generation = accessor.GetGeneration()
	meta.OwnerReferences = append([]metav1.OwnerReference(nil), accessor.GetOwnerReferences()...)
	if dt := accessor.GetDeletionTimestamp(); dt != nil {
		meta.DeletionTimestamp = dt.DeepCopy()
	}
	meta.ManagedFields = append([]metav1.ManagedFieldsEntry(nil), accessor.GetManagedFields()...)
	meta.Labels = ensureMap(meta.Labels)
	meta.Annotations = ensureMap(meta.Annotations)

	return meta, nil
}

func ensureMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	if len(in) == 0 {
		return map[string]string{}
	}
	return in
}

type tableFetcher interface {
	ListTable(ctx context.Context, mapping *meta.RESTMapping, namespace string, opts metav1.ListOptions) (*metav1.Table, error)
	WatchTable(ctx context.Context, mapping *meta.RESTMapping, namespace string, opts metav1.ListOptions) (watch.Interface, error)
}

type restTableFetcher struct {
	restClient rest.Interface
	paramCodec runtime.ParameterCodec
}

func newRESTTableFetcher(cfg *rest.Config, scheme *runtime.Scheme) (tableFetcher, error) {
	config := rest.CopyConfig(cfg)
	codec := serializer.NewCodecFactory(scheme)
	config.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: codec}
	config.ContentConfig = rest.ContentConfig{
		GroupVersion:         &schema.GroupVersion{Version: "v1"},
		NegotiatedSerializer: serializer.WithoutConversionCodecFactory{CodecFactory: codec},
	}
	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	restClient, err := rest.RESTClientFor(config)
	if err != nil {
		return nil, err
	}

	return &restTableFetcher{
		restClient: restClient,
		paramCodec: runtime.NewParameterCodec(scheme),
	}, nil
}

func (f *restTableFetcher) ListTable(ctx context.Context, mapping *meta.RESTMapping, namespace string, opts metav1.ListOptions) (*metav1.Table, error) {
	req := f.restClient.Get().Resource(mapping.Resource.Resource)
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace && namespace != "" {
		req = req.Namespace(namespace)
	}

	req.VersionedParams(tableOptions, f.paramCodec)
	req.VersionedParams(&opts, f.paramCodec)
	req.SetHeader("Accept", tableAcceptHeader)

	table := &metav1.Table{}
	if err := req.Do(ctx).Into(table); err != nil {
		return nil, err
	}
	return table, nil
}

func (f *restTableFetcher) WatchTable(ctx context.Context, mapping *meta.RESTMapping, namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	req := f.restClient.Get().Resource(mapping.Resource.Resource)
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace && namespace != "" {
		req = req.Namespace(namespace)
	}

	req.VersionedParams(tableOptions, f.paramCodec)
	req.VersionedParams(&opts, f.paramCodec)
	req.SetHeader("Accept", tableAcceptHeader)

	return req.Watch(ctx)
}

type tableWatch struct {
	upstream watch.Interface
	result   chan watch.Event
	stopOnce sync.Once
	stopCh   chan struct{}
}

func newTableWatch(upstream watch.Interface, convert func(watch.Event) ([]watch.Event, error)) watch.Interface {
	tw := &tableWatch{
		upstream: upstream,
		result:   make(chan watch.Event),
		stopCh:   make(chan struct{}),
	}

	go tw.stream(convert)
	return tw
}

func (w *tableWatch) stream(convert func(watch.Event) ([]watch.Event, error)) {
	defer close(w.result)
	defer w.upstream.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case evt, ok := <-w.upstream.ResultChan():
			if !ok {
				return
			}

			events, err := convert(evt)
			if err != nil {
				status := apierrors.NewInternalError(err).Status()
				w.forward(watch.Event{Type: watch.Error, Object: &status})
				continue
			}

			for _, e := range events {
				w.forward(e)
			}
		}
	}
}

func (w *tableWatch) forward(evt watch.Event) {
	select {
	case <-w.stopCh:
		return
	case w.result <- evt:
	}
}

// Stop signals the wrapper to terminate.
func (w *tableWatch) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
}

// ResultChan returns the downstream event stream.
func (w *tableWatch) ResultChan() <-chan watch.Event {
	return w.result
}
