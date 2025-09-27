# Kubernetes Commander (kc) Requirements

## Purpose & Scope
- Terminal UI for Kubernetes that looks and feels like Midnight Commander (mc).
- Two-panel browser over Kubernetes APIs with live data and actionable keys.

## UX Parity with mc
- Two panels; one active at a time. Switch with `Tab`.
- Navigate with arrows; `Enter` opens item; `..` goes up.
- Function-key bar always visible; actions greyed when not applicable.
- Selected items are bold yellow; selection clears when changing location.

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

## Extensible Actions (Per Resource Type)
- Allow adding extra actions that can call external tools based on current location and selection.
- Pass context via env/args: `KUBECONFIG`, `CONTEXT`, `NAMESPACE`, `GROUP`, `VERSION`, `KIND`, `NAME`, and serialized object (path or stdin).
- Actions may be contributed by handlers or configured by users; operate on multi-selection when present.

## Terminal Integration
- Terminal follows kubeconfig, context, and namespace changes to keep `kubectl` in sync.
- Enter-only navigation when the terminal did not already consume typed keys.

## Data Model & Live Updates
- All Kubernetes data is live via controller-runtime informers; list/watch requests prefer “Table” responses for columns.
- Preserve cursor stability across updates where possible; when rows shrink, cursor may move up to the last item.

## Presentation
- Table columns: use server-provided table columns where available for each resource type.
- Status line in each panel bottom shows key details for the item under the cursor.
- Path breadcrumb is drawn at the top of the panel overlaying the frame (e.g., `/cluster/namespaces/kube-system/pods`).

## Menus & View Options
- Popdown menu bar (mc-style) planned at the top.
- "View" menu toggles panel settings per side: sort order (by name, creationTimestamp, last change time from `metadata.managedFields`), sort direction, column visibility, and optional grouping.
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
