# kc Navigation Hierarchy — Design and Interfaces

This document describes how kc models the Kubernetes navigation hierarchy and the Go interfaces that compose it. The goal is to keep the UI precise (no brittle string matching), modular, and easy to extend.

## High‑Level Model

The UI navigates a tree of locations derived from Kubernetes discovery and live caches:

- Root
  - `contexts` (switch context; special row)
  - `namespaces` (default context)
  - `<cluster‑scoped resource>` (e.g., nodes, storageclasses, …)
- Context‑scoped
  - `/contexts/<ctx>/namespaces` → all namespaces in the selected context
  - `/contexts/<ctx>/namespaces/<ns>` → resource groups within the namespace
  - `/contexts/<ctx>/namespaces/<ns>/<resource>` → objects of that resource
  - `/contexts/<ctx>/namespaces/<ns>/pods/<pod>` → folders for containers + initContainers
  - `/contexts/<ctx>/namespaces/<ns>/(configmaps|secrets)/<name>` → data keys as file‑like entries

Every location is backed by exact identities (GVR), never heuristic breadcrumb parsing. GVK is presentation-only and used for display or creation workflows.

## Packages and Responsibilities

- `internal/cluster`
  - Thin extension of controller‑runtime `Cluster` with a self‑updating `RESTMapper` and discovery cache.
  - Exposes `RESTMapper()` and `ListTableByGVR(...)` helpers used by Folders and viewers.

- `internal/ui` (TUI composition)
  - `App` orchestrates panels, modals, and terminal.
  - `Panel` renders a provided `Folder` via BigTable and routes keys; it does not build rows.
  - `Item` is a row with identity and optional capabilities.

- `internal/navigation` (items + folders)
  - Owns the core navigation model: `Item`, `Enterable`, `Folder`, `Back`.
  - Folders are self‑sufficient: each lazily populates rows from injected dependencies and `Enterable.Enter()` returns the next Folder.
- `BaseFolder` automatically exposes a synthetic “..” row (`BackItem`) whenever `len(path) > 0`.
  - Depends on `internal/cluster` and `internal/table`.

- `internal/ui/view` (modular viewers)
  - Helpers that navigation items can call from `ViewContent()` to format specialised views.
  - Concrete viewers capture required dependencies (e.g., store/manager) at construction time.
  - Example helpers:
    - `KubeObjectView`: YAML of a full object.
    - `ConfigKeyView`: value for a single key (secret values decoded when textual).
    - `PodContainerView`: YAML of a single container’s spec.

## Core Types and Interfaces

### Item (internal/ui)

```go
// Item drives both rendering (table.Row) and actions.
// It must return a stable ID and aligned cells/styles for the table.
type Item interface {
    table.Row        // Columns() (id, cells, styles, exists)
    Details() string // concise info shown in status/footer
    Path() []string  // absolute path segments, excluding leading "/"
}

// ObjectItem is implemented by items that represent actual Kubernetes objects.
// Non-object entries (containers, logs, config keys) do not need to implement it.
type ObjectItem interface {
    Item
    GVR() schema.GroupVersionResource
    Namespace() string
    Name() string
    // Optional: GVK() schema.GroupVersionKind // for visuals only
}

// Viewable is a capability: an item can render a focused view for F3 and optionally provide syntax hints.
type Viewable interface {
    ViewContent() (title, body, lang, mime, filename string, err error)
}

// Countable reports aggregate information for list-style rows (resource groups, context lists).
// Count must go through the shared informer (starting it if required) so downstream readers benefit
// from a primed cache, whereas Empty performs a lightweight peek (limit=1) that avoids booting
// informers; this lets the UI hide empty resources by default without paying the informer cost.
type Countable interface {
    Count() int
    Empty() bool
}
```

Notes:
- Items expose their table shape directly via `table.Row`, unifying all list rendering.
- `Details()` is a short, non-tabular description for status lines.
- `Path()` is the canonical breadcrumb path for this row (e.g., ["namespaces","ns1","pods","web-0"]). Panels/viewers render it with "/"+strings.Join(path, "/").
- Row styling is set per-cell via the `styles` returned from `Columns()`. Default: make all cells green here; selection/other modes can override.
- `ObjectItem` provides precise identity via accessors instead of brittle paths.
- `Countable` rows (resource groups, context lists) lazily warm informers when counts are requested, while `Empty()` performs a cheap peek (limit=1) so the UI can hide empty collections without booting caches up-front.
- Details semantics:
  - Resource groups (pods/configmaps/…): show "<resource> (<group>/<version>)" or just "<version>" for core.
  - Object rows: show "<ns>/<name> (Kind <group>/<version>)" when the group is set, otherwise "<ns>/<name> (Kind <version>)" for core; for cluster-scoped, drop the namespace prefix. Kind resolves via RESTMapper for the list GVR. Implementation uses `gvr.GroupVersion().String()` and `types.NamespacedName.String()`.
  - Panels prefer `Item.Details()` for the footer when available; otherwise fall back to the item’s `TypedGVR`.

