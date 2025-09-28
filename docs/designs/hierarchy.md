# kc Navigation Hierarchy — Design and Interfaces

This document describes how kc models the Kubernetes navigation hierarchy and the Go interfaces that compose it. The goal is to keep the UI precise (no brittle string matching), modular, and easy to extend.

## High‑Level Model

The UI navigates a tree of locations derived from Kubernetes discovery and live caches:

- Root
  - `contexts` (switch context; special row)
  - `namespaces` (cluster namespaces)
  - `<cluster‑scoped resource>` (e.g., nodes, storageclasses, …)
- Namespaced
  - `/namespaces/<ns>` → resource groups within the namespace
  - `/namespaces/<ns>/<resource>` → objects of that resource
  - `/namespaces/<ns>/pods/<pod>` → folders for containers + initContainers
  - `/namespaces/<ns>/(configmaps|secrets)/<name>` → data keys as file‑like entries

Every location is backed by exact identities (GVR/GVK), never heuristic breadcrumb parsing.

## Packages and Responsibilities

- `pkg/resources`
  - Discovery and resource client utilities.
  - `Manager` provides:
    - `ResourceToGVK(resource string) (schema.GroupVersionKind, error)`
    - `GVKToGVR(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error)`
    - `ListTableByGVR(ctx, gvr, ns)` to fetch server‑side Table.
  - Store provider and cluster pool (list/watch via controller‑runtime cache).

- `internal/ui` (TUI composition)
  - `App` orchestrates panels, modals, terminal, and implements `view.Context` (see below).
  - `Panel` presents a listing for the current location and constructs `Item`s precisely from discovery/store data.
  - `Item` is a row in the panel with exact identity and optional capabilities.

- `internal/ui/view` (modular viewers)
  - `Context` — minimal read API the UI exposes to viewers:
    - `ResourceToGVK(resource string) (schema.GroupVersionKind, error)`
    - `GetObject(gvk schema.GroupVersionKind, ns, name string) (map[string]interface{}, error)`
  - `ViewProvider` — provides `BuildView(ctx Context) (title, body string, err error)` for `F3`.
  - Example viewers:
    - `KubeObjectView`: YAML of a full object.
    - `ConfigKeyView`: value for a single key (secret values decoded when textual).
    - `PodContainerView`: YAML of a single container’s spec.

## Core Types and Interfaces

### Item (internal/ui)

```go
// Item represents a row in a panel.
type Item struct {
    Name      string
    Type      ItemType             // Directory, File, Resource, Namespace, Context
    Size      string               // e.g., count column for resource groups
    Modified  string
    Selected  bool                 // multi‑select state
    TypedGVK  schema.GroupVersionKind
    TypedGVR  schema.GroupVersionResource
    Enterable bool                 // ‘/’ prefix and enter navigation
    Viewer    view.ViewProvider    // optional F3 provider
}
```

The `Item` carries full identity (`TypedGVK`/`TypedGVR`). Capabilities are opt‑in via interfaces on the item (e.g., `Viewer`). This avoids brittle string parsing on path segments.

### Panel (internal/ui)

- Responsible for:
  - Turning the current location into a list of `Item`s.
  - Populating precise capabilities on each item (e.g., attaching `ConfigKeyView` to key rows).
  - Rendering via a shared table pipeline (header + aligned columns) whenever server‑side Table or synthesized group tables are available.
- Navigation:
  - Computes “next” locations deterministically. E.g., entering a Pod object produces container folder items by reading the object under the cursor and creating `Item`s with `Enterable` and `Viewer`.

### App (internal/ui)

- Owns resource/discovery managers and implements `view.Context`:

```go
type Context interface {
    ResourceToGVK(resource string) (schema.GroupVersionKind, error)
    GetObject(gvk schema.GroupVersionKind, namespace, name string) (map[string]interface{}, error)
}
```

- Delegates F3 to `Item.Viewer` when present, otherwise falls back to `KubeObjectView`.
- Provides modal composition and theme handling.

### Viewers (internal/ui/view)

- `KubeObjectView` — YAML for the whole object.
- `ConfigKeyView` — renders only the key’s value, never the whole object. For secrets, base64 is decoded and displayed as UTF‑8 text when appropriate; otherwise the base64 string is shown.
- `PodContainerView` — renders a single container/initContainer spec from `spec.containers`/`spec.initContainers` by name.

These enable a strict, type‑safe F3 experience without relying on breadcrumb string matching.

## Data Flow

1. Discovery + store give `Panel` the exact GVRs for each resource group.
2. `Panel` builds `Item`s with identities and attaches capabilities (e.g., `ConfigKeyView` for configmap keys) based on the precise type of each row.
3. On `F3`, `App` asks the `Item.Viewer` (if present) to build the content and opens a viewer modal. Otherwise it renders the full object YAML.
4. Column layout uses one table pipeline (headers: Name, Group, Count for resource group views) for consistent alignment/styling.

## Extensibility / Capability Pattern

Additional behaviors plug into the same pattern:

- `EditProvider` (future): provides an edit implementation for `F4`.
- `ActionProvider` (future): context‑menu actions.
- `SelectProvider` (future): custom selection semantics for pattern dialogs (`+`/`-`).

The panel attaches capabilities per item; `App` routes actions to those capabilities.

## Path vs Identity

Paths (`/namespaces/<ns>/<res>/<name>…`) are UX artifacts only. All behavior is driven by exact identities carried on `Item`s (GVR/GVK) and discovery results. No string parsing is used to decide behavior.

## Examples

- ConfigMap key
  - Panel attaches: `Viewer = &ConfigKeyView{Namespace: ns, Name: cmName, Key: k, IsSecret: false}`.
  - `F3`: viewer returns title `cmName:key` and body = plain text value.

- Secret key
  - Same viewer with `IsSecret: true` — body = decoded text if UTF‑8; otherwise base64 string.

- Pod container
  - Panel attaches: `Viewer = &PodContainerView{Namespace: ns, Pod: podName, Container: c}`.
  - `F3`: viewer returns title `podName/container` and body = YAML of the container.

## Future: Logs

- Add `LogsView` (container logs) implementing `ViewProvider` or a dedicated streaming viewer interface.
- Attach as a child `Item` under each container folder.

## Testing Strategy

- Unit test viewers with fake `Context` implementations.
- Unit test panel’s item construction logic (ensuring correct capabilities and identities) with synthetic unstructured objects.

## Summary

The navigation hierarchy is defined by exact Kubernetes identities instead of ad‑hoc path parsing. Panels assemble typed `Item`s with explicit capabilities. `App` stays thin and delegates to modular providers. This keeps the design precise, testable, and extensible.

