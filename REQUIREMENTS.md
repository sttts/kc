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
- "View" menu toggles panel settings per side: sort order (by name, creationTimestamp, last change time derived from `metadata.managedFields`), sort direction, column visibility, and optional grouping.
- Menu navigation via keyboard shortcuts and F-keys; persists per panel and path kind.

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
