package tablecache

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
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
	tableListAcceptHeader  = "application/json;as=Table;g=meta.k8s.io;v=v1, application/json"
	tableWatchAcceptHeader = "application/json;as=Table;g=meta.k8s.io;v=v1;watch=true, application/json"
)

var tableOptions = &metav1.TableOptions{IncludeObject: metav1.IncludeObject}

var errRequireFallback = errors.New("tablecache: require fallback watch")

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
		return nil, fmt.Errorf("tablecache: delegate reader must not be nil")
	}
	if mapper == nil {
		return nil, fmt.Errorf("tablecache: rest mapper must not be nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("tablecache: rest config must not be nil")
	}
	if scheme == nil {
		return nil, fmt.Errorf("tablecache: scheme must not be nil")
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
		panic("tablecache: delegate reader must not be nil")
	}
	if mapper == nil {
		panic("tablecache: rest mapper must not be nil")
	}
	if fetcher == nil {
		panic("tablecache: table fetcher must not be nil")
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
		if evt.Type == watch.Bookmark {
			return []watch.Event{evt}, nil
		}
		if evt.Type == watch.Error {
			return nil, errRequireFallback
		}

		table, ok := evt.Object.(*metav1.Table)
		if !ok {
			return nil, errRequireFallback
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

	fallback := func() (watch.Interface, func(watch.Event) ([]watch.Event, error), error) {
		listGVK := schema.GroupVersionKind{Group: targetGVK.Group, Version: targetGVK.Version, Kind: targetGVK.Kind + "List"}
		listObj := &unstructured.UnstructuredList{}
		listObj.SetGroupVersionKind(listGVK)
		watchOpts := make([]client.ListOption, len(opts))
		copy(watchOpts, opts)
		fallbackWatch, err := r.delegate.Watch(ctx, listObj, watchOpts...)
		if err != nil {
			return nil, nil, err
		}
		return fallbackWatch, r.objectEventConverter(ctx, mapping, targetGVK), nil
	}

	return newTableWatch(upstream, converter, fallback), nil
}

var errUnsupportedTargetList = errors.New("tablecache: list does not implement RowList")

func targetFromList(list client.ObjectList) (schema.GroupVersionKind, *RowList, error) {
	rowList, ok := list.(*RowList)
	if !ok {
		return schema.GroupVersionKind{}, nil, errUnsupportedTargetList
	}
	target := rowList.TableTarget()
	if target.Empty() {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("tablecache: list %T missing table target", list)
	}
	return target, rowList, nil
}

func convertTable(table *metav1.Table, target schema.GroupVersionKind) (*RowList, error) {
	if table == nil {
		return nil, fmt.Errorf("tablecache: nil table")
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

var fallbackColumns = []metav1.TableColumnDefinition{
	{Name: "Name", Type: "string"},
	{Name: "Namespace", Type: "string"},
}

func (r *Reader) objectEventConverter(ctx context.Context, mapping *meta.RESTMapping, target schema.GroupVersionKind) func(watch.Event) ([]watch.Event, error) {
	return func(evt watch.Event) ([]watch.Event, error) {
		if evt.Type == watch.Bookmark {
			return []watch.Event{evt}, nil
		}
		if evt.Type == watch.Error {
			status, _ := evt.Object.(*metav1.Status)
			if isWatchDecodeError(status) {
				return nil, nil
			}
			return []watch.Event{evt}, nil
		}

		obj, ok := evt.Object.(runtime.Object)
		if !ok {
			return []watch.Event{evt}, nil
		}

		accessor, err := meta.Accessor(obj)
		if err != nil {
			return nil, err
		}

		if evt.Type != watch.Deleted {
			table, err := r.fetcher.GetTable(ctx, mapping, accessor.GetNamespace(), accessor.GetName())
			if err == nil && table != nil && len(table.Rows) > 0 {
				rows, err := convertTable(table, target)
				if err == nil && len(rows.Items) > 0 {
					row := rows.Items[0].DeepCopy()
					row.SetTableTarget(target)
					return []watch.Event{{Type: evt.Type, Object: row}}, nil
				}
			}
		}

		row := NewRow(target)
		row.ObjectMeta = metav1.ObjectMeta{
			Name:            accessor.GetName(),
			Namespace:       accessor.GetNamespace(),
			UID:             accessor.GetUID(),
			ResourceVersion: accessor.GetResourceVersion(),
		}
		row.SetTableTarget(target)
		row.Columns = make([]metav1.TableColumnDefinition, len(fallbackColumns))
		copy(row.Columns, fallbackColumns)
		row.TableRow = metav1.TableRow{
			Cells:  []interface{}{row.Name, row.Namespace},
			Object: runtime.RawExtension{Object: obj.DeepCopyObject()},
		}

		return []watch.Event{{Type: evt.Type, Object: row}}, nil
	}
}

func isWatchDecodeError(status *metav1.Status) bool {
	if status == nil {
		return false
	}
	if strings.Contains(status.Message, "unable to decode an event from the watch stream") {
		return true
	}
	if status.Details != nil {
		for _, cause := range status.Details.Causes {
			if cause.Type == metav1.CauseTypeUnexpectedServerResponse || string(cause.Type) == "ClientWatchDecoding" {
				return true
			}
		}
	}
	return false
}

func extractMetadata(row *metav1.TableRow) (metav1.ObjectMeta, error) {
	if row == nil {
		return metav1.ObjectMeta{}, fmt.Errorf("tablecache: nil row")
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
			return metav1.ObjectMeta{}, fmt.Errorf("tablecache: decode embedded object: %w", err)
		}
		runtimeObj = &u
	}

	if runtimeObj == nil {
		return metav1.ObjectMeta{}, nil
	}

	accessor, err := meta.Accessor(runtimeObj)
	if err != nil {
		return metav1.ObjectMeta{}, fmt.Errorf("tablecache: access embedded object metadata: %w", err)
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
	GetTable(ctx context.Context, mapping *meta.RESTMapping, namespace, name string) (*metav1.Table, error)
}

type restTableFetcher struct {
	baseConfig *rest.Config
	httpClient *http.Client
	codec      serializer.CodecFactory
	paramCodec runtime.ParameterCodec
	clients    sync.Map // key: schema.GroupVersion.String() -> rest.Interface
}

func newRESTTableFetcher(cfg *rest.Config, scheme *runtime.Scheme) (tableFetcher, error) {
	config := rest.CopyConfig(cfg)
	codec := serializer.NewCodecFactory(scheme)
	config.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: codec}
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, err
	}
	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return &restTableFetcher{
		baseConfig: config,
		httpClient: httpClient,
		codec:      codec,
		paramCodec: metav1.ParameterCodec,
	}, nil
}

func (f *restTableFetcher) clientFor(mapping *meta.RESTMapping) (rest.Interface, error) {
	gv := mapping.GroupVersionKind.GroupVersion()
	key := gv.String()
	if client, ok := f.clients.Load(key); ok {
		return client.(rest.Interface), nil
	}

	cfg := rest.CopyConfig(f.baseConfig)
	cfg.GroupVersion = &gv
	if gv.Group == "" {
		cfg.APIPath = "/api"
	} else {
		cfg.APIPath = "/apis"
	}
	cfg.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: f.codec}

	restClient, err := rest.RESTClientForConfigAndClient(cfg, f.httpClient)
	if err != nil {
		return nil, err
	}

	f.clients.Store(key, restClient)
	return restClient, nil
}

func (f *restTableFetcher) ListTable(ctx context.Context, mapping *meta.RESTMapping, namespace string, opts metav1.ListOptions) (*metav1.Table, error) {
	client, err := f.clientFor(mapping)
	if err != nil {
		return nil, err
	}
	req := client.Get().Resource(mapping.Resource.Resource)
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace && namespace != "" {
		req = req.Namespace(namespace)
	}

	req.Param("includeObject", string(metav1.IncludeObject))
	req.SpecificallyVersionedParams(&opts, f.paramCodec, metav1.SchemeGroupVersion)
	req.SetHeader("Accept", tableListAcceptHeader)

	table := &metav1.Table{}
	if err := req.Do(ctx).Into(table); err != nil {
		return nil, err
	}
	return table, nil
}

func (f *restTableFetcher) WatchTable(ctx context.Context, mapping *meta.RESTMapping, namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	client, err := f.clientFor(mapping)
	if err != nil {
		return nil, err
	}
	req := client.Get().Resource(mapping.Resource.Resource)
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace && namespace != "" {
		req = req.Namespace(namespace)
	}

	req.Param("includeObject", string(metav1.IncludeObject))
	req.SpecificallyVersionedParams(&opts, f.paramCodec, metav1.SchemeGroupVersion)
	req.SetHeader("Accept", tableWatchAcceptHeader)

	return req.Watch(ctx)
}

func (f *restTableFetcher) GetTable(ctx context.Context, mapping *meta.RESTMapping, namespace, name string) (*metav1.Table, error) {
	client, err := f.clientFor(mapping)
	if err != nil {
		return nil, err
	}
	req := client.Get().Resource(mapping.Resource.Resource)
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace && namespace != "" {
		req = req.Namespace(namespace)
	}
	req = req.Name(name)
	req.Param("includeObject", string(metav1.IncludeObject))
	req.SetHeader("Accept", tableListAcceptHeader)

	table := &metav1.Table{}
	if err := req.Do(ctx).Into(table); err != nil {
		return nil, err
	}
	return table, nil
}

type tableWatch struct {
	source       watch.Interface
	convert      func(watch.Event) ([]watch.Event, error)
	fallback     func() (watch.Interface, func(watch.Event) ([]watch.Event, error), error)
	fallbackUsed bool
	result       chan watch.Event
	stopOnce     sync.Once
	stopCh       chan struct{}
}

func newTableWatch(upstream watch.Interface, convert func(watch.Event) ([]watch.Event, error), fallback func() (watch.Interface, func(watch.Event) ([]watch.Event, error), error)) watch.Interface {
	tw := &tableWatch{
		source:   upstream,
		convert:  convert,
		fallback: fallback,
		result:   make(chan watch.Event),
		stopCh:   make(chan struct{}),
	}

	go tw.stream()
	return tw
}

func (w *tableWatch) stream() {
	defer close(w.result)
	defer w.source.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case evt, ok := <-w.source.ResultChan():
			if !ok {
				if w.fallback != nil && !w.fallbackUsed {
					newSource, newConvert, err := w.fallback()
					if err != nil {
						status := apierrors.NewInternalError(err).Status()
						w.forward(watch.Event{Type: watch.Error, Object: &status})
						return
					}
					w.source = newSource
					w.convert = newConvert
					w.fallbackUsed = true
					continue
				}
				return
			}

			events, err := w.convert(evt)
			if errors.Is(err, errRequireFallback) {
				if w.fallback == nil {
					continue
				}
				newSource, newConvert, fbErr := w.fallback()
				if fbErr != nil {
					status := apierrors.NewInternalError(fbErr).Status()
					w.forward(watch.Event{Type: watch.Error, Object: &status})
					return
				}
				w.source.Stop()
				w.source = newSource
				w.convert = newConvert
				w.fallbackUsed = true
				continue
			}
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
		w.source.Stop()
	})
}

// ResultChan returns the downstream event stream.
func (w *tableWatch) ResultChan() <-chan watch.Event {
	return w.result
}
