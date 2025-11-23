# Node Pods Folder (Field-Filtered Pods Under a Node)

## Goal

Expose `/nodes/<node>/pods` so users can drill into the pods scheduled on a node without starting a cluster-wide pod watch. The folder should stream live updates, show namespace-first columns from the server table response, and avoid a count column.

## Requirements

- Use a server-side field selector `spec.nodeName=<node>` for both LIST and WATCH; never rely on client-side filtering of a cluster-wide pod informer.
- No count column; keep the table columns returned by the server (namespace first). Fallback columns must include Namespace before Name.
- The watch must be cluster-scoped with the nodeName field selector (pods span namespaces).
- Integrate cleanly with the cluster pool; no leaked goroutines or caches when the view closes.

## Options Considered

1) **Selector via main cache only** (pass `cache.WithSelector` to `GetInformer` and selector-aware list helpers).
   - Pros: minimal plumbing; reuse cache lifecycle.
   - Cons: any other unfiltered pod informer (e.g., the generic Pods view) will still start a global pod watch; weak guarantee. Requires selector-aware list APIs anyway.

2) **Dedicated selector cache per node (previously recommended)**: spin up a small controller-runtime cache scoped by `SelectorsByObject` for pods with the node field selector; use it for list/watch in the node folder only.
   - Pros: isolates the watch, avoids a global pod watch for this view.
   - Cons: adds a new cache type alongside the main cluster; selector feels like a first-class scope, not just a cache tweak.

3) **Dedicated Cluster per selector dimension (updated recommendation)**: treat the field selector as another scope axis (like namespace). Extend the cluster pool key to include a selector signature for the target GVR, and create a dedicated `Cluster` whose caches and table fetcher are pre-wired to always apply that selector to pod LIST/WATCH.
   - Pros: clear ownership and lifecycle (pool already manages Cluster start/stop); hard guarantee that every client (cache, tablecache) in that Cluster speaks with the selector; keeps the model consistent with the “Cluster per namespace” strategy.
   - Cons: more Cluster instances if many nodes are opened; must ensure discovery and other shared clients remain unaffected or can be reused.

## Recommended Approach (Option 3)

### Cluster Pool Key Extension
- Extend `cluster.Pool` key to include a selector signature per GVR (e.g., `SelectorKey string` where we hash/normalize `gvr+fieldSelector+labelSelector`).
- For the node pods view, the key would be `(kubeconfig, context, namespace="", selectorKey=pods|field:spec.nodeName=<node>)`.
- Pool returns a distinct `Cluster` for that selector scope; TTL eviction continues to apply.

### Cluster Wiring for Selector Scope
- Add a Cluster option (e.g., `WithSelectorScope(map[schema.GroupVersionResource]cache.ObjectSelector)`) that:
  - Configures the default cache with `SelectorsByObject` for the specified GVRs (pods only for this view).
  - Passes the same selectors into `tablecache` operations by default (field/label selectors applied to `ListTable`/`GetTable`/`WatchTable`).
  - Wraps any informer starts to respect the selector without extra parameters.
- Discovery, RESTMapper, and dynamic client remain shared inside the Cluster; only cache/tablecache/list/watch calls are selector-scoped.

### List/Watch Behavior
- `ObjectsFolder` for node pods uses the selector-scoped Cluster:
  - `ListRowsByGVR`/`ListByGVR` automatically apply the selector through the Cluster’s cache/tablecache.
  - Informers started via `startInformerForResource` are already filtered by the Cluster’s cache; no extra selector plumbing needed at call sites.
- Field selector is `spec.nodeName=<node>`, namespace empty.

### Node Pods Folder
- New `NodePodsFolder` under `nodes/<node>/pods`:
  - Uses `ObjectsFolder` scaffolding but backed by the selector-scoped Cluster keyed to the node.
  - No count column override; rely on server table columns. If the table path fails, set columns to `Namespace`, `Name`, `Age` to keep namespace first.
  - Apply a defensive filter (`spec.nodeName` match) to rows in case a wider list leaks through.
- Register a child for `core/v1/nodes` in `registry.go` that builds `NodePodsFolder`.
- Watch TTL from `watchDuration` still applies; when it fires or the folder drops, the selector-scoped Cluster can be evicted via pool TTL.

## Trade-offs

- **Resource usage**: extra Cluster per selector scope (node); mitigated by pool TTL/eviction and shared discovery/mapper inside each Cluster.
- **Complexity**: pool key grows; Cluster gains selector-scoped cache/tablecache defaults. Call sites stay simple (no selector args).
- **Isolation**: hard guarantee that the node view never starts a global pod watch. Other views keep their existing scope; adopting selector-scoped Clusters elsewhere is opt-in.

## Open Questions / Follow-ups

- Pool key format: best way to hash/serialize the selector to keep keys deterministic.
- Should discovery be reused across selector-scoped Clusters to reduce load? (e.g., share a discovery client per kubeconfig/context).
- Could we generalize selector-scoped Clusters for other field/label-scoped views once stable?
