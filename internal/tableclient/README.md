# Table Client Design

## Motivation

Kubernetes servers are able to return `Table` objects (`meta.k8s.io/v1`) when the client advertises the table MIME types
(`application/json;as=Table;g=meta.k8s.io;v=v1` or protobuf variant). Tables contain condensed rows that are ideal for driving a
TUI, but controller-runtime does not support them: its cache layer (reflectors) only understands traditional Kubernetes
`List` / `Watch` payloads. The upstream issue (kubernetes/kubernetes#132926) tracks first-class support, but we need a local
workaround to consume table data while continuing to use controller-runtime’s manager, cache, and reconcilers.

## High-level idea

Wrap a `controller-runtime` client and cache so that `List`/`Watch` requests negotiate the table format with the API server, then
expand the returned tables into synthetic list/watch objects that the rest of controller-runtime can digest. Downstream
controllers still receive typed objects (`client.Object`) whose payload is the original table row.

Key points:

- Never fork controller-runtime; instead adapt the transport and decoding boundaries.
- Keep the external surface compatible with `client.WithWatch` so informers, caches, and reconcilers work unchanged.
- Expose a typed row object (`tableclient.Row`) and list (`tableclient.RowList`) so downstream code can access table metadata plus
  the original columns.
- Derive object identity (`namespace/name`, resource GVR) from table rows and feed it into the synthetic objects so
  controller-runtime caches maintain correct keys and deduplicate updates.

## Data model

`meta/v1.Table` rows look like:

```
type TableRow struct {
    Cells    []interface{}
    Object   runtime.RawExtension // optional
    //... Name, Namespace, etc. live in row.Object if includeObject=Object
}
```

We materialize each row as a Kubernetes object implementing `client.Object`, but we embed upstream types instead of inventing new
ones:

```
type Row struct {
    metav1.TypeMeta
    metav1.ObjectMeta
    Columns []metav1.TableColumnDefinition
    metav1.TableRow
}
```

- `TableRow` carries the cells, nested object, and condition metadata verbatim from upstream.
- `Columns` reuses `[]metav1.TableColumnDefinition` so we keep the column schema identical to the server response.
- `TypeMeta` uses `GroupVersionKind{Group: "table.kc.dev", Version: "v1alpha1", Kind: "Row"}` (configurable) and is registered
  in the scheme.
- `ObjectMeta.Name` / `Namespace` are copied from the row when present; if the API server omits them we reconstruct them from the
  embedded object’s metadata and cache the values on the row for downstream readers.

`RowList` mirrors Kubernetes list conventions, embeds `metav1.TypeMeta` + `metav1.ListMeta`, and stores items as `[]Row`. The list
metadata is copied straight from the table response so Continue tokens and counts behave normally.
Rows and RowLists carry the target resource identity through a `TableTarget` interface. Callers set the
target `GroupVersionKind` (e.g., via `tableclient.NewRowList(gvk)`) before invoking `List`/`Watch`, letting the
wrapper resolve the real REST mapping while still returning table-shaped objects.


## Request path

1. Retrieve the target GVK from the `TableTarget` list (set by the caller) and resolve the matching GVR via the
   shared RESTMapper owned by the manager.
2. Use a dedicated REST client (`rest.Interface`) built from the manager’s `rest.Config`, but override the `Accept` header to
   prefer table formats and set `includeObject=Object` so rows embed the original object.
3. Send list/watch requests through that REST client instead of the default cached reader; decode into `*metav1.Table`.

MIME negotiation string:

```
Accept: application/json;as=Table;g=meta.k8s.io;v=v1, application/json
```

We keep the standard JSON fallback so non-table endpoints continue to work.

## List flow

```
List(ctx, list, opts...)
  -> resolve GVR + namespace + label/field selectors
  -> rest.Interface.Get().Namespace(ns).Resource(resource).VersionedParams(opts, scheme.ParameterCodec)
  -> set Accept header + includeObject=Object
  -> decode body into metav1.Table
  -> expand table rows into RowList
  -> assign the resulting list into the provided client.ObjectList using apimeta.Accessor
```

Expansion rules:

- For each row create a `Row` object, populating `TypeMeta`, `ObjectMeta`, reusing the upstream `metav1.TableRow`, and copying the
  shared column definitions.
- Set `Row.TableRow.Object` from `row.Object` for callers that want the raw object.
- Compute `Row.ResourceVersion` from the embedded object if present, otherwise from table metadata.
- Populate list metadata (`Continue`, `RemainingItemCount`, `ResourceVersion`) from the table.

## Watch flow

Controller-runtime watchers expect a `watch.Interface` that yields `watch.Event` containing the target runtime object. Our
wrapper:

1. Calls the REST watch with table MIME types and `includeObject=Object`.
2. Wraps the returned `watch.Interface` with a converter that, for each `watch.Event`,
   - decodes the payload as `*metav1.Table` (one table per event), and then
   - emits **one event per row**:
     - Create a `Row` object as in the list path.
     - Derive the event type: table watch streams include `TableRow.Conditions` to mark deletion; otherwise fall back to
       `row.Object` metadata (`DeletionTimestamp`) or use the table’s event type.

We buffer multi-row tables by pushing them onto a FIFO so downstream consumers keep receiving single-row events. This preserves
controller-runtime’s expectation that each event references exactly one object instance.

## Cache integration

- Inject `tableclient.Client` into `manager.Options.ClientBuilder` so the manager uses it for both direct reads and cache
  population.
- Provide a custom `NewCacheFunc` that creates controller-runtime’s cache with our client’s `Reader` so
  List/Watch calls go through the table wrapper.
  the table wrapper.
- Register `Row` and `RowList` types in the manager scheme before starting the manager.
- For reconcilers that want the original Kubernetes object, expose a helper to decode `Row.TableRow.Object` back into the typed
  struct using the manager’s scheme.

## Error handling & fallbacks

- If the API server returns a non-table response (because the resource does not support Table), the decoder falls back to the
  standard JSON list/watch and short-circuits the wrapping logic.
- Translate HTTP status codes to controller-runtime errors using `apierrors.FromObject`.
- Preserve `resourceVersion` semantics: lists expose the table’s `ResourceVersion`, watches start from `options.ResourceVersion`
  and honor bookmark events.

## Testing strategy

- Unit tests for the converter (Table → RowList, Table → watch events) covering multiple rows, empty tables, missing metadata, and
  cluster-scoped resources.
- Tests for includeObject=Object vs. omitted object to make sure fallbacks work and metadata reconstruction stays consistent.
- Fake REST client tests to ensure Accept headers and query params are set correctly.
- Integration test using envtest that spins up a fake API server, seeds data, and verifies controller-runtime cache receives `Row`
  objects via our client.

## Open questions

- Columns at runtime: should we normalize columns to canonical field names (e.g., use JSONPath) so downstream UI components can
  rely on them?
- Performance: splitting tables into per-row events might stress the cache under very wide tables. Should we batch events and
  expose a custom informer instead?
- API design: do we keep the raw object on every row or allow callers to opt in (to reduce memory)?

## Next steps

1. Implement `Row`/`RowList` types and register them in the scheme.
2. Build the REST wrapper that negotiates table MIME types.
3. Write the Table → Row converter and wrap list/watch paths.
4. Add unit tests plus an envtest integration case.
5. Document how controllers consume `Row` objects and recover the original object when needed.
