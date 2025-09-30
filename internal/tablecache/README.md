# Table Cache Design

## Motivation

Kubernetes APIs can return `Table` objects (`meta.k8s.io/v1`) whenever the client advertises the table MIME types
(`application/json;as=Table;g=meta.k8s.io;v=v1` or the protobuf variant). Tables include the condensed row data that many UIs
expect, but controller-runtime’s cache only understands the traditional List/Watch payloads. Until upstream support lands
(kubernetes/kubernetes#132926), we need a cache implementation that keeps controller-runtime ergonomics while reading the server’s
Table responses.

## What tablecache provides

`tablecache` is a drop-in replacement for controller-runtime’s cache:

- `tablecache.New(*rest.Config, tablecache.Options)` constructs a cache that serves `RowList`/`Row` objects using the server’s
  Table responses and delegates every other type to controller-runtime’s default cache.
- `tablecache.NewCacheFunc()` produces a `cache.NewCacheFunc` that can be plugged into wiring that expects the standard
  signature (e.g. `manager.Options.NewCache`).
- `Row` and `RowList` types mirror the server’s Table payload. They implement `client.Object` / `client.ObjectList`, so callers
  interact with them using the normal controller-runtime interfaces.
- A `Reader` adapter (`tablecache.Reader`) is still available for direct use when you want table semantics without the cache.

## Data model

```
type Row struct {
    metav1.TypeMeta
    metav1.ObjectMeta
    Columns []metav1.TableColumnDefinition
    metav1.TableRow
}

type RowList struct {
    metav1.TypeMeta
    metav1.ListMeta
    Columns []metav1.TableColumnDefinition
    Items   []Row
}
```

- `metav1.TableRow` carries the raw cells, object snippet, and row conditions straight from the server.
- `Columns` records the server’s column definitions so callers can interpret the values without custom logic.
- `ObjectMeta` is reconstructed from the embedded object (when available) so controller-runtime keys (`namespace/name`) work as
  expected.
- `Row`/`RowList` implement a `TableTarget()` accessor; callers must set the underlying GVK via `NewRowList(gvk)` or
  `Row.SetTableTarget(gvk)` before calling the cache so the adapter knows which resource to query.

## Request flow

1. The caller constructs a `RowList` via `tablecache.NewRowList(gvk)` (or sets the target GVK manually) and passes it to the cache
   `List`/`Get` call.
2. `tablecache` resolves the resource using the shared RESTMapper and issues list/watch/GET requests with the table `Accept`
   headers and `includeObject=Object` to pull the embedded metadata.
3. The server returns a `metav1.Table`; `tablecache` converts it into `RowList`/`Row` so downstream code receives typed objects
   without having to parse raw tables.
4. For non-table objects, the wrapper simply delegates to controller-runtime’s default cache implementation.

## Watch behaviour

When watching resources in table mode, the server may emit raw objects instead of table rows if it cannot fulfil the table request
(e.g. decode errors). `tablecache` falls back to the underlying object watch, rehydrates each event into a `Row`, and continues the
stream so consumers always see row-shaped objects. Built-in columns are preserved (`Name`, `Ready`, `Status`, `Restarts` for pods,
for example).

## Usage examples

```go
cfg := ctrl.GetConfigOrDie()
rowCache, err := tablecache.New(cfg, tablecache.Options{Options: cache.Options{Scheme: scheme}})
if err != nil {
    log.Fatal(err)
}

rows := tablecache.NewRowList(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
if err := rowCache.List(ctx, rows, client.InNamespace("default")); err != nil {
    log.Fatal(err)
}
for _, row := range rows.Items {
    fmt.Printf("%s: %v\n", row.Name, row.TableRow.Cells)
}
```

## Testing

Envtests under `tablecache` exercise two layers:

- `reader_envtest_test.go` drives the `Reader` directly against an envtest API server and asserts the canonical columns/values
  returned by Kubernetes (
  `Name`, `Ready`, `Status`, `Restarts` for pods).
- `cache_envtest_test.go` creates a standalone cache via `tablecache.New`, seeds real pods, and confirms both `List` and `Get`
  return `Row` objects through the cache API.

Running the package tests (`go test ./internal/tablecache`) spins up envtest automatically and verifies both paths.
