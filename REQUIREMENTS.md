# Kubernetes Commander (kc) Requirements

## Purpose & Scope
- Terminal UI for Kubernetes that looks and feels like Midnight Commander (mc).
- Two-panel browser over Kubernetes APIs with live data and actionable keys.

## UX Parity with mc
- Two panels; one active at a time. Switch with `Tab`.
- Navigate with arrows; `Enter` opens item; `..` goes up.
- Function-key bar always visible; actions greyed when not applicable.
- Selected items are bold yellow; selection clears when changing location.
- `Alt+I` makes the inactive panel jump to the same location as the active panel (synchronize locations), mirroring mc behavior.

## Navigation Model (Paths)
- Root hierarchies are browsable as path-like locations:
  - `/cluster` — cluster-scoped resources for current kubeconfig/context.
  - `/cluster/<resource>` — e.g., `/cluster/clusterrolebindings` lists objects.
  - `/cluster/namespaces/<ns>` — namespace details; `F3` view, `F4` edit, `F8` delete; `Enter` to enter namespace resources.
  - `/cluster/namespaces/<ns>/<resource>` — e.g., `/cluster/namespaces/kube-system/pods` lists pods.
  - `/cluster/namespaces/<ns>/pods/<pod>` — `Enter` shows containers, then subresources like `logs` per container.
  - `/contexts` — contexts from current kubeconfig; current context is bold.
  - `/contexts/<ctx>/...` — full hierarchy as under `/cluster` but for `<ctx>`.
  - `/kubeconfigs` — discovered kubeconfigs; includes an action to add by path via dialog.
- Every level shows `..` to navigate back.

## Selection & Bulk Actions
- Toggle selection with `Ctrl+T` or `Insert`; multiple selections allowed.
- Actions (e.g., delete) operate on selection when non-empty, otherwise on the item under the cursor.

## Actions & Keys
- `F3` View: show YAML for objects; works on namespaces and resources.
- `F4` Edit: open YAML editor; save applies changes; works on namespaces and resources.
- `F7` Create: context-sensitive create. At `/cluster/namespaces` creates a namespace via dialog. Future: OpenAPI v3–driven forms.
- `F8` Delete: delete selected/current objects with confirmation.
- `Ctrl+O` Terminal toggle: hide UI to show full-screen terminal; when not full-screen, show last two terminal lines above cursor.
- `Alt+I` Sync other panel location: when pressed, the non-focused panel navigates to the current path of the focused panel.

## Extensible Actions (Per Resource Type)
- Allow adding extra actions that can call external tools based on current location and selection.
- Pass context via env/args: `KUBECONFIG`, `CONTEXT`, `NAMESPACE`, `GROUP`, `VERSION`, `KIND`, `NAME`, and serialized object (path or stdin).
- Actions may be contributed by handlers or configured by users; operate on multi-selection when present.

## Terminal Integration
- Terminal follows kubeconfig, context, and namespace changes to keep `kubectl` in sync.
- Enter-only navigation when the terminal did not already consume typed keys.

## Data Model & Live Updates
- All Kubernetes data is live via controller-runtime informers; list/watch requests prefer “Table” responses for columns.
- Each kubeconfig+context uses its own controller-runtime `cluster` (and cache) so informers can be started/stopped independently.
- UI list updates are driven by Watch events from the cache; emit a `Synced` signal after initial informer sync to trigger the first stable render.

## API Surface Coverage
- Discover all resources (including CRDs) via discovery; navigation must work generically for every resource. Use OpenAPI v3 where necessary for creation dialogs and schema-aware input.
- Periodically refresh discovery (≈ every 30s) and reset RESTMapper to pick up new/removed APIs. Prefer aggregated discovery clients and cached mappers with invalidation.

## Resource‑Agnostic Model
- Listings and watches are resource‑agnostic: drive UI from GroupVersionResource (GVR) resolved via discovery/RESTMapper, not hardcoded kinds.
- Use controller‑runtime cache/informers for watch events keyed by GVR and namespace; emit an initial Synced signal to trigger first render.
- Panels render generic tables using server‑provided Table columns when available; fall back to minimal metadata (name, namespace, age, status).
- Extensible enhancements are allowed for well‑known resources (e.g., Pods, Nodes) without wrapping core types; compose via handlers/actions.
- Preserve cursor stability across updates where possible; when rows shrink, cursor may move up to the last item.

## Presentation
- Table columns: use server-provided Table columns where available for each resource type; render a header row and column-aligned cells; fall back to names/metadata when unavailable.
- Status line in each panel bottom shows key details for the item under the cursor.
- Path breadcrumb is drawn at the top of the panel overlaying the frame (e.g., `/cluster/namespaces/kube-system/pods`) and is ellipsized from the left (`.../kube-system/pods`) when too long to keep the frame intact.
- YAML viewer uses Chroma syntax highlighting. Theme is selectable (default: Dracula) and must not override the panel background; background remains the app’s dark blue.

## Menus & View Options
- Popdown menu bar (mc-style) planned at the top.
- "View" menu toggles panel settings per side: sort order (by name, creationTimestamp, last change time from `metadata.managedFields`), sort direction, column visibility, and optional grouping.
- Tri‑state settings (Yes/No/Default) with scope: global defaults and per‑resource overrides (by resource plural). Example: Table View = Default globally, but Pods = Yes, Services = No.

