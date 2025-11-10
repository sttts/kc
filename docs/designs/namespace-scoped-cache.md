# Namespace-Scoped Cache Strategy

## Background

Resource-group rows pre-compute object counts by peeking directly into the controller-runtime cache via `GetStore()`/`GetIndexer()`. That works as long as the cache exposes plain client-go `SharedIndexInformer` instances, but breaks when the cache is restricted to a subset of namespaces. In that mode controller-runtime swaps in a `multiNamespaceInformer` wrapper that only satisfies the narrow `cache.Informer` interface—`GetStore()` and `GetIndexer()` are gone, so count recomputation has to fall back to fresh LIST calls.

We tried controller-runtime's built-in namespace scoping (`Cache.DefaultNamespaces` / `ByObject.Namespaces`) and observed exactly that: informers no longer expose their stores, our type assertions fail, and `Count()` bounces back to `ListByGVR`, defeating the point of the cache.

## Goal

Keep direct access to informer stores so counts, empty peeks, and live updates remain fast, while still avoiding cluster-wide watches for every namespace. Each folder should operate against a cache that only lists/watch its namespace without losing access to the underlying `SharedIndexInformer`.

## Approach

Instead of letting controller-runtime filter namespaces, we spin up a dedicated cluster/cache per namespace and rewrite LIST/WATCH requests at the transport layer:

1. **Cluster per namespace.** The existing pool already keys by kubeconfig+context. We treat `Namespace` as part of the key so each folder (or namespace tab) pulls its own cluster instance. From controller-runtime's view the cache is unrestricted, so `GetInformer` still returns the raw `SharedIndexInformer` (with `GetStore()` available).

2. **Transport shim.** We wrap the `http.RoundTripper` used by the cache's REST client. When it sees a LIST or WATCH for a namespaced resource, it rewrites the URL path to `.../namespaces/<ns>/<resource>` (and preserves query parameters like `resourceVersion`, `continue`, etc.). Non-namespaced resources pass through untouched, as do non-cache clients like discovery or the dynamic client.

3. **Requester isolation.** Because each namespace gets its own cluster/cache, the transport shim never needs to multiplex access. Every informer on that cache is understood to be "for ns=X" by construction. Two different namespaces use two different clusters, so there's no risk of a rewritten request bleeding into the wrong informer.

4. **Store access retained.** With the cache unrestricted from controller-runtime's perspective, all informers remain `SharedIndexInformer`s. Our `countFromInformerLocked` path can keep using `GetStore()/GetIndexer()` without type gymnastics or fallback LIST calls.

## Trade-offs

- **Resource usage.** We still pay for one cache per namespace, same as today, but we avoid starting extra controllers or double-watching resources.
- **Path rewrite complexity.** The shim must understand Kubernetes REST paths (core vs. aggregated groups, subresources). It's systematic, and we can lean on `RequestInfoFactory` if needed.
- **Future flexibility.** Because the cache stays “cluster-wide” in controller-runtime's eyes, features like indexers, predicates, and cache removal continue to work unchanged.

## Next steps

1. Implement the transport wrapper (likely in `internal/cluster`) that takes a namespace and rewrites cache requests.
2. Thread that wrapper into cluster creation when `Key.Namespace` is set, so only namespace-specific clusters use it.
3. Remove the fallback `ListByGVR` in `countFromInformerLocked` once the wrapper is proven, keeping the direct store code path.