#### Planned Item Hierarchy

```
RowItem (minimal table-backed row)
├─ ObjectItem (adds GVR/Namespace/Name + ViewContent via handler registry)
│  ├─ NamespaceItem (adds Enter into namespace resource groups)
│  ├─ PodItem (plain pod object row)
│  ├─ ConfigMapItem (plain ConfigMap object row)
│  ├─ SecretItem (plain Secret object row)
│  └─ …other resource-specific object rows
├─ ContextItem (enterable; opens the context-scoped hierarchy)
├─ ContextListItem (enterable; lists contexts, Countable via kubeconfig provider)
├─ ResourceGroupItem (enterable; opens an object list/folder of ObjectItems; Countable for counts)
├─ ConfigKeyItem (viewable; ConfigMap/Secret key value)
├─ ContainerItem (viewable; pod container spec)
└─ BackItem (synthetic ".." row; neither viewable nor enterable)
```

### Panel (internal/ui)

- Renders `Folder` via BigTable; does not build rows. Keys route to Enter/Back.

### App (internal/ui)

- Owns resource/discovery managers. Factories construct viewers/folders with these dependencies bound.
- Delegates F3 to `Viewable` items; otherwise falls back to object YAML when the item is an `ObjectItem`.
- Provides modal composition and theme handling.

### Viewers (internal/ui/view)

- `KubeObjectView` — YAML for the whole object. Constructed with store access and object coordinates (GVR/ns/name).
- `ConfigKeyView` — renders only the key’s value, never the whole object. For secrets, base64 is decoded and displayed as UTF‑8 text when appropriate; otherwise the base64 string is shown. Constructed with store access and key coordinates (GVR/ns/name/key).
- `PodContainerView` — renders a single container/initContainer spec from `spec.containers`/`spec.initContainers` by name. Constructed with store access and container coordinates (pod GVR/ns/name/container).

These helpers enable a strict, type‑safe F3 experience without relying on breadcrumb string matching. Navigation items (or handlers) call them from `ViewContent()` with dependencies bound upfront.

### Enterable and Folder

Some items navigate to another listing when entered (e.g., resource groups, a pod’s containers, a configmap’s data keys). We model this with two interfaces:

```go
// Enterable is a capability: selecting Enter on the item yields a Folder.
type Enterable interface {
    Item
    Enter() (Folder, error)
}

// Folder represents a navigable listing. It implements table.List for rows
// and provides the visible column descriptors.
type Folder interface {
    table.List                // Lines/Above/Below/Len/Find over table.Row rows
    Columns() []table.Column  // column titles (+ future width hints)
    Title() string            // short label for breadcrumbs
    Key() string              // stable identity for history/restore (e.g., ns/gvr or object/children)
}
// ObjectsFolder additionally exposes identity for object lists (for F3/edit).
type ObjectsFolder interface {
    Folder
    ObjectListMeta() (schema.GroupVersionResource, string, bool)
}
```

Notes:
- `Enterable.Enter` constructs the next Folder directly; no string path parsing.
- `Folder.Columns` returns `[]table.Column` (initially just titles). Future additions can include width, orientation, or priority.
- Root, contexts, namespaces, resource groups, object lists, and virtual children (containers, config/secret keys, logs) are all represented as Folders.

### Folder Hierarchy and Responsibilities

Folders embed smaller building blocks so shared behaviour lives in one place instead of string checks. The intended hierarchy is:

```
BaseFolder
├─ ResourcesFolder             // shared count/peek logic for resource groups
│  ├─ ClusterResourcesFolder   // cluster-scoped resource groups (namespaces, nodes, …)
│  │  ├─ RootFolder            // "/" entrypoint; wraps contexts, namespaces, cluster resources
│  │  └─ ContextRootFolder     // root when browsing a context tree (same behaviour as RootFolder)
│  └─ NamespacedResourcesFolder // namespaced resource groups (pods, configmaps, …)
├─ ObjectsFolder               // shared population for object lists and server-side Tables
│  ├─ ClusterObjectsFolder     // cluster-scoped objects for a GVR
│  └─ NamespacedObjectsFolder  // namespace-scoped objects for a GVR
├─ PodContainersFolder         // virtual folder for pod containers/initContainers
├─ ConfigMapKeysFolder         // virtual folder for ConfigMap data keys
└─ SecretKeysFolder            // virtual folder for Secret data keys
```

- `ResourcesFolder` wires the `Countable` interface, async informer startup, and throttled peeks using `resources.peekInterval`, reading behaviour directly from the injected app config (`Deps.Config`). Children only describe which `ResourceGroupItem`s to build.
- `ObjectsFolder` centralises server-side Table usage, fallback `GetByGVR` listing, and `ObjectListMeta` bookkeeping so viewers consistently know the active GVR + namespace.
- `RootFolder` and `ContextRootFolder` embed `ClusterResourcesFolder`; their only differences are breadcrumb titles and keys.
- Synthetic folders (containers / config-map keys / secret keys) embed `BaseFolder` directly because they only need lazy population.

### Back navigation and breadcrumbs

The “..” row is a Back item (not Enterable). It signals the app to pop the current Folder and restore prior selection/scroll state.

Usage boundaries:
- App owns navigation state (a private stack of {Folder, SelectedID}). Panel is ignorant of history.
- App injects a BackItem (implements `Item` and a `Back` marker) at the top when stack depth > 1.
- On Enter:
  - If the selected row implements `Back`, the app pops to the previous Folder and restores its `SelectedID`.
  - Else if it implements `Enterable`, the app pushes the new Folder and resets `SelectedID`.
- Breadcrumbs are computed from navigator state (see “Breadcrumbs and Paths”).

Interface sketch (markers only):

```go
// Back marks a row as the "go up" action.
type Back interface{ Item }

// BackItem is a concrete row with ID "__back__" and first cell "..".
// It fits the current Columns() shape (other cells empty).
```

Notes:
- Panel does not need to “know” what “..” means; it just renders the rows it receives.
- `Folder.Key()` provides a stable identity for storing/restoring selection even when revisiting via non-back paths.

## Data Flow

1. App creates a `Navigator` with the root `Folder` (Deps injected).
2. Panel renders `Current()` directly and uses `HasBack()` only to decide cursor defaults or disable the back action when inactive.
3. Enter pushes the next Folder returned by the selected `Enterable` row. Back pops.
4. Viewers use GVR/ns/rowID from the current Folder to fetch content; titles use `item.Path()` when available (fallback: GVR/ns path).

## Self‑Sufficient Folders

- Dependencies: Folders receive an immutable `Deps` struct with:
  - `Cl *internal/cluster.Cluster` (controller‑runtime client + cache + RESTMapper)
  - `Ctx context.Context`
  - `CtxName string` (for keys/titles)
  - `KubeConfig clientcmdapi.Config` (contexts map for listings; `CurrentContext` marks the active entry)
  - `AppConfig *appconfig.Config` (validated application settings)
- Constructors (UI‑agnostic) live in `internal/navigation/folders/` (same Go package) and carry base path segments:
  - `NewRootFolder(deps, enterContext func(name string, basePath []string) (Folder, error))`
  - `NewContextRootFolder(deps, basePath []string)`
  - `NewContextsFolder(deps, enterContext func(name string, basePath []string) (Folder, error))`
  - `NewNamespacedResourcesFolder(deps, ns, basePath []string)`
  - `NewNamespacedObjectsFolder(deps, gvr, ns, basePath []string)`
  - `NewClusterObjectsFolder(deps, gvr, basePath []string)`
  - `NewPodContainersFolder(deps, ns, pod, basePath []string)`
  - `NewConfigMapKeysFolder(deps, ns, name, basePath []string)`
  - `NewSecretKeysFolder(deps, ns, name, basePath []string)`
- Lazy population: first access to `Lines/Len/Find` triggers a single populate pass that builds
  a `table.SliceList` of rows. Rows are `table.Row` values (typically `SimpleRow`/`EnterableItem`).
- Enterable rows: items that can be entered implement `Enterable` and return the exact next Folder with `deps` already bound. Rows and child folders get a propagated `path []string` so breadcrumbs remain consistent at every level.
- Back: The “..” entry is emitted by the folder itself (via `BaseFolder`) when the breadcrumb path is non-empty; no wrapper is required.

