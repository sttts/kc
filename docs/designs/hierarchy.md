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

Every location is backed by exact identities (GVR), never heuristic breadcrumb parsing. GVK is presentation-only and used for display or creation workflows.

## Packages and Responsibilities

- `pkg/resources`
  - Discovery and resource client utilities.
  - `Manager` provides:
    - `ListTableByGVR(ctx, gvr, ns)` to fetch server‑side Table.
    - Mapping helpers are internal-only; navigation uses GVR end-to-end.
  - Store provider and cluster pool (list/watch via controller‑runtime cache).

- `internal/ui` (TUI composition)
  - `App` orchestrates panels, modals, and terminal.
  - `Panel` presents a listing for the current location and constructs `Item`s precisely from discovery/store data.
  - `Item` is a row in the panel with exact identity and optional capabilities.

- `internal/navigation` (items + folders)
  - Owns the core navigation model: `Item`, `ObjectItem`, `Enterable`, `Viewable`, `Folder`, and the Back marker.
  - Contains concrete item and folder implementations (contexts, namespaces, resource groups, object lists, containers, config/secret keys, logs).
  - Minimal UI code only (per-cell default styles); no Bubble Tea components.
  - Depends on `pkg/resources` for cluster access and `internal/table` for rows/columns.

- `internal/ui/view` (modular viewers)
  - `ViewProvider` — provides `BuildView() (title, body string, err error)` for `F3`.
  - Concrete viewers capture required dependencies (e.g., store/manager) at construction time.
  - Example viewers:
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

// Viewable is a capability: an item can render a focused view for F3.
// Items that don’t implement Viewable fall back to a generic object view (when ObjectItem).
type Viewable interface {
    BuildView() (title, body string, err error)
}
```

Notes:
- Items expose their table shape directly via `table.Row`, unifying all list rendering.
- `Details()` is a short, non-tabular description for status lines.
- Row styling is set per-cell via the `styles` returned from `Columns()`. Default: make all cells green here; selection/other modes can override.
- `ObjectItem` provides precise identity via accessors instead of brittle paths.

### Panel (internal/ui)

- Responsible for:
  - Turning the current location into a list of `Item`s.
  - Populating precise capabilities on each item (e.g., attaching `ConfigKeyView` to key rows).
  - Rendering via a shared table pipeline using `[]table.Row` (header + aligned columns) for both server‑side Table and synthesized group/object lists.
- Navigation:
  - Computes “next” locations deterministically. E.g., entering a Pod object produces container folder items by reading the object under the cursor and creating `Item`s with `Enterable` and `Viewer`.

### App (internal/ui)

- Owns resource/discovery managers. Factories construct viewers/folders with these dependencies bound.
- Delegates F3 to `Viewable` items; otherwise falls back to object YAML when the item is an `ObjectItem`.
- Provides modal composition and theme handling.

### Viewers (internal/ui/view)

- `KubeObjectView` — YAML for the whole object. Constructed with store access and object coordinates (GVR/ns/name).
- `ConfigKeyView` — renders only the key’s value, never the whole object. For secrets, base64 is decoded and displayed as UTF‑8 text when appropriate; otherwise the base64 string is shown. Constructed with store access and key coordinates (GVR/ns/name/key).
- `PodContainerView` — renders a single container/initContainer spec from `spec.containers`/`spec.initContainers` by name. Constructed with store access and container coordinates (pod GVR/ns/name/container).

These enable a strict, type‑safe F3 experience without relying on breadcrumb string matching. Viewers are created by factories that bind dependencies upfront; `BuildView()` takes no parameters.

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
```

Notes:
- `Enterable.Enter` constructs the next Folder directly; no string path parsing.
- `Folder.Columns` returns `[]table.Column` (initially just titles). Future additions can include width, orientation, or priority.
- Root, contexts, namespaces, resource groups, object lists, and virtual children (containers, config/secret keys, logs) are all represented as Folders.

### Back navigation and breadcrumbs

The “..” row is a Back item (not Enterable). It signals the app to pop the current Folder and restore prior selection/scroll state.

Usage boundaries:
- App owns navigation state (a private stack of {Folder, SelectedID}). Panel is ignorant of history.
- App injects a BackItem (implements `Item` and a `Back` marker) at the top when stack depth > 1.
- On Enter:
  - If the selected row implements `Back`, the app pops to the previous Folder and restores its `SelectedID`.
  - Else if it implements `Enterable`, the app pushes the new Folder and resets `SelectedID`.
- Breadcrumbs are derived from the stack of Folder titles.

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

1. Discovery + store give `Panel` the exact GVRs for each resource group.
2. `Panel` builds `Item`s with identities and attaches capabilities (e.g., `ConfigKeyView` for configmap keys) based on the precise type of each row.
3. On `F3`, if the selected item implements `Viewable`, `App` calls `BuildView()`. Otherwise, if it is an `ObjectItem`, it renders the full object YAML.
4. Column layout uses one table pipeline (headers: Name, Group, Count for resource group views) for consistent alignment/styling. We now import `internal/table` directly and build rows using its exported `Row` and `SimpleRow` types:

```go
import tbl "github.com/sttts/kc/internal/table"

row := tbl.SimpleRow{ID: "pods"}
row.SetColumn(0, "pods", nil)           // Name
row.SetColumn(1, "", dimStyle)          // Group (dimmed when appropriate)
row.SetColumn(2, "42", rightAlignStyle) // Count (right‑aligned)
```

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

## Summary

The navigation hierarchy is defined by exact Kubernetes identities instead of ad‑hoc path parsing. Panels assemble typed `Item`s with explicit capabilities. `App` stays thin and delegates to modular providers. This keeps the design precise, testable, and extensible.