## Favorites
- Provide a favorites list of resource types used to populate shortcuts (e.g., the default resource set in selectors). Seed this list from the server’s discovery alias "all". Users can add or remove resources from favorites to override discovery (persist this preference and respect it across sessions).

## Object Views
- Pods: entering a pod shows its containers (including initContainers) as folders. Under each container, a `logs` entry exists. `F3` on `logs` opens a modal viewer; `Ctrl+F` follows (jump to end + live streaming). `Esc` closes.
- ConfigMaps/Secrets: entering shows data keys as file‑like entries. `F3` views the value in a modal; `F4` edits the field in an editor modal (text); binary data is indicated and not shown raw.
- YAML view (`F3`): fullscreen popup with syntax highlighting; supports Up/Down, PgUp/PgDn, Home/End, and `Esc` to exit. `F4` from the viewer runs `kubectl edit`. Always render the real object (strip `managedFields`), not a Table.
- Edit (`F4`): runs `kubectl edit` for the current object; on save, refreshes the view.

## Function Keys
- The function key bar is dynamic and context‑aware. Keys with no action in the current location/object are greyed out; active keys are highlighted.

## Terminal 2‑Line Mode
- When the 2‑line terminal has typed input, `Enter` and `Ctrl+C` return focus to the panel instead of sending the keys to the terminal. Otherwise keys are routed according to the standard gating rules.

## Table Behavior
- Namespaces and resource lists prefer server‑side Tables with headers and aligned columns when available; fall back to metadata otherwise.
- When a Table exceeds the panel width, implement column‑wise horizontal scrolling with Left/Right keys (only when the terminal has no typed input).
- Provide two modes:
  - Condensed: aggressively trim columns to fit the available width.
  - Full width: allocate the space columns need and enable horizontal scrolling.

## Internal Identity
- Internal navigation and selection must carry precise Kubernetes identity, not just display names. Each location item should include its `GroupVersionResource` and/or `GroupVersionKind` as resolved via the RESTMapper for the current cluster/context. Avoid ambiguous plain strings like "pods" for internal logic.
- Additional sort keys:
  - Nodes: by Capacity (CPU/memory), by Resource Consumption (from metrics API), by Status health.
  - Pods: by Resource Consumption (CPU/memory), by Status health, by Ready containers.
- Inverse ordering supported orthogonally to sort key (Asc/Desc toggle).
- Menu navigation via keyboard shortcuts and F-keys; persists per panel and path kind.

### Sorting Rules
- Status health order (most broken first):
  - Nodes: NotReady > Unknown > Ready=False (with critical conditions) > Ready=True.
  - Pods: Failed > CrashLoopBackOff/ImagePullBackOff > Pending/Unschedulable > Running (not Ready) > Running (Ready) > Succeeded.
- Resource consumption: prefer metrics.k8s.io if available; degrade to recent usage samples if cached; otherwise omit and fall back to name.
- Capacity: compute from `status.capacity` (nodes) and requested/limits from pod spec; tie-break by name.

## Panel Menus & Modes
- Each panel has a dedicated menu ("Left" / "Right") controlling its view mode:
  - API Surface (default): hierarchical browser as specified above.
  - Describe: `kubectl describe`-like view for the current object.
  - YAML: syntax-highlighted YAML for the current object; read-only for View, editable via `F4`.
  - Logs (pin): follow pod/container logs in the panel; selection chooses container when applicable.
  - Top (metrics): `kubectl top`-like table for pods/nodes (requires metrics API); gracefully degrade if unavailable.
- Modes are per-panel and persist across navigation within the same path kind; reset on context/kubeconfig switch unless pinned.

## Panel Swap
- `Ctrl+U` swaps the two panels' locations, cursor/scroll positions, selections, and active view modes.

## Panels & Focus
- Two symmetrical panels; only one focused. `Tab` switches focus.
- Up/Down moves cursor; `Enter` follows into folders/resources; `..` moves up one level.
- Entering any folder (namespace/context/resource group/resource) positions the cursor on `..`; going back restores the previous selection when possible.
- Only enterable kinds show a leading `/` and accept `Enter`: pods (containers/logs), secrets/configmaps (keys view) are enterable; non‑enterable kinds (e.g., services) appear as plain rows.
- At `/namespaces/<ns>` hide empty resource folders; show a count column for non‑empty resource folders.

## Contexts & Kubeconfigs
- All browsing is for the current kubeconfig and context by default.
- `/contexts` lets users switch context; `/kubeconfigs` lists known kubeconfigs and offers “Add kubeconfig…” dialog to register a path.

## API Group Hierarchy (Optional)
- Optional path mode to expose group/version layout:
  - `/groups/<api-group>/<version>/[namespaces/<ns>/]<resource>`
- When enabled, both classic paths (e.g., `/cluster/pods`) and group paths resolve to the same data source.
- Group mode aids discovery and disambiguation for resources with identical kinds across groups.

## Non-Goals (for now)
- No live-cluster destructive operations without confirmation dialogs.
- No partial edits without server-side validation; edits apply on save.

## Performance & Reliability
- Avoid blocking UI on network; use async loads with spinners/placeholders.
- Cache informer data per kubeconfig/context/namespace; reuse across panels when possible.