## Programmatic Navigation

- Paths are UX‑level, not a data source. Programmatic navigation composes Enter calls and sets selection IDs on the navigator:
  - Example: `/namespaces/<ns>` → `nav.SetSelectionID("namespaces"); nav.Push(NewClusterObjectsFolder(deps, schema.GroupVersionResource{Group:"",Version:"v1",Resource:"namespaces"}, ["namespaces"]))`; then `nav.SetSelectionID(ns); nav.Push(NewNamespacedGroupsFolder(deps, ns, ["namespaces",ns]))`.
  - Validation (missing ns/resource/object) falls back to the nearest valid parent.
- The App owns a `Navigator`; `GoToNamespace(ns)` builds a clean stack using Enterable rows and updates the panels with `Current()`/`HasBack()`.

## Breadcrumbs and Paths

- Rule: the first column of each selected row is the path segment. Enterable rows display a UI leading “/”; navigation trims it for the logical segment.
- `Navigator.Path()` walks the navigator stack, finds each parent frame’s selected row, collects its first column (after trimming one leading “/”), ignores the synthetic back row, and joins segments with a leading "/". Root is just `/`.
- Items expose `Path() []string` carrying their absolute segments so viewers/modal titles don’t need to inspect navigator state.
- Panels set their header from `nav.Path()`; viewers prefer `item.Path()` and fall back to GVR/ns when needed.

## Testing (Envtest)

- Use controller‑runtime envtest to start an API server and seed fixtures (ns/configmap/secret/node).
- Build cluster + cache from envtest `rest.Config` via `internal/cluster`.
- Create `Deps` and walk the hierarchy via Folders only (no UI imports):
  1. Root → assert rows include `/contexts`, `/namespaces`.
  2. Enter `/namespaces` → assert `/<ns>` present.
  3. Enter `/<ns>` (groups) → assert `/configmaps`, `/secrets` with counts.
  4. Enter `/configmaps` → assert `/cm1`.
  5. Enter `cm1` → assert keys `a`, `b`.
  6. Back using `Navigator.Back()` → verify parent restored.
  7. Cluster objects (`nodes`) → assert `n1`.


## Extensibility / Capability Pattern

Additional behaviors plug into the same pattern:

- `EditProvider` (future): provides an edit implementation for `F4`.
- `ActionProvider` (future): context‑menu actions.
- `SelectProvider` (future): custom selection semantics for pattern dialogs (`+`/`-`).

For tabular rendering, panels produce `[]table.Row` via `internal/table`, enabling consistent alignment, virtualization, and selection rendering across the app.

The panel attaches capabilities per item; `App` routes actions to those capabilities.

## Path vs Identity

Paths (`/namespaces/<ns>/<res>/<name>…`) are UX artifacts only. All behavior is driven by exact identities carried on `Item`s (GVR) and discovery results. No string parsing is used to decide behavior.

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

- Unit test viewers by injecting fake stores/managers into viewer constructors.
- Unit test panel’s item construction logic (ensuring correct capabilities and identities) with synthetic unstructured objects.

## Current vs. Planned

- Current:
  - Navigation via `ContextsFolder`, `ResourcesFolder`, `ObjectsFolder` using GVR identities.
  - Back item + Navigator with selection restore (by row ID).
  - Root shows `/contexts`, `/namespaces` (default context), and cluster-scoped resources with counts.
  - F3 on `ObjectsFolder` fetches `(GVR, ns, rowID)` and renders YAML.
  - Specialized child folders exist for pods (containers) and {configmaps,secrets} (keys).
  - YAML modal removes left/right borders for clean copy/paste.

- Planned:
  - Viewers for specialized folders:
    - `ConfigKeyView` (plain or UTF‑8 decoded vs base64 for secrets).
    - `PodContainerView` (single container spec); `LogsView` with follow/search.
  - Big table integration (internal/table `BigTable`) with virtualization + selection styles.
  - Group column dimming and refined styles.
  - Replace any legacy `SliceFolder` remnants; fully adopt pluralized folder types.
  - Extend Enterables for logs under containers.

## Summary

The navigation hierarchy is defined by exact Kubernetes identities instead of ad‑hoc path parsing. Panels assemble typed `Item`s with explicit capabilities. `App` stays thin and delegates to modular providers. This keeps the design precise, testable, and extensible.
